package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

type cliSession struct {
	store             *threads.Store
	projectRoot       string
	activeThread      *threads.Metadata
	pendingThreadName string
	conversation      []Message
	toolSession       *tools.Session
	scanner           *bufio.Scanner
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
	if s.store == nil {
		fmt.Println(styleMuted("InMemoryOnly is enabled; no threads are stored."))
		fmt.Println()

		return
	}

	list, err := s.store.List(s.projectRoot)
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
	if s.store == nil {
		fmt.Println(styleMuted("InMemoryOnly is enabled; no threads are stored."))
		fmt.Println()

		return
	}

	if query != "" {
		meta, err := s.store.Find(s.projectRoot, query)
		if err != nil {
			fmt.Println(styleError("Error:"), err)
			fmt.Println()

			return
		}

		s.loadThread(meta.ID)

		return
	}

	list, err := s.store.List(s.projectRoot)
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

	if s.activeThread == nil {
		s.pendingThreadName = name
		fmt.Println(styleMuted("Thread name set to: " + name))
		fmt.Println()

		return
	}

	s.activeThread.Name = name
	if s.store != nil {
		if err := s.store.SaveMetadata(s.activeThread); err != nil {
			fmt.Println(styleError("Error:"), err)
			fmt.Println()

			return
		}
	}

	fmt.Println(styleMuted("Renamed thread to: " + name))
	fmt.Println()
}

func (s *cliSession) startNewThread(name string) {
	s.activeThread = nil
	s.pendingThreadName = name
	s.conversation = []Message{{Role: "system", Content: config.DefaultSystemPrompt}}
	s.toolSession.Tainted = false

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
		if s.activeThread != nil && s.activeThread.ID == m.ID {
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
	meta, rawMsgs, err := s.store.Load(id)
	if err != nil {
		fmt.Println(styleError("Error:"), err)
		fmt.Println()

		return
	}

	loaded := make([]Message, 0, len(rawMsgs)+1)

	for _, raw := range rawMsgs {
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		loaded = append(loaded, msg)
	}

	if len(loaded) == 0 || loaded[0].Role != "system" {
		loaded = append([]Message{{Role: "system", Content: config.DefaultSystemPrompt}}, loaded...)
	}

	s.activeThread = meta
	s.pendingThreadName = ""
	s.conversation = loaded
	s.toolSession.Tainted = meta.Tainted

	fmt.Println(styleMuted(fmt.Sprintf("Resumed %q (%d messages, tainted=%v)", meta.Name, len(loaded)-1, meta.Tainted)))
	fmt.Println()
}

// createThreadIfNeeded initializes the thread directory on disk when the first real prompt is sent.
func (s *cliSession) createThreadIfNeeded(firstPrompt string) {
	if s.store == nil || s.activeThread != nil {
		return
	}

	name := cmp.Or(s.pendingThreadName, threads.NameFromPrompt(firstPrompt))

	meta, err := s.store.Create(s.projectRoot, config.DefaultModel, name, s.toolSession.Tainted)
	if err != nil {
		fmt.Println(styleError("Warning: failed to create thread:"), err)

		return
	}

	s.activeThread = meta
	s.pendingThreadName = ""

	if len(s.conversation) > 0 {
		if err := s.store.AppendMessage(meta.ID, s.conversation[0]); err != nil {
			fmt.Println(styleError("Warning: failed to save system message:"), err)
		}
	}
}

func (s *cliSession) appendMessage(msg Message) {
	s.conversation = append(s.conversation, msg)

	if s.store != nil && s.activeThread != nil {
		if err := s.store.AppendMessage(s.activeThread.ID, msg); err != nil {
			fmt.Println(styleError("Warning: failed to save message:"), err)
		}
	}
}

func (s *cliSession) persistTaintState() {
	if s.store == nil || s.activeThread == nil {
		return
	}

	if s.toolSession.Tainted && !s.activeThread.Tainted {
		s.activeThread.Tainted = true

		if err := s.store.SaveMetadata(s.activeThread); err != nil {
			fmt.Println(styleError("Warning: failed to save taint state:"), err)
		}
	}
}
