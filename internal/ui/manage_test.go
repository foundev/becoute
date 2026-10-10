package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// bookWith builds a book with the given metadata and chapter count.
func bookWith(id, title, author string, chapters int, added time.Time) *library.Book {
	b := &library.Book{ID: id, Title: title, Author: author, AddedAt: added}
	for i := 0; i < chapters; i++ {
		b.Chapters = append(b.Chapters, library.Chapter{
			Title:      "Chapter " + string(rune('1'+i)),
			Path:       filepath.Join("/books", id, string(rune('1'+i))+".mp3"),
			DurationMS: 60_000,
		})
	}
	return b
}

func noEntries(string) (progress.Entry, bool) { return progress.Entry{}, false }

func TestApplySort(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	books := []*library.Book{
		bookWith("c", "Zebra", "B Author", 1, base.Add(1*time.Hour)),
		bookWith("a", "Alpha", "C Author", 3, base),
		bookWith("b", "Middle", "A Author", 2, base.Add(2*time.Hour)),
	}

	titles := func(bs []*library.Book) []string {
		out := make([]string, len(bs))
		for i, b := range bs {
			out[i] = b.Title
		}
		return out
	}

	cases := []struct {
		mode string
		want []string
	}{
		{sortRecent, []string{"Middle", "Zebra", "Alpha"}},
		{sortTitle, []string{"Alpha", "Middle", "Zebra"}},
		{sortAuthor, []string{"Middle", "Zebra", "Alpha"}},
		{sortDuration, []string{"Alpha", "Middle", "Zebra"}},
		{"unknown mode", []string{"Middle", "Zebra", "Alpha"}},
	}
	for _, c := range cases {
		got := append([]*library.Book(nil), books...)
		ApplySort(got, c.mode, noEntries)
		if diff := titles(got); !equalStrings(diff, c.want) {
			t.Errorf("ApplySort(%q) = %v, want %v", c.mode, diff, c.want)
		}
	}
}

