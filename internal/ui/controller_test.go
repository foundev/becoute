package ui

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"

	"github.com/foundev/becoute/internal/audio"
	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// fakePlayer is a Player that records calls instead of producing sound.
type fakePlayer struct {
	mu       sync.Mutex
	paths    []string
	track    int
	offset   time.Duration
	state    audio.State
	speed    float64
	volume   float64
	duration time.Duration
	drained  chan int
}

func newFakePlayer() *fakePlayer {
	return &fakePlayer{speed: 1, volume: 1, drained: make(chan int, 4)}
}

func (f *fakePlayer) SetQueue(paths []string, startAt int, offset time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = paths
	f.track = startAt
	f.offset = offset
	f.state = audio.StateStopped
	return nil
}
func (f *fakePlayer) Play(offset time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offset = offset
	f.state = audio.StatePlaying
	return nil
}
func (f *fakePlayer) Pause()               { f.mu.Lock(); f.state = audio.StatePaused; f.mu.Unlock() }
func (f *fakePlayer) Stop()                { f.mu.Lock(); f.state = audio.StateStopped; f.offset = 0; f.mu.Unlock() }
func (f *fakePlayer) Seek(d time.Duration) { f.mu.Lock(); f.offset = d; f.mu.Unlock() }
func (f *fakePlayer) Next(offset time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.track+1 >= len(f.paths) {
		return false
	}
	f.track++
	f.offset = offset
	f.state = audio.StateStopped
	return true
}
func (f *fakePlayer) Previous(offset time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.track > 0 {
		f.track--
	}
	f.offset = offset
	f.state = audio.StateStopped
}
func (f *fakePlayer) SetTrack(index int, offset time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.track = index
	f.offset = offset
	f.state = audio.StateStopped
}
func (f *fakePlayer) SetSpeed(s float64)  { f.mu.Lock(); f.speed = s; f.mu.Unlock() }
func (f *fakePlayer) SetVolume(v float64) { f.mu.Lock(); f.volume = v; f.mu.Unlock() }
func (f *fakePlayer) Speed() float64      { f.mu.Lock(); defer f.mu.Unlock(); return f.speed }
func (f *fakePlayer) Volume() float64     { f.mu.Lock(); defer f.mu.Unlock(); return f.volume }
func (f *fakePlayer) Position() audio.Position {
	f.mu.Lock()
	defer f.mu.Unlock()
	return audio.Position{Track: f.track, Elapsed: f.offset, Total: f.duration}
}
func (f *fakePlayer) State() audio.State { f.mu.Lock(); defer f.mu.Unlock(); return f.state }
func (f *fakePlayer) TrackCount() int    { f.mu.Lock(); defer f.mu.Unlock(); return len(f.paths) }
func (f *fakePlayer) TrackPath(i int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.paths) {
		return ""
	}
	return f.paths[i]
}
func (f *fakePlayer) Drained() <-chan int { return f.drained }
func (f *fakePlayer) Complete()           { f.mu.Lock(); f.state = audio.StateStopped; f.mu.Unlock() }
func (f *fakePlayer) Close()              {}

func newTestController(t *testing.T) (*Controller, *fakePlayer, *library.Store, *progress.Store) {
	t.Helper()
	dir := t.TempDir()
	lib, err := library.Open(filepath.Join(dir, "library.json"))
	if err != nil {
		t.Fatalf("library.Open: %v", err)
	}
	prog, err := progress.Open(filepath.Join(dir, "progress.json"))
	if err != nil {
		t.Fatalf("progress.Open: %v", err)
	}
	player := newFakePlayer()
	var app fyne.App = fyneapp.NewWithID("io.github.foundev.becoute.test")
	return NewWithPrefs(app, newMemPrefs(), lib, prog, player, dir), player, lib, prog
}

// memPrefs is an in-memory Prefs implementation for tests.
type memPrefs struct {
	strings map[string]string
	floats  map[string]float64
	bools   map[string]bool
}

func newMemPrefs() *memPrefs {
	return &memPrefs{strings: map[string]string{}, floats: map[string]float64{}, bools: map[string]bool{}}
}

func (m *memPrefs) String(key string) string    { return m.strings[key] }
func (m *memPrefs) SetString(key, value string) { m.strings[key] = value }
func (m *memPrefs) Float(key string) float64    { return m.floats[key] }
func (m *memPrefs) FloatWithFallback(key string, fallback float64) float64 {
	if v, ok := m.floats[key]; ok {
		return v
	}
	return fallback
}
func (m *memPrefs) SetFloat(key string, value float64) { m.floats[key] = value }
func (m *memPrefs) BoolWithFallback(key string, fallback bool) bool {
	if v, ok := m.bools[key]; ok {
		return v
	}
	return fallback
}
func (m *memPrefs) SetBool(key string, value bool) { m.bools[key] = value }

