package ui

import (
	"path/filepath"
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"

	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// mustOpenProgress returns an empty progress store for a test.
func mustOpenProgress(t *testing.T) *progress.Store {
	t.Helper()
	store, err := progress.Open(filepath.Join(t.TempDir(), "progress.json"))
	if err != nil {
		t.Fatalf("progress.Open: %v", err)
	}
	return store
}

// newTestApp builds the real window on Fyne's headless test driver.
func newTestApp(t *testing.T) (*App, *fakePlayer, *library.Store, *progress.Store) {
	t.Helper()
	fyneApp := fynetest.NewApp()
	prefs := newMemPrefs()
	_, player, lib, _ := newTestController(t)
	prog := mustOpenProgress(t)
	ctrl := NewWithPrefs(fyneApp, prefs, lib, prog, player, t.TempDir())
	// The background ticker is disabled in tests: Fyne's test driver runs
	// fyne.Do inline, which would let it touch widgets concurrently with the
	// test goroutine. Tests call the refresh helpers directly.
	app := NewAppWithOptions(fyneApp, ctrl, Options{DisableTicker: true})
	return app, player, lib, prog
}

func TestNewAppBuildsWithoutPanic(t *testing.T) {
	// Regression test: SetSelected fires its handler synchronously, so the
	// library tab must create its labels before selecting a filter.
	app, _, _, _ := newTestApp(t)
	if app.win == nil {
		t.Fatal("window was not created")
	}
	if app.win.Content() == nil {
		t.Fatal("window has no content")
	}
}

func TestRefreshLibraryListsImportedBooks(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	addTestBook(t, lib, "b1", 2)

	app.refreshLibrary()
	if len(app.visibleBooks) != 1 {
		t.Fatalf("visible books = %d, want 1", len(app.visibleBooks))
	}

	app.filter.SetSelected("Not started")
	if len(app.visibleBooks) != 1 {
		t.Fatalf("not started books = %d, want 1", len(app.visibleBooks))
	}

	app.filter.SetSelected("Finished")
	if len(app.visibleBooks) != 0 {
		t.Fatalf("finished books = %d, want 0", len(app.visibleBooks))
	}
}

func TestOpenBookPopulatesNowPlaying(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "b1", 3)

	app.openBook(book)

	if app.loadedBookID != book.ID {
		t.Fatalf("loadedBookID = %q", app.loadedBookID)
	}
	if got := app.nowTitle.Text; got != book.Title {
		t.Fatalf("now title = %q, want %q", got, book.Title)
	}
	if len(app.chapterSelect.Options) != 3 {
		t.Fatalf("chapter options = %d, want 3", len(app.chapterSelect.Options))
	}
	if player.TrackCount() != 3 {
		t.Fatalf("player tracks = %d, want 3", player.TrackCount())
	}
}

func TestRefreshNowPlayingWithNoBookDisablesTransport(t *testing.T) {
	app, _, _, _ := newTestApp(t)
	app.refreshNowPlaying()

	if !app.playButton.Disabled() {
		t.Fatal("play button should be disabled with no book loaded")
	}
	if !app.seekSlider.Disabled() {
		t.Fatal("seek slider should be disabled with no book loaded")
	}
	if app.nowTitle.Text != "Nothing loaded" {
		t.Fatalf("now title = %q", app.nowTitle.Text)
	}
}

func TestUpdateTransportReflectsPosition(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "b1", 2)
	app.openBook(book)

	player.mu.Lock()
	player.duration = 60 * time.Second
	player.offset = 15 * time.Second
	player.mu.Unlock()

	app.updateTransport()
	if got := app.elapsedLabel.Text; got != "0:15" {
		t.Fatalf("elapsed = %q, want 0:15", got)
	}
	if got := app.totalLabel.Text; got != "1:00" {
		t.Fatalf("total = %q, want 1:00", got)
	}
	if app.seekSlider.Value < 0.24 || app.seekSlider.Value > 0.26 {
		t.Fatalf("seek value = %v, want ~0.25", app.seekSlider.Value)
	}
}

func TestPlayButtonIconFollowsState(t *testing.T) {
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "b1", 1)
	app.openBook(book)

	app.updatePlayButton()
	if app.playButton.Text != "Play" {
		t.Fatalf("button text = %q, want Play", app.playButton.Text)
	}

	if err := app.ctrl.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	app.updatePlayButton()
	if app.playButton.Text != "Pause" {
		t.Fatalf("button text = %q, want Pause", app.playButton.Text)
	}
}

func TestTabsContainLibraryAndPlayer(t *testing.T) {
	app, _, _, _ := newTestApp(t)
	tabs := app.tabs
	if tabs == nil {
		t.Fatal("no tab container was built")
	}
	if len(tabs.Items) != 2 {
		t.Fatalf("tabs = %d, want 2", len(tabs.Items))
	}
	if tabs.Items[0].Text != "Library" || tabs.Items[1].Text != "Now Playing" {
		t.Fatalf("unexpected tabs: %q, %q", tabs.Items[0].Text, tabs.Items[1].Text)
	}
}
