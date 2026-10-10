package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStore returns a store backed by a temporary directory.
func newStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenWithDataDir(filepath.Join(dir, "library.json"), dir)
	if err != nil {
		t.Fatalf("OpenWithDataDir: %v", err)
	}
	return s
}

// addBook adds a book with the given chapter paths.
func addBook(t *testing.T, s *Store, title string, paths ...string) *Book {
	t.Helper()
	b := &Book{ID: NewBookID(), Title: title, Author: "Author"}
	for _, p := range paths {
		b.Chapters = append(b.Chapters, Chapter{Path: p, Title: filepath.Base(p)})
	}
	if err := s.Add(b); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return b
}

func TestUpdateMetadataKeepsID(t *testing.T) {
	s := newStore(t)
	book := addBook(t, s, "Old Title", "/a/1.mp3")

	if err := s.UpdateMetadata(book.ID, Metadata{Title: "New Title", Author: "New Author", Narrator: "Reader"}); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	got, err := s.Book(book.ID)
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if got.ID != book.ID {
		t.Fatal("metadata edit changed the book ID, which would orphan progress")
	}
	if got.Title != "New Title" || got.Author != "New Author" || got.Narrator != "Reader" {
		t.Fatalf("got %+v", got)
	}

	if err := s.UpdateMetadata("missing", Metadata{Title: "x"}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateMetadataTrimsWhitespace(t *testing.T) {
	s := newStore(t)
	book := addBook(t, s, "T", "/a/1.mp3")
	if err := s.UpdateMetadata(book.ID, Metadata{Title: "  Padded  ", Author: "\tA\n"}); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	got, _ := s.Book(book.ID)
	if got.Title != "Padded" || got.Author != "A" {
		t.Fatalf("got %q / %q", got.Title, got.Author)
	}
}

func TestMoveChapter(t *testing.T) {
	s := newStore(t)
	book := addBook(t, s, "Book", "/a/1.mp3", "/a/2.mp3", "/a/3.mp3")

	// Move the first chapter to the end.
	if err := s.MoveChapter(book.ID, 0, 2); err != nil {
		t.Fatalf("MoveChapter: %v", err)
	}
	got, _ := s.Book(book.ID)
	want := []string{"2.mp3", "3.mp3", "1.mp3"}
	for i, ch := range got.Chapters {
		if filepath.Base(ch.Path) != want[i] {
			t.Fatalf("chapter %d = %s, want %s", i, filepath.Base(ch.Path), want[i])
		}
	}
	if !got.CustomOrder {
		t.Fatal("manual reorder should be remembered")
	}

	// Move it back to the front.
	if err := s.MoveChapter(book.ID, 2, 0); err != nil {
		t.Fatalf("MoveChapter: %v", err)
	}
	got, _ = s.Book(book.ID)
	want = []string{"1.mp3", "2.mp3", "3.mp3"}
	for i, ch := range got.Chapters {
		if filepath.Base(ch.Path) != want[i] {
			t.Fatalf("chapter %d = %s, want %s", i, filepath.Base(ch.Path), want[i])
		}
	}

	if err := s.MoveChapter(book.ID, 9, 0); err == nil {
		t.Fatal("expected an error for an out-of-range move")
	}
}

func TestRemoveChapter(t *testing.T) {
	s := newStore(t)
	book := addBook(t, s, "Book", "/a/1.mp3", "/a/2.mp3")

	if err := s.RemoveChapter(book.ID, 0); err != nil {
		t.Fatalf("RemoveChapter: %v", err)
	}
	got, _ := s.Book(book.ID)
	if len(got.Chapters) != 1 || filepath.Base(got.Chapters[0].Path) != "2.mp3" {
		t.Fatalf("got %+v", got.Chapters)
	}

	// Removing the last chapter removes the book.
	if err := s.RemoveChapter(book.ID, 0); err != nil {
		t.Fatalf("RemoveChapter: %v", err)
	}
	if s.Len() != 0 {
		t.Fatal("a book with no chapters should be removed")
	}
}

func TestRemoveChapterDoesNotAliasSlice(t *testing.T) {
	s := newStore(t)
	book := addBook(t, s, "Book", "/a/1.mp3", "/a/2.mp3", "/a/3.mp3")
	if err := s.RemoveChapter(book.ID, 1); err != nil {
		t.Fatalf("RemoveChapter: %v", err)
	}
	got, _ := s.Book(book.ID)
	want := []string{"1.mp3", "3.mp3"}
	if len(got.Chapters) != 2 {
		t.Fatalf("got %d chapters", len(got.Chapters))
	}
	for i, ch := range got.Chapters {
		if filepath.Base(ch.Path) != want[i] {
			t.Fatalf("chapter %d = %s, want %s", i, filepath.Base(ch.Path), want[i])
		}
	}
}

func TestRescanUpdatesDurationsAndDropsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	present := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(20), nil)
	missing := filepath.Join(dir, "gone.mp3")

	s := newStore(t)
	book := &Book{ID: NewBookID(), Title: "Book", Author: "A", Album: "The Album"}
	book.Chapters = []Chapter{
		{Path: present, Title: "Chapter One", DurationMS: 0},
		{Path: missing, Title: "gone"},
	}
	if err := s.Add(book); err != nil {
		t.Fatalf("Add: %v", err)
	}

	changed, err := s.RescanChapters(book.ID)
	if err != nil {
		t.Fatalf("RescanChapters: %v", err)
	}
	if changed == 0 {
		t.Fatal("rescan should report changes")
	}
	got, _ := s.Book(book.ID)
	if len(got.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1 (the missing file should be dropped)", len(got.Chapters))
	}
	if got.Chapters[0].DurationMS == 0 {
		t.Fatal("rescan should fill in the duration")
	}
}

