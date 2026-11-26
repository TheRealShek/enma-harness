package threads

import (
	"bufio"
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/therealshek/enma-harness/config"
)

const (
	metadataFileName = "meta.json"
	messagesFileName = "messages.jsonl"
)

// Metadata stores mutable metadata for a saved conversation thread.
type Metadata struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProjectRoot string    `json:"project_root"`
	Model       string    `json:"model"`
	Tainted     bool      `json:"tainted"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store manages thread directories under BaseDir.
type Store struct {
	BaseDir string
}

// DefaultStore returns a Store rooted at ~/.enma/threads.
func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("unable to determine home directory: %w", err)
	}

	return &Store{
		BaseDir: filepath.Join(home, config.HarnessDirName, config.ThreadsDirName),
	}, nil
}

// Create initializes a new thread directory and writes its initial meta.json.
func (s *Store) Create(projectRoot, model, name string, tainted bool) (*Metadata, error) {
	id, err := newThreadID()
	if err != nil {
		return nil, err
	}

	dir := s.threadDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create thread directory: %w", err)
	}

	now := time.Now().UTC()
	meta := &Metadata{
		ID:          id,
		Name:        cmp.Or(strings.TrimSpace(name), id),
		ProjectRoot: projectRoot,
		Model:       model,
		Tainted:     tainted,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.SaveMetadata(meta); err != nil {
		return nil, err
	}

	return meta, nil
}

// SaveMetadata atomically writes updated thread metadata to meta.json.
func (s *Store) SaveMetadata(meta *Metadata) error {
	meta.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode thread metadata: %w", err)
	}

	data = append(data, '\n')

	return atomicWriteFile(filepath.Join(s.threadDir(meta.ID), metadataFileName), data, 0o600)
}

// AppendMessage appends one JSON-encoded message line to messages.jsonl.
func (s *Store) AppendMessage(id string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	data = append(data, '\n')

	path := filepath.Join(s.threadDir(id), messagesFileName)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open messages log: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()

		return fmt.Errorf("failed to append message: %w", err)
	}

	return f.Close()
}

// Load reads a thread's metadata and message lines, ignoring a truncated trailing line after a crash.
func (s *Store) Load(id string) (*Metadata, []json.RawMessage, error) {
	metaBytes, err := os.ReadFile(filepath.Join(s.threadDir(id), metadataFileName))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read thread metadata: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, nil, fmt.Errorf("failed to parse thread metadata: %w", err)
	}

	msgPath := filepath.Join(s.threadDir(id), messagesFileName)

	f, err := os.Open(msgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &meta, nil, nil
		}

		return nil, nil, fmt.Errorf("failed to open messages log: %w", err)
	}

	defer f.Close()

	var (
		messages []json.RawMessage
		rawLines [][]byte
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		rawLines = append(rawLines, slices.Clone(line))
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to read messages log: %w", err)
	}

	for i, line := range rawLines {
		if !json.Valid(line) {
			if i == len(rawLines)-1 {
				break
			}

			return nil, nil, fmt.Errorf("corrupt message line %d in %s", i+1, id)
		}

		messages = append(messages, json.RawMessage(line))
	}

	return &meta, messages, nil
}

// List returns saved threads for projectRoot sorted newest UpdatedAt first.
func (s *Store) List(projectRoot string) ([]Metadata, error) {
	entries, err := os.ReadDir(s.BaseDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to list threads: %w", err)
	}

	var list []Metadata

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.threadDir(e.Name()), metadataFileName))
		if err != nil {
			continue
		}

		var meta Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		if projectRoot != "" && meta.ProjectRoot != projectRoot {
			continue
		}

		list = append(list, meta)
	}

	slices.SortFunc(list, func(a, b Metadata) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})

	return list, nil
}

// Find locates the newest thread for projectRoot matching query by exact ID or Name.
func (s *Store) Find(projectRoot, query string) (*Metadata, error) {
	list, err := s.List(projectRoot)
	if err != nil {
		return nil, err
	}

	q := strings.TrimSpace(query)
	for i := range list {
		if list[i].ID == q || list[i].Name == q {
			return &list[i], nil
		}
	}

	return nil, fmt.Errorf("thread %q not found", query)
}

// Paginate returns one page of threads along with totalPages and the clamped 1-based page number.
func Paginate(items []Metadata, page, pageSize int) ([]Metadata, int, int) {
	if len(items) == 0 {
		return nil, 1, 1
	}

	if pageSize <= 0 {
		pageSize = config.ThreadsPerPage
	}

	totalPages := (len(items) + pageSize - 1) / pageSize
	page = max(1, min(page, totalPages))

	start := (page - 1) * pageSize
	end := min(start+pageSize, len(items))

	return items[start:end], totalPages, page
}

// NameFromPrompt derives a short single-line thread name from the first user prompt.
func NameFromPrompt(prompt string) string {
	collapsed := strings.Join(strings.Fields(prompt), " ")
	runes := []rune(collapsed)

	if len(runes) <= 40 {
		return collapsed
	}

	return string(runes[:37]) + "..."
}

func (s *Store) threadDir(id string) string {
	return filepath.Join(s.BaseDir, id)
}

func newThreadID() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("failed to generate thread id: %w", err)
	}

	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(b[:])), nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".meta-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tempPath := tempFile.Name()

	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()

		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()

		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Chmod(tempPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