func TestApplySortRecentlyPlayedPutsUnplayedLast(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	books := []*library.Book{
		bookWith("unplayed", "Unplayed", "A", 1, base),
		bookWith("old", "Old", "A", 1, base),
		bookWith("recent", "Recent", "A", 1, base),
	}
	entries := map[string]progress.Entry{
		"old":    {UpdatedAt: base.Add(time.Hour)},
		"recent": {UpdatedAt: base.Add(2 * time.Hour)},
	}
	lookup := func(id string) (progress.Entry, bool) {
		e, ok := entries[id]
		return e, ok
	}

	ApplySort(books, sortPlayed, lookup)
	got := []string{books[0].Title, books[1].Title, books[2].Title}
	want := []string{"Recent", "Old", "Unplayed"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatchesQuery(t *testing.T) {
	book := bookWith("a", "The Hobbit", "Tolkien", 1, time.Now())
	book.Narrator = "Andy Serkis"
	book.Album = "Middle Earth"
	book.Chapters[0].Title = "An Unexpected Party"

	cases := map[string]bool{
		"hobbit":     true,
		"TOLKIEN":    true,
		"serkis":     true,
		"earth":      true,
		"unexpected": true,
		"dune":       false,
	}
	for query, want := range cases {
		if got := matchesQuery(book, query); got != want {
			t.Errorf("matchesQuery(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestSearchNarrowsVisibleBooks(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 1)
	addTestBook(t, lib, "Beta", 1)

	app.refreshLibrary()
	if len(app.visibleBooks) != 2 {
		t.Fatalf("visible = %d, want 2", len(app.visibleBooks))
	}

	app.search.SetText("alpha")
	app.refreshLibrary()
	if len(app.visibleBooks) != 1 {
		t.Fatalf("visible = %d, want 1", len(app.visibleBooks))
	}
	if app.visibleBooks[0].Title != "Book Alpha" {
		t.Fatalf("got %q", app.visibleBooks[0].Title)
	}

	app.search.SetText("")
	app.refreshLibrary()
	if len(app.visibleBooks) != 2 {
		t.Fatalf("clearing the query should restore all books, got %d", len(app.visibleBooks))
	}
}

func TestSelectionClearsWhenFilteredOut(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	alpha := addTestBook(t, lib, "Alpha", 1)
	addTestBook(t, lib, "Beta", 1)

	app.refreshLibrary()
	app.selectedBookID = alpha.ID
	app.refreshDetails()
	if app.selectedBook() == nil {
		t.Fatal("expected the book to be selected")
	}

	app.search.SetText("beta")
	app.refreshLibrary()
	if app.selectedBookID != "" {
		t.Fatalf("selection should be cleared, got %q", app.selectedBookID)
	}
}

func TestRefreshDetailsRendersSelectedBook(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 2)

	app.selectedBookID = book.ID
	app.refreshDetails()
	if app.details == nil {
		t.Fatal("details container was not created")
	}
	if len(app.details.Objects) != 1 {
		t.Fatalf("details has %d objects, want 1", len(app.details.Objects))
	}

	// With no selection the panel shows its placeholder.
	app.selectedBookID = ""
	app.refreshDetails()
	if len(app.details.Objects) != 1 {
		t.Fatalf("placeholder details has %d objects", len(app.details.Objects))
	}
}

func TestBookDetailsShowsProgressAndActions(t *testing.T) {
	app, _, lib, prog := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 3)
	_ = prog.Set(book.ID, progress.Entry{ChapterIndex: 1, PositionMS: 30_000, Percent: 0.5, PlayCount: 2})

	obj := app.bookDetails(book)
	if obj == nil {
		t.Fatal("bookDetails returned nil")
	}
	// A nil book must render the placeholder rather than panicking.
	if app.bookDetails(nil) == nil {
		t.Fatal("bookDetails(nil) returned nil")
	}
}

func TestEditMetadataUpdatesControllerAndStore(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 1)

	if err := app.ctrl.UpdateMetadata(book.ID, library.Metadata{Title: "Renamed", Author: "New Author"}); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	got, err := app.ctrl.Book(book.ID)
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if got.Title != "Renamed" || got.Author != "New Author" {
		t.Fatalf("got %+v", got)
	}
	// Progress must survive a metadata edit.
	_ = app.ctrl.progress.Set(book.ID, progress.Entry{Percent: 0.25})
	if e, ok := app.ctrl.Entry(book.ID); !ok || e.Percent != 0.25 {
		t.Fatal("progress should be preserved across an edit")
	}
}

func TestMoveChapterReloadsPlayingBook(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 3)
	if err := app.ctrl.Load(book.ID); err != nil {
		t.Fatalf("Load: %v", err)
	}
	before := app.ctrl.Player().TrackPath(0)

	if err := app.ctrl.MoveChapter(book.ID, 0, 2); err != nil {
		t.Fatalf("MoveChapter: %v", err)
	}
	// The player queue should follow the new chapter order.
	if got := app.ctrl.Player().TrackPath(2); got != before {
		t.Fatalf("player queue did not follow the reorder: %q at index 2, want %q", got, before)
	}
}

func TestMergeBooksCarriesProgress(t *testing.T) {
	app, _, lib, prog := newTestApp(t)
	target := addTestBook(t, lib, "Target", 2)
	source := addTestBook(t, lib, "Source", 2)

	_ = prog.Set(source.ID, progress.Entry{ChapterIndex: 1, PositionMS: 30_000, Percent: 0.5, PlayCount: 3})

	if err := app.ctrl.MergeBooks(target.ID, source.ID); err != nil {
		t.Fatalf("MergeBooks: %v", err)
	}

	merged, err := app.ctrl.Book(target.ID)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if len(merged.Chapters) != 4 {
		t.Fatalf("merged book has %d chapters, want 4", len(merged.Chapters))
	}
	if _, err := app.ctrl.Book(source.ID); err == nil {
		t.Fatal("source book should be gone")
	}
	if _, ok := app.ctrl.Entry(source.ID); ok {
		t.Fatal("source progress should be removed")
	}

	entry, ok := app.ctrl.Entry(target.ID)
	if !ok {
		t.Fatal("progress should have moved to the target")
	}
	// The source's chapter 2 is now the target's chapter 4.
	if entry.ChapterIndex != 3 {
		t.Fatalf("chapter index = %d, want 3", entry.ChapterIndex)
	}
	if entry.PositionMS != 30_000 {
		t.Fatalf("position = %d, want 30000", entry.PositionMS)
	}
}

func TestMergeBooksRejectsSelfMerge(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 1)
	if err := app.ctrl.MergeBooks(book.ID, book.ID); err == nil {
		t.Fatal("expected an error merging a book into itself")
	}
}

