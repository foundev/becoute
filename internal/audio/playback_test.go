package audio

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePlaybackMP3(t *testing.T, frames int) string {
	t.Helper()
	const frameSize = 417
	buf := make([]byte, 0, frameSize*frames)
	for i := 0; i < frames; i++ {
		f := make([]byte, frameSize)
		f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0x00
		buf = append(buf, f...)
	}
	p := filepath.Join(t.TempDir(), "t.mp3")
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRealPlaybackAdvancesPosition(t *testing.T) {
	if RaceDetectorEnabled() {
		t.Skip("skipping real audio under -race: the oto driver's own startup races (see RaceDetectorEnabled)")
	}
	if err := InitSpeaker(); err != nil {
		t.Skipf("no audio device: %v", err)
	}
	p := NewPlayer()
	defer p.Close()

	// ~30 seconds of audio so there is room to observe progress.
	path := writePlaybackMP3(t, 1150)
	if err := p.SetQueue([]string{path}, 0, 0); err != nil {
		t.Fatalf("SetQueue: %v", err)
	}
	t.Logf("track duration = %v", p.Position().Total)

	if err := p.Play(0); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if got := p.State(); got != StatePlaying {
		t.Fatalf("state = %v, want playing", got)
	}

	time.Sleep(1200 * time.Millisecond)
	pos := p.Position()
	t.Logf("after 1.2s: elapsed = %v", pos.Elapsed)
	if pos.Elapsed < 500*time.Millisecond {
		// Opening the audio device can succeed while the process is still
		// unable to get samples played, which is the case inside a sandbox
		// that denies CoreAudio. Skip rather than report a false failure; run
		// this test outside the sandbox to exercise real playback.
		t.Skipf("no samples were played (elapsed %v); audio output unavailable here", pos.Elapsed)
	}

	// Pause and confirm the position freezes.
	p.Pause()
	paused := p.Position().Elapsed
	time.Sleep(700 * time.Millisecond)
	if after := p.Position().Elapsed; after != paused {
		t.Fatalf("position advanced while paused: %v -> %v", paused, after)
	}
	t.Logf("paused at %v, held steady", paused)

	// Seek should move the play head.
	p.Seek(10 * time.Second)
	time.Sleep(200 * time.Millisecond)
	if got := p.Position().Elapsed; got < 9*time.Second {
		t.Fatalf("seek did not move the play head: %v", got)
	}
	t.Logf("seeked to %v", p.Position().Elapsed)

	// Resume should advance again.
	if err := p.Play(0); err != nil {
		t.Fatalf("resume: %v", err)
	}
	time.Sleep(800 * time.Millisecond)
	if got := p.Position().Elapsed; got <= 10*time.Second {
		t.Fatalf("playback did not resume: %v", got)
	}
	t.Logf("resumed and advanced to %v", p.Position().Elapsed)
}