func addTestBook(t *testing.T, lib *library.Store, id string, chapters int) *library.Book {
	t.Helper()
	b := &library.Book{ID: id, Title: "Book " + id, Author: "Author"}
	for i := 0; i < chapters; i++ {
		b.Chapters = append(b.Chapters, library.Chapter{
			Title:      "Chapter " + string(rune('1'+i)),
			Path:       "/tmp/" + id + "/" + string(rune('1'+i)) + ".mp3",
			DurationMS: 60_000,
		})
	}
	if err := lib.Add(b); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return b
}

func TestLoadResumesSavedPosition(t *testing.T) {
	ctrl, player, lib, prog := newTestController(t)
	book := addTestBook(t, lib, "b1", 3)
	if err := prog.Set(book.ID, progress.Entry{ChapterIndex: 1, PositionMS: 30_000, Percent: 0.5}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := ctrl.Load(book.ID); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if player.track != 1 {
		t.Fatalf("started at chapter %d, want 1", player.track)
	}
	if player.offset != 30*time.Second {
		t.Fatalf("offset = %v, want 30s", player.offset)
	}
	if got := ctrl.CurrentBookID(); got != book.ID {
		t.Fatalf("CurrentBookID = %q", got)
	}
}

func TestLoadFinishedBookStartsOver(t *testing.T) {
	ctrl, player, lib, prog := newTestController(t)
	book := addTestBook(t, lib, "b1", 3)
	if err := prog.Set(book.ID, progress.Entry{ChapterIndex: 2, PositionMS: 59_000, Percent: 1, Finished: true}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := ctrl.Load(book.ID); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if player.track != 0 || player.offset != 0 {
		t.Fatalf("finished book should restart, got track %d offset %v", player.track, player.offset)
	}
}

func TestSaveComputesPercent(t *testing.T) {
	ctrl, player, lib, _ := newTestController(t)
	book := addTestBook(t, lib, "b1", 4) // 4 x 60s = 240s
	if err := ctrl.Load(book.ID); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Pretend we are 30s into chapter 2 (index 1): 60+30 = 90 of 240 = 37.5%.
	player.SetTrack(1, 30*time.Second)
	ctrl.Save()

	entry, ok := ctrl.Entry(book.ID)
	if !ok {
		t.Fatal("no progress saved")
	}
	if entry.ChapterIndex != 1 || entry.PositionMS != 30_000 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Percent < 0.37 || entry.Percent > 0.38 {
		t.Fatalf("percent = %v, want ~0.375", entry.Percent)
	}
	if entry.Finished {
		t.Fatal("book should not be finished")
	}
}

func TestHandleDrainedAdvancesThenFinishes(t *testing.T) {
	ctrl, player, lib, _ := newTestController(t)
	book := addTestBook(t, lib, "b1", 2)
	if err := ctrl.Load(book.ID); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := ctrl.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}

	// First chapter drains: should move to chapter 2 and keep playing.
	ctrl.HandleDrained()
	if player.track != 1 {
		t.Fatalf("track = %d, want 1", player.track)
	}
	if player.State() != audio.StatePlaying {
		t.Fatalf("state = %v, want playing", player.State())
	}

	// Last chapter drains: book is marked finished.
	ctrl.HandleDrained()
	entry, ok := ctrl.Entry(book.ID)
	if !ok {
		t.Fatal("no progress saved")
	}
	if !entry.Finished {
		t.Fatalf("entry = %+v, want finished", entry)
	}
	if len(ctrl.Finished()) != 1 {
		t.Fatal("Finished() should list the book")
	}
}

func TestContinueListeningExcludesFinishedAndUnstarted(t *testing.T) {
	ctrl, _, lib, prog := newTestController(t)
	inProgress := addTestBook(t, lib, "b1", 2)
	finished := addTestBook(t, lib, "b2", 2)
	_ = addTestBook(t, lib, "b3", 2)

	_ = prog.Set(inProgress.ID, progress.Entry{ChapterIndex: 0, PositionMS: 1000, Percent: 0.1})
	_ = prog.Set(finished.ID, progress.Entry{ChapterIndex: 1, PositionMS: 60_000, Percent: 1, Finished: true})

	got := ctrl.ContinueListening()
	if len(got) != 1 || got[0].ID != inProgress.ID {
		t.Fatalf("ContinueListening = %+v", got)
	}
}

func TestRemoveBookClearsProgress(t *testing.T) {
	ctrl, _, lib, prog := newTestController(t)
	book := addTestBook(t, lib, "b1", 1)
	_ = prog.Set(book.ID, progress.Entry{Percent: 0.5})

	if err := ctrl.RemoveBook(book.ID); err != nil {
		t.Fatalf("RemoveBook: %v", err)
	}
	if lib.Len() != 0 {
		t.Fatal("book not removed")
	}
	if _, ok := prog.Get(book.ID); ok {
		t.Fatal("progress not removed")
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                  "0:00",
		90 * time.Second:   "1:30",
		3661 * time.Second: "1:01:01",
		-5 * time.Second:   "0:00",
	}
	for d, want := range cases {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