func TestRescanPicksUpNewFilesAndKeepsRenamedTitles(t *testing.T) {
	dir := t.TempDir()
	first := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(20), nil)
	// A second chapter appears in the same folder after the first import.
	writeTaggedMP3(t, dir, "Chapter Two.mp3", silentMP3Frames(20), nil)

	s := newStore(t)
	book := &Book{ID: NewBookID(), Title: "Book", Author: "A", Album: "The Album"}
	book.Chapters = []Chapter{{Path: first, Title: "My Own Name"}}
	if err := s.Add(book); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := s.RescanChapters(book.ID); err != nil {
		t.Fatalf("RescanChapters: %v", err)
	}
	got, _ := s.Book(book.ID)
	if len(got.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(got.Chapters))
	}
	for _, ch := range got.Chapters {
		if filepath.Base(ch.Path) == "Chapter One.mp3" && ch.Title != "My Own Name" {
			t.Fatalf("a hand-renamed chapter was overwritten: %q", ch.Title)
		}
	}
}

func TestRescanRespectsCustomOrder(t *testing.T) {
	dir := t.TempDir()
	a := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(20), nil)
	b := writeTaggedMP3(t, dir, "Chapter Two.mp3", silentMP3Frames(20), nil)

	s := newStore(t)
	book := &Book{ID: NewBookID(), Title: "Book", Album: "The Album"}
	book.Chapters = []Chapter{{Path: b, Title: "Chapter Two"}, {Path: a, Title: "Chapter One"}}
	book.CustomOrder = true
	if err := s.Add(book); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := s.RescanChapters(book.ID); err != nil {
		t.Fatalf("RescanChapters: %v", err)
	}
	got, _ := s.Book(book.ID)
	if filepath.Base(got.Chapters[0].Path) != "Chapter Two.mp3" {
		t.Fatal("rescan re-sorted a book the listener ordered by hand")
	}
}

func TestMergeBooks(t *testing.T) {
	s := newStore(t)
	target := addBook(t, s, "Target", "/a/1.mp3")
	source := addBook(t, s, "Source", "/b/1.mp3", "/b/2.mp3")

	if err := s.MergeBooks(target.ID, source.ID); err != nil {
		t.Fatalf("MergeBooks: %v", err)
	}
	got, err := s.Book(target.ID)
	if err != nil {
		t.Fatalf("target is gone: %v", err)
	}
	if len(got.Chapters) != 3 {
		t.Fatalf("target has %d chapters, want 3", len(got.Chapters))
	}
	if _, err := s.Book(source.ID); err != ErrNotFound {
		t.Fatal("source book should be removed after merging")
	}
	if err := s.MergeBooks(target.ID, target.ID); err == nil {
		t.Fatal("merging a book into itself should fail")
	}
}

