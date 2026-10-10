package ui

import (
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"

	"github.com/foundev/becoute/internal/audio"
	"github.com/foundev/becoute/internal/library"
)

// TestClickingLibraryRowLoadsBook walks the exact path a user takes: pick a
// book from the library, then go to the player and press play.
func TestClickingLibraryRowLoadsBook(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 2)
	app.refreshLibrary()

	// Click the first row in the library list.
	app.libraryList.Select(0)
	t.Logf("after row click: loadedBookID=%q playerTracks=%d",
		app.loadedBookID, player.TrackCount())

	if player.TrackCount() == 0 {
		t.Fatal("clicking a library row did not load the book into the player")
	}

	// Now the play button should work.
	app.refreshNowPlaying()
	t.Logf("play button disabled = %v, text = %q", app.playButton.Disabled(), app.playButton.Text)
	if app.playButton.Disabled() {
		t.Fatal("play button is disabled after selecting a book")
	}

	fynetest.Tap(app.playButton)
	t.Logf("after tap: state = %v", player.State())
	if player.State() != 1 {
		t.Fatalf("play did not start: state = %v", player.State())
	}
}

func TestTransportBarIsAlwaysAvailable(t *testing.T) {
	// The transport must be reachable from the Library tab, not only from Now
	// Playing, because that is where the listener picks a book.
	app, player, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 2)
	app.refreshLibrary()

	// Nothing is loaded yet, so the transport reports that and disables the
	// chapter buttons rather than pretending to work.
	app.refreshTransportBar()
	if app.barTitle.Text != "Nothing loaded" {
		t.Fatalf("bar title = %q", app.barTitle.Text)
	}
	if !app.barPrev.Disabled() || !app.barNext.Disabled() {
		t.Fatal("chapter buttons should be disabled with nothing loaded")
	}

	// Selecting a book loads it and lights up the transport.
	app.libraryList.Select(0)
	app.refreshTransportBar()
	if app.barTitle.Text != book.Title {
		t.Fatalf("bar title = %q, want %q", app.barTitle.Text, book.Title)
	}
	if app.barPrev.Disabled() || app.barNext.Disabled() {
		t.Fatal("chapter buttons should be enabled once a book is loaded")
	}

	// The bar's play button drives playback.
	fynetest.Tap(app.barPlay)
	if player.State() != audio.StatePlaying {
		t.Fatalf("transport bar play did not start playback: %v", player.State())
	}
	app.refreshTransportBar()
	t.Logf("bar elapsed = %q total = %q", app.barElapsed.Text, app.barTotal.Text)

	fynetest.Tap(app.barPlay)
	if player.State() == audio.StatePlaying {
		t.Fatal("transport bar play did not pause")
	}
}

func TestTransportBarNextPrev(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 3)
	app.refreshLibrary()
	app.libraryList.Select(0)

	fynetest.Tap(app.barNext)
	if got := player.Position().Track; got != 1 {
		t.Fatalf("bar next did not advance: track = %d", got)
	}
	fynetest.Tap(app.barPrev)
	if got := player.Position().Track; got != 0 {
		t.Fatalf("bar prev did not go back: track = %d", got)
	}
}

func TestTransportBarSeek(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 1)
	app.refreshLibrary()
	app.libraryList.Select(0)

	player.mu.Lock()
	player.duration = 100 * time.Second
	player.mu.Unlock()

	app.barSeek.SetValue(0.25)
	app.barSeek.OnChangeEnded(0.25)
	if got := player.Position().Elapsed; got < 20*time.Second || got > 30*time.Second {
		t.Fatalf("bar seek did not apply: %v", got)
	}
}

func TestTransportButtonsDoNotDependOnTheEventLoop(t *testing.T) {
	// Regression test: button handlers must update widgets directly rather than
	// going through fyne.Do. fyne.Do drops the function when no event loop is
	// running, which is the case in tests, so wrapping handlers in it made the
	// transport buttons silently do nothing.
	app, player, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 3)
	app.refreshLibrary()
	app.libraryList.Select(0)

	player.SetTrack(1, 0)
	fynetest.Tap(app.barPrev)
	if got := player.Position().Track; got != 0 {
		t.Fatalf("prev did nothing: track = %d", got)
	}

	fynetest.Tap(app.barNext)
	if got := player.Position().Track; got != 1 {
		t.Fatalf("next did nothing: track = %d", got)
	}
}

func TestNowPlayingButtonsWork(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	addTestBook(t, lib, "Alpha", 3)
	app.openBook(book0(t, lib))

	fynetest.Tap(app.nextButton)
	if got := player.Position().Track; got != 1 {
		t.Fatalf("now-playing next did nothing: track = %d", got)
	}
	fynetest.Tap(app.prevButton)
	if got := player.Position().Track; got != 0 {
		t.Fatalf("now-playing prev did nothing: track = %d", got)
	}
}

// book0 returns the first book in the library.
func book0(t *testing.T, lib *library.Store) *library.Book {
	t.Helper()
	books := lib.Books()
	if len(books) == 0 {
		t.Fatal("no books in library")
	}
	return books[0]
}
