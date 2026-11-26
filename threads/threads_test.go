package threads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLifecycleAndPagination(t *testing.T) {
	store := &Store{BaseDir: t.TempDir()}

	meta, err := store.Create("/proj/a", "qwen3:4b", "auth-fix", false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ev, err := NewEvent(1, EventUserMessage, map[string]string{"content": "hi"})
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	if err := store.AppendEvent(meta.ID, ev); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	// Simulate a truncated trailing line from a crash mid-write.
	eventsPath := filepath.Join(store.BaseDir, meta.ID, eventsFileName)

	f, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = f.WriteString(`{"sequence":2,"type":"llm.response","data":{"con`)
	_ = f.Close()

	meta.Tainted = true
	if err := store.SaveMetadata(meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	loadedMeta, events, err := store.Load(meta.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !loadedMeta.Tainted || loadedMeta.Name != "auth-fix" {
		t.Errorf("loaded meta = %+v, want Tainted=true Name=auth-fix", loadedMeta)
	}

	if len(events) != 1 || events[0].Type != EventUserMessage || events[0].Sequence != 1 {
		t.Fatalf("events = %+v, want 1 user.message event (truncated last line should be skipped)", events)
	}

	found, err := store.Find("/proj/a", "auth-fix")
	if err != nil || found.ID != meta.ID {
		t.Fatalf("Find failed: found=%+v err=%v", found, err)
	}

	if _, err := store.Find("/proj/b", "auth-fix"); err == nil {
		t.Fatal("expected Find for another project root to fail")
	}
}

func TestPaginateAndNameFromPrompt(t *testing.T) {
	items := make([]Metadata, 25)
	for i := range items {
		items[i] = Metadata{ID: string(rune('a' + i))}
	}

	p1, total, page := Paginate(items, 1, 10)
	if len(p1) != 10 || total != 3 || page != 1 {
		t.Errorf("page 1 = (%d, %d, %d), want (10, 3, 1)", len(p1), total, page)
	}

	p3, _, page3 := Paginate(items, 99, 10)
	if len(p3) != 5 || page3 != 3 {
		t.Errorf("clamped page = (%d, %d), want (5, 3)", len(p3), page3)
	}

	if got := NameFromPrompt("  hello\n   world  "); got != "hello world" {
		t.Errorf("NameFromPrompt = %q, want %q", got, "hello world")
	}
}
