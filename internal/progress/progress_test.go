package progress

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPercent(t *testing.T) {
	tests := []struct {
		name           string
		chapter        int
		position, base int64
		total          int64
		want           float64
	}{
		{"start", 0, 0, 0, 1000, 0},
		{"middle", 1, 250, 1000, 2000, 0.625},
		{"end", 1, 1000, 1000, 2000, 1},
		{"unknown total", 0, 500, 0, 0, 0},
		{"clamped", 0, 5000, 0, 1000, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Percent(tt.chapter, tt.position, tt.base, tt.total); got != tt.want {
				t.Fatalf("Percent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("expected no entry for unknown book")
	}
	if err := s.Set("book-1", Entry{ChapterIndex: 2, PositionMS: 1500, Percent: 0.4}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := reopened.Get("book-1")
	if !ok {
		t.Fatal("entry was not persisted")
	}
	if got.ChapterIndex != 2 || got.PositionMS != 1500 {
		t.Fatalf("got %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt was not set")
	}

	if err := reopened.Delete("book-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := reopened.Get("book-1"); ok {
		t.Fatal("entry was not deleted")
	}
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "nope", "progress.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(s.All()) != 0 {
		t.Fatal("expected empty store")
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "nope")); !os.IsNotExist(err) {
		t.Fatal("Open should not create directories")
	}
}
