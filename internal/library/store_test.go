package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewBookIDIsUniqueAndIndependentOfMetadata(t *testing.T) {
	// Book IDs must not depend on title or author, because those are editable
	// and progress is keyed by ID.
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewBookID()
		if id == "" {
			t.Fatal("NewBookID returned an empty ID")
		}
		if seen[id] {
			t.Fatalf("NewBookID returned a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Chapter 2", "Chapter 10", true},
		{"Chapter 10", "Chapter 2", false},
		{"Chapter 1", "Chapter 2", true},
		{"intro", "Chapter 1", false},
	}
	for _, c := range cases {
		if got := NaturalLess(c.a, c.b); got != c.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestStoreAddRemovePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	book := &Book{ID: "id-1", Title: "Book", Author: "Author", Chapters: []Chapter{{Path: "/x/1.mp3"}}}
	if err := s.Add(book); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
	if book.AddedAt.IsZero() {
		t.Fatal("AddedAt was not set")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := reopened.Book("id-1")
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if got.Title != "Book" {
		t.Fatalf("got title %q", got.Title)
	}

	if err := reopened.Remove("id-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if reopened.Len() != 0 {
		t.Fatal("book was not removed")
	}
}

func TestImportFileGroupsAlbumSiblings(t *testing.T) {
	dir := t.TempDir()
	// ImportFile reads real ID3 tags, so the grouping path is exercised by
	// scanning files with no tags: every audio file in the directory is grouped
	// when the album is unknown.
	for _, name := range []string{"Chapter 1.mp3", "Chapter 2.mp3", "Chapter 10.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("not real audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	book, err := s.ImportFile(filepath.Join(dir, "Chapter 1.mp3"))
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(book.Chapters))
	}
	if got := book.Chapters[2].Title; got != "Chapter 10" {
		t.Fatalf("chapters are not in natural order: %+v", book.Chapters)
	}
	if book.Source != "local" {
		t.Fatalf("Source = %q", book.Source)
	}
	if book.ID == "" {
		t.Fatal("book has no ID")
	}
}

func TestImportFileIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Chapter 1.mp3", "Chapter 2.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("no tags"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first, err := s.ImportFile(filepath.Join(dir, "Chapter 1.mp3"))
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := s.ImportFile(filepath.Join(dir, "Chapter 2.mp3"))
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("re-importing a sibling created a new book: %q vs %q", first.ID, second.ID)
	}
	if s.Len() != 1 {
		t.Fatalf("library has %d books, want 1", s.Len())
	}
	if len(second.Chapters) != 2 {
		t.Fatalf("book has %d chapters, want 2", len(second.Chapters))
	}
}

func TestFindByChapterPaths(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	book := &Book{ID: NewBookID(), Title: "Book", Chapters: []Chapter{{Path: "/a/1.mp3"}, {Path: "/a/2.mp3"}}}
	if err := s.Add(book); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := s.FindByChapterPaths([]Chapter{{Path: "/a/2.mp3"}}); got == nil || got.ID != book.ID {
		t.Fatalf("expected to find the book by chapter path, got %+v", got)
	}
	if got := s.FindByChapterPaths([]Chapter{{Path: "/b/9.mp3"}}); got != nil {
		t.Fatalf("expected no match, got %+v", got)
	}
	if got := s.FindByChapterPaths(nil); got != nil {
		t.Fatalf("expected no match for an empty query, got %+v", got)
	}
}

func TestImportDirGroupsWholeDirectoryWhenUntagged(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1.mp3", "2.mp3", "3.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("no tags"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	books, err := s.ImportDir(dir)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("imported %d books, want 1 (the whole folder)", len(books))
	}
	if got := books[0].Title; got != filepath.Base(dir) {
		t.Fatalf("book title = %q, want %q", got, filepath.Base(dir))
	}
	if len(books[0].Chapters) != 3 {
		t.Fatalf("book has %d chapters, want 3", len(books[0].Chapters))
	}
}

func TestBookKeyIgnoresChapterSet(t *testing.T) {
	// The key that groups files into a book must not depend on which chapters
	// happen to be present, otherwise scanning order splits a book apart.
	a := BookKey("The Hobbit", "Tolkien")
	b := BookKey("the hobbit", "Tolkien")
	if a != b {
		t.Fatalf("BookKey is case sensitive: %q vs %q", a, b)
	}
	if a == BookKey("The Hobbit", "Someone Else") {
		t.Fatal("BookKey should depend on the author")
	}
}

func TestBookHelpers(t *testing.T) {
	b := &Book{Chapters: []Chapter{{DurationMS: 1000}, {DurationMS: 2500}}}
	if got := b.TotalDuration().Milliseconds(); got != 3500 {
		t.Fatalf("TotalDuration = %d", got)
	}
	if b.DisplayAuthor() != "Unknown author" {
		t.Fatal("expected placeholder author")
	}
	if _, ok := b.ChapterAt(5); ok {
		t.Fatal("ChapterAt out of range should be false")
	}
	if _, ok := b.ChapterAt(1); !ok {
		t.Fatal("ChapterAt in range should be true")
	}
}