func TestExportImportThroughController(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 2)

	path := filepath.Join(t.TempDir(), "export.json")
	if err := app.ctrl.ExportLibrary(path); err != nil {
		t.Fatalf("ExportLibrary: %v", err)
	}
	added, merged, err := app.ctrl.ImportLibrary(path)
	if err != nil {
		t.Fatalf("ImportLibrary: %v", err)
	}
	if added != 0 || merged != 1 {
		t.Fatalf("added=%d merged=%d, want 0 and 1", added, merged)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestManagementDialogsBuild(t *testing.T) {
	// The dialogs run outside the render loop in real use, so make sure
	// constructing them does not panic or depend on unresolved state.
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 3)
	addTestBook(t, lib, "Beta", 1)

	app.ShowEditMetadata(book)
	app.ShowChapterManager(book)
	app.ShowMergeDialog(book)

	// A book with no other book to merge into must not panic either.
	soloApp, _, soloLib, _ := newTestApp(t)
	only := addTestBook(t, soloLib, "Only", 1)
	soloApp.ShowMergeDialog(only)
}

func TestChapterRowCallbacks(t *testing.T) {
	// Chapter rows talk to the app through package-level hooks while the modal
	// dialog is open. Verify the hooks are wired and then cleared on close.
	newTestApp(t)
	book := &library.Book{
		ID: library.NewBookID(),
		Chapters: []library.Chapter{
			{Title: "One", Path: "/a/1.mp3", DurationMS: 1000},
			{Title: "Two", Path: "/a/2.mp3", DurationMS: 2000},
		},
	}

	var moved [2]int
	var renamed string
	var removed int
	chapterRowHooks = &chapterHooks{
		move:   func(from, to int) { moved = [2]int{from, to} },
		rename: func(index int, title string) { renamed = title },
		remove: func(index int) { removed = index },
	}
	defer func() { chapterRowHooks = nil }()

	row := newChapterRow()
	row.update(0, book.Chapters[0], 2)
	if row.label.Text != "1. One" {
		t.Fatalf("label = %q", row.label.Text)
	}
	if !row.up.Disabled() {
		t.Fatal("the first row's up button should be disabled")
	}
	if row.down.Disabled() {
		t.Fatal("the first row's down button should be enabled")
	}

	row.toggleRename()
	if !row.renaming {
		t.Fatal("expected rename mode")
	}
	row.entry.SetText("Renamed")
	row.commitRename()
	if renamed != "Renamed" {
		t.Fatalf("rename hook got %q", renamed)
	}

	row.down.OnTapped()
	if moved != [2]int{0, 1} {
		t.Fatalf("move hook got %v", moved)
	}
	row.del.OnTapped()
	if removed != 0 {
		t.Fatalf("remove hook got %d", removed)
	}
}

func TestChapterRowLastRowDisablesDown(t *testing.T) {
	newTestApp(t)
	row := newChapterRow()
	row.update(1, library.Chapter{Title: "Two", Path: "/a/2.mp3"}, 2)
	if !row.down.Disabled() {
		t.Fatal("the last row's down button should be disabled")
	}
	if row.up.Disabled() {
		t.Fatal("the last row's up button should be enabled")
	}
}
