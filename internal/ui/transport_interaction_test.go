package ui

import (
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"
)

func TestPlayButtonActuallyStartsAndStops(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 2)
	app.openBook(book)

	t.Logf("initial state = %v, button = %q", player.State(), app.playButton.Text)

	// Click the real button, as a user would.
	fynetest.Tap(app.playButton)
	t.Logf("after tap: state = %v, button = %q", player.State(), app.playButton.Text)
	if player.State() != 1 { // StatePlaying
		t.Fatalf("tap did not start playback: state = %v", player.State())
	}

	fynetest.Tap(app.playButton)
	t.Logf("after 2nd tap: state = %v, button = %q", player.State(), app.playButton.Text)
	if player.State() == 1 {
		t.Fatal("second tap did not pause")
	}
}

func TestNextPrevButtonsChangeChapter(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 3)
	app.openBook(book)

	fynetest.Tap(app.nextButton)
	t.Logf("after next: track = %d", player.Position().Track)
	if player.Position().Track != 1 {
		t.Fatalf("next did not advance: track = %d", player.Position().Track)
	}

	fynetest.Tap(app.prevButton)
	t.Logf("after prev: track = %d", player.Position().Track)
	if player.Position().Track != 0 {
		t.Fatalf("prev did not go back: track = %d", player.Position().Track)
	}
}

func TestSeekSliderSeekWorks(t *testing.T) {
	app, player, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 2)
	app.openBook(book)

	player.mu.Lock()
	player.duration = 100 * time.Second
	player.mu.Unlock()
	app.updateTransport()

	// Drag the slider to the middle and release.
	app.seekSlider.SetValue(0.5)
	app.seekSlider.OnChangeEnded(0.5)
	t.Logf("after seek: offset = %v", player.Position().Elapsed)
	if player.Position().Elapsed < 45*time.Second || player.Position().Elapsed > 55*time.Second {
		t.Fatalf("seek did not apply: %v", player.Position().Elapsed)
	}
}