func TestMergeBooksSkipsDuplicateChapters(t *testing.T) {
	s := newStore(t)
	target := addBook(t, s, "Target", "/a/1.mp3")
	source := addBook(t, s, "Source", "/a/1.mp3")

	if err := s.MergeBooks(target.ID, source.ID); err != nil {
		t.Fatalf("MergeBooks: %v", err)
	}
	got, _ := s.Book(target.ID)
	if len(got.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1", len(got.Chapters))
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	s := newStore(t)
	addBook(t, s, "Alpha", "/a/1.mp3", "/a/2.mp3")
	addBook(t, s, "Beta", "/b/1.mp3")

	out := filepath.Join(t.TempDir(), "export.json")
	if err := s.Export(out); err != nil {
		t.Fatalf("Export: %v", err)
	}

	fresh := newStore(t)
	added, merged, err := fresh.Import(out)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if added != 2 || merged != 0 {
		t.Fatalf("added=%d merged=%d, want 2 and 0", added, merged)
	}
	if fresh.Len() != 2 {
		t.Fatalf("library has %d books, want 2", fresh.Len())
	}

	// Importing the same file again should merge, not duplicate.
	added, merged, err = fresh.Import(out)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if added != 0 || merged != 2 {
		t.Fatalf("added=%d merged=%d, want 0 and 2", added, merged)
	}
	if fresh.Len() != 2 {
		t.Fatalf("library grew to %d books", fresh.Len())
	}
}

func TestImportRejectsBadInput(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(t.TempDir(), "junk.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Import(path); err == nil {
		t.Fatal("expected an error for a non-JSON file")
	}

	newer := filepath.Join(t.TempDir(), "newer.json")
	if err := os.WriteFile(newer, []byte(`{"version":999,"books":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Import(newer); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected a version error, got %v", err)
	}
}

func TestSearch(t *testing.T) {
	s := newStore(t)
	alpha := addBook(t, s, "The Hobbit", "/a/1.mp3")
	alpha.Author = "Tolkien"
	if err := s.Add(alpha); err != nil {
		t.Fatal(err)
	}
	beta := addBook(t, s, "Dune", "/b/1.mp3")
	beta.Narrator = "Scott Brick"
	if err := s.Add(beta); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"":        "", // empty matches everything
		"hobbit":  "The Hobbit",
		"TOLKIEN": "The Hobbit",
		"scott":   "Dune",
		"chapter": "", // chapter titles are "1.mp3" here, no match
		"no such": "",
	}
	for query, want := range cases {
		got := s.Search(query)
		if want == "" {
			if query == "" && len(got) != 2 {
				t.Fatalf("empty query returned %d books, want 2", len(got))
			}
			if query != "" && len(got) != 0 {
				t.Fatalf("query %q returned %d books, want 0", query, len(got))
			}
			continue
		}
		if len(got) != 1 || got[0].Title != want {
			t.Fatalf("query %q returned %+v, want %q", query, got, want)
		}
	}
}

func TestSearchMatchesChapterTitles(t *testing.T) {
	s := newStore(t)
	book := &Book{ID: NewBookID(), Title: "Book", Chapters: []Chapter{{Path: "/a/1.mp3", Title: "The Council of Elrond"}}}
	if err := s.Add(book); err != nil {
		t.Fatal(err)
	}
	if got := s.Search("elrond"); len(got) != 1 {
		t.Fatalf("search by chapter title returned %d books", len(got))
	}
}

func TestConcurrentReadsAndWritesAreRaceFree(t *testing.T) {
	// The store hands out copies so a UI goroutine can read a book while
	// playback, a rescan, or an import mutates it. Run with -race.
	s := newStore(t)
	book := addBook(t, s, "Book", "/a/1.mp3", "/a/2.mp3")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = s.Books()
			if b, err := s.Book(book.ID); err == nil {
				for _, ch := range b.Chapters {
					_ = ch.Path
				}
				_ = b.TotalDuration()
			}
			_ = s.Search("book")
		}
	}()

	for i := 0; i < 200; i++ {
		if err := s.UpdateMetadata(book.ID, Metadata{Title: "Book", Author: "A"}); err != nil {
			t.Fatalf("UpdateMetadata: %v", err)
		}
		if err := s.MoveChapter(book.ID, 0, 1); err != nil {
			t.Fatalf("MoveChapter: %v", err)
		}
	}
	<-done
}

func TestBooksReturnsCopies(t *testing.T) {
	s := newStore(t)
	book := addBook(t, s, "Book", "/a/1.mp3")

	books := s.Books()
	books[0].Title = "Mutated"
	books[0].Chapters[0].Path = "/mutated.mp3"

	stored, err := s.Book(book.ID)
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if stored.Title != "Book" {
		t.Fatalf("mutating a returned book changed the store: %q", stored.Title)
	}
	if stored.Chapters[0].Path != "/a/1.mp3" {
		t.Fatalf("mutating a returned chapter changed the store: %q", stored.Chapters[0].Path)
	}
}

func TestImportFileDoesNotDuplicateDistinctBooks(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeTaggedMP3(t, dirA, "Book A.mp3", silentMP3Frames(10), nil)
	writeTaggedMP3(t, dirB, "Book B.mp3", silentMP3Frames(10), nil)

	s := newStore(t)
	// Both carry the same album tag, but they live in different folders, so
	// importing one must not swallow the other.
	first, err := s.ImportFile(filepath.Join(dirA, "Book A.mp3"))
	if err != nil {
		t.Fatalf("ImportFile A: %v", err)
	}
	if len(first.Chapters) != 1 {
		t.Fatalf("book A has %d chapters, want 1", len(first.Chapters))
	}
}
