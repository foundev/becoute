package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"

	"github.com/foundev/becoute/internal/audio"
	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// TestRealPlayerDrivesUI wires the real audio engine to the real UI and checks
// that the transport display actually moves during playback.
func TestRealPlayerDrivesUI(t *testing.T) {
	if os.Getenv("BECOUTE_AUDIO_IT") == "" {
		t.Skip("set BECOUTE_AUDIO_IT=1 to run the real audio integration test")
	}
	if audio.RaceDetectorEnabled() {
		t.Skip("skipping real audio under -race: the oto driver's own startup races (see RaceDetectorEnabled)")
	}
	if err := audio.InitSpeaker(); err != nil {
		t.Skipf("no audio device: %v", err)
	}

	dir := t.TempDir()
	mp3 := writeSilentMP3(t, dir, 1150) // ~30s

	fyneApp := fynetest.NewApp()
	lib, err := library.OpenWithDataDir(filepath.Join(dir, "library.json"), dir)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := progress.Open(filepath.Join(dir, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	book := &library.Book{ID: library.NewBookID(), Title: "Real", Author: "A"}
	book.Chapters = []library.Chapter{{Path: mp3, Title: "One", DurationMS: 30_000}}
	if err := lib.Add(book); err != nil {
		t.Fatal(err)
	}

	player := audio.NewPlayer()
	defer player.Close()
	ctrl := NewWithPrefs(fyneApp, newMemPrefs(), lib, prog, player, dir)
	app := NewApp(fyneApp, ctrl)

	app.openBook(book)
	t.Logf("after load: label=%q state=%v", app.elapsedLabel.Text, player.State())

	fynetest.Tap(app.playButton)
	if player.State() != audio.StatePlaying {
		t.Fatalf("play did not start: %v", player.State())
	}

	// Let the ticker run, then check the UI reflects progress.
	deadline := time.Now().Add(4 * time.Second)
	var sawLabel string
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		app.updateTransport()
		sawLabel = app.elapsedLabel.Text
		if player.Position().Elapsed > 1200*time.Millisecond {
			break
		}
	}
	t.Logf("player elapsed=%v, UI label=%q, slider=%v", player.Position().Elapsed, sawLabel, app.seekSlider.Value)

	if player.Position().Elapsed < time.Second {
		// The device can open on a headless machine while nothing actually
		// gets played, which is the case on CI runners without audio output.
		// Skip rather than report a false failure; run this test on a machine
		// with working audio to exercise real playback.
		t.Skipf("no samples were played (elapsed %v); audio output unavailable here", player.Position().Elapsed)
	}
	if sawLabel == "0:00" || sawLabel == "" {
		t.Fatalf("UI elapsed label never updated: %q", sawLabel)
	}
	if app.seekSlider.Value <= 0 {
		t.Fatalf("seek slider never moved: %v", app.seekSlider.Value)
	}
}

// writeSilentMP3 writes valid silent MPEG frames to a file.
func writeSilentMP3(t *testing.T, dir string, frames int) string {
	t.Helper()
	const frameSize = 417
	buf := make([]byte, 0, frameSize*frames)
	for i := 0; i < frames; i++ {
		f := make([]byte, frameSize)
		f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0x00
		buf = append(buf, f...)
	}
	p := filepath.Join(dir, "real.mp3")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
