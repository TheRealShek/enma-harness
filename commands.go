package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/threads"
)

type cliSession struct {
	runner  *Runner
	scanner *bufio.Scanner
}

// handleCommand runs a /threads, /resume, /rename, or /new command and reports whether input was a slash command.
func (s *cliSession) handleCommand(input string) bool {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return false
	}

	command, arg, _ := strings.Cut(trimmed, " ")
	arg = strings.TrimSpace(arg)

	switch command {
	case "/threads":
		page := 1

		if arg != "" {
			if p, err := strconv.Atoi(arg); err == nil {
				page = p
			}
		}

		s.listThreads(page)
	case "/resume":
		s.resumeThread(arg)
	case "/rename":
		s.renameThread(arg)
	case "/new":
		s.startNewThread(arg)
	default:
		fmt.Println(styleError("Unknown command. Available: /threads, /resume, /rename, /new"))
		fmt.Println()
	}

	return true
}

func (s *cliSession) listThreads(page int) {
	if s.runner.store == nil {
		fmt.Println(styleMuted("InMemoryOnly is enabled; no threads are stored."))
		fmt.Println()

		return
	}

	list, err := s.runner.store.List(s.runner.projectRoot)
	if err != nil {
		fmt.Println(styleError("Error:"), err)
		fmt.Println()

		return
	}

	if len(list) == 0 {
		fmt.Println(styleMuted("No saved threads for this project."))
		fmt.Println()

		return
	}

	items, totalPages, clampedPage := threads.Paginate(list, page, config.ThreadsPerPage)
	s.printThreadsPage(items, clampedPage, totalPages)
	fmt.Println()
}

func (s *cliSession) resumeThread(query string) {
	if s.runner.store == nil {
		fmt.Println(styleMuted("InMemoryOnly is enabled; no threads are stored."))
		fmt.Println()

		return
	}

	if query != "" {
		meta, err := s.runner.store.Find(s.runner.projectRoot, query)
		if err != nil {
			fmt.Println(styleError("Error:"), err)
			fmt.Println()

			return
		}

		s.loadThread(meta.ID)

		return
	}

	list, err := s.runner.store.List(s.runner.projectRoot)
	if err != nil {
		fmt.Println(styleError("Error:"), err)
		fmt.Println()

		return
	}

	if len(list) == 0 {
		fmt.Println(styleMuted("No saved threads for this project."))
		fmt.Println()

		return
	}

	page := 1

	for {
		items, totalPages, clampedPage := threads.Paginate(list, page, config.ThreadsPerPage)
		page = clampedPage
		s.printThreadsPage(items, page, totalPages)

		fmt.Print(styleMuted("Pick # (n=next, p=prev, q=cancel): "))

		if !s.scanner.Scan() {
			fmt.Println()

			return
		}

		choice := strings.TrimSpace(strings.ToLower(s.scanner.Text()))
		switch choice {
		case "q", "":
			fmt.Println()

			return
		case "n":
			if page < totalPages {
				page++
			}
		case "p":
			if page > 1 {
				page--
			}
		default:
			idx, err := strconv.Atoi(choice)
			if err != nil || idx < 1 || idx > len(items) {
				fmt.Println(styleError("Invalid selection."))

				continue
			}

			s.loadThread(items[idx-1].ID)

			return
		}
	}
}

func (s *cliSession) renameThread(name string) {
	if name == "" {
		fmt.Println(styleError("Usage: /rename <name>"))
		fmt.Println()

		return
	}

	if s.runner.activeThread == nil {
		s.runner.pendingThreadName = name
		fmt.Println(styleMuted("Thread name set to: " + name))
		fmt.Println()

		return
	}

	s.runner.activeThread.Name = name
	if s.runner.store != nil {
		if err := s.runner.store.SaveMetadata(s.runner.activeThread); err != nil {
			fmt.Println(styleError("Error:"), err)
			fmt.Println()

			return
		}
	}

	fmt.Println(styleMuted("Renamed thread to: " + name))
	fmt.Println()
}

func (s *cliSession) startNewThread(name string) {
	s.runner.Reset(name)

	if name != "" {
		fmt.Println(styleMuted("Started new thread: " + name))
	} else {
		fmt.Println(styleMuted("Started new thread."))
	}

	fmt.Println()
}

func (s *cliSession) printThreadsPage(items []threads.Metadata, page, totalPages int) {
	fmt.Println(styleMuted(fmt.Sprintf("Threads (page %d/%d):", page, totalPages)))

	for i, m := range items {
		marker := " "
		if s.runner.activeThread != nil && s.runner.activeThread.ID == m.ID {
			marker = "*"
		}

		taintTag := ""
		if m.Tainted {
			taintTag = " [tainted]"
		}

		fmt.Printf(" %s %2d. %s (%s)%s\n", marker, i+1, m.Name, m.UpdatedAt.Local().Format("2006-01-02 15:04"), taintTag)
	}
}

func (s *cliSession) loadThread(id string) {
	meta, err := s.runner.LoadThread(id)
	if err != nil {
		fmt.Println(styleError("Error:"), err)
		fmt.Println()

		return
	}

	fmt.Println(styleMuted(fmt.Sprintf("Resumed %q (%d messages, tainted=%v)", meta.Name, len(s.runner.conversation)-1, s.runner.toolSession.Tainted)))
	fmt.Println()
}
