// Package ui wires the library, progress store, and audio player into the
// Becoute desktop application.
package ui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/foundev/becoute/internal/audio"
	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// Player is the audio backend the controller drives. It is an interface so the
// controller can be tested without a real audio device.
type Player interface {
	SetQueue(paths []string, startAt int, offset time.Duration) error
	Play(offset time.Duration) error
	Pause()
	Stop()
	Seek(d time.Duration)
	Next(offset time.Duration) bool
	Previous(offset time.Duration)
	SetTrack(index int, offset time.Duration)
	SetSpeed(speed float64)
	SetVolume(volume float64)
	Speed() float64
	Volume() float64
	Position() audio.Position
	State() audio.State
	TrackCount() int
	TrackPath(index int) string
	Drained() <-chan int
	Complete()
	Close()
}

// Prefs is the small slice of Fyne's preferences API the controller uses. It is
// an interface so tests can avoid touching the real settings store.
type Prefs interface {
	String(key string) string
	SetString(key, value string)
	Float(key string) float64
	FloatWithFallback(key string, fallback float64) float64
	SetFloat(key string, value float64)
	BoolWithFallback(key string, fallback bool) bool
	SetBool(key string, value bool)
}

// Controller owns the application state and the playback rules. It is the only
// place that writes progress, so the UI stays a thin layer over it.
type Controller struct {
	app   fyne.App
	prefs Prefs

	library  *library.Store
	progress *progress.Store
	player   Player
	dataDir  string

	mu           sync.Mutex
	currentID    string
	autoplay     bool
	saveInterval time.Duration
	lastSave     time.Time
}

// New builds a controller around the given stores and player.
func New(app fyne.App, lib *library.Store, prog *progress.Store, player Player) *Controller {
	return NewWithPrefs(app, app.Preferences(), lib, prog, player, "")
}

// NewWithPrefs is like New but takes an explicit preferences implementation and
// data directory, which keeps the controller testable.
func NewWithPrefs(app fyne.App, prefs Prefs, lib *library.Store, prog *progress.Store, player Player, dataDir string) *Controller {
	c := &Controller{
		app:          app,
		prefs:        prefs,
		library:      lib,
		progress:     prog,
		player:       player,
		dataDir:      dataDir,
		autoplay:     true,
		saveInterval: 5 * time.Second,
	}
	player.SetSpeed(c.Speed())
	player.SetVolume(c.Volume())
	return c
}

const (
	prefSpeed      = "playback.speed"
	prefVolume     = "playback.volume"
	prefAutoplay   = "playback.autoplayNext"
	prefLastBook   = "library.lastBook"
	prefLastVolume = "ui.libraryVolume"
)

// Speed returns the saved playback speed, defaulting to 1.0.
func (c *Controller) Speed() float64 {
	s := c.prefs.FloatWithFallback(prefSpeed, 1.0)
	if s <= 0 {
		return 1.0
	}
	return s
}

// SetSpeed changes and persists the playback speed.
func (c *Controller) SetSpeed(speed float64) {
	c.prefs.SetFloat(prefSpeed, speed)
	c.player.SetSpeed(speed)
}

// Volume returns the saved output volume, defaulting to 1.0.
func (c *Controller) Volume() float64 {
	return c.prefs.FloatWithFallback(prefVolume, 1.0)
}

// SetVolume changes and persists the output volume.
func (c *Controller) SetVolume(volume float64) {
	c.prefs.SetFloat(prefVolume, volume)
	c.player.SetVolume(volume)
}

// AutoplayNext reports whether the next chapter should start automatically.
func (c *Controller) AutoplayNext() bool {
	return c.prefs.BoolWithFallback(prefAutoplay, true)
}

// SetAutoplayNext persists the auto-advance preference.
func (c *Controller) SetAutoplayNext(v bool) {
	c.prefs.SetBool(prefAutoplay, v)
}

// Player returns the audio backend, for read-only state queries from the UI.
func (c *Controller) Player() Player { return c.player }

// Books returns the whole library.
func (c *Controller) Books() []*library.Book { return c.library.Books() }

// Book returns a single book by ID.
func (c *Controller) Book(id string) (*library.Book, error) { return c.library.Book(id) }

// Entry returns the saved progress for a book.
func (c *Controller) Entry(bookID string) (progress.Entry, bool) { return c.progress.Get(bookID) }

// CurrentBookID returns the ID of the book loaded in the player, or "".
func (c *Controller) CurrentBookID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentID
}

// CurrentBook returns the book currently loaded in the player, if any.
func (c *Controller) CurrentBook() *library.Book {
	id := c.CurrentBookID()
	if id == "" {
		return nil
	}
	b, err := c.library.Book(id)
	if err != nil {
		return nil
	}
	return b
}

// LastBook returns the most recently opened book, if any.
func (c *Controller) LastBook() *library.Book {
	id := c.prefs.String(prefLastBook)
	if id == "" {
		return nil
	}
	b, err := c.library.Book(id)
	if err != nil {
		return nil
	}
	return b
}

// ImportFile imports a single audio file as a book.
func (c *Controller) ImportFile(path string) (*library.Book, error) {
	return c.library.ImportFile(path)
}

// ImportDir imports every audio file in a directory as one or more books.
func (c *Controller) ImportDir(dir string) ([]*library.Book, error) {
	return c.library.ImportDir(dir)
}

// Search returns books matching a free-text query.
func (c *Controller) Search(query string) []*library.Book { return c.library.Search(query) }

// UpdateMetadata applies edited metadata, keeping the book's ID and progress.
func (c *Controller) UpdateMetadata(id string, meta library.Metadata) error {
	return c.library.UpdateMetadata(id, meta)
}

// UpdateChapterTitle renames one chapter.
func (c *Controller) UpdateChapterTitle(id string, index int, title string) error {
	return c.library.UpdateChapterTitle(id, index, title)
}

// MoveChapter reorders chapters within a book.
func (c *Controller) MoveChapter(id string, from, to int) error {
	if err := c.library.MoveChapter(id, from, to); err != nil {
		return err
	}
	return c.reloadIfPlaying(id)
}

// RemoveChapter drops a chapter, updating progress if the playing book changed.
func (c *Controller) RemoveChapter(id string, index int) error {
	if err := c.library.RemoveChapter(id, index); err != nil {
		return err
	}
	return c.reloadIfPlaying(id)
}

// RescanChapters refreshes a book's chapters from disk.
func (c *Controller) RescanChapters(id string) (int, error) {
	changed, err := c.library.RescanChapters(id)
	if err != nil {
		return changed, err
	}
	return changed, c.reloadIfPlaying(id)
}

// MergeBooks folds source into target and re-points progress at the target.
func (c *Controller) MergeBooks(targetID, sourceID string) error {
	if targetID == sourceID {
		return errors.New("cannot merge a book into itself")
	}
	source, err := c.library.Book(sourceID)
	if err != nil {
		return err
	}
	// Preserve the source's position by translating it into the target's
	// chapter list before the source is removed.
	entry, hadProgress := c.progress.Get(sourceID)
	if err := c.library.MergeBooks(targetID, sourceID); err != nil {
		return err
	}
	if hadProgress && !entry.Finished {
		target, err := c.library.Book(targetID)
		if err == nil {
			if ch, ok := source.ChapterAt(entry.ChapterIndex); ok {
				for i, tc := range target.Chapters {
					if tc.Path == ch.Path {
						entry.ChapterIndex = i
						break
					}
				}
			}
			_ = c.progress.Set(targetID, entry)
		}
	}
	if err := c.progress.Delete(sourceID); err != nil {
		return err
	}
	if c.CurrentBookID() == sourceID {
		_ = c.Load(targetID)
	}
	return nil
}

// ExportLibrary writes the library to a portable JSON file.
func (c *Controller) ExportLibrary(path string) error { return c.library.Export(path) }

// ImportLibrary merges a library export into the current library.
func (c *Controller) ImportLibrary(path string) (int, int, error) {
	return c.library.Import(path)
}

// CoverPath returns the cached cover art path for a book, extracting it on
// first use. It returns "" when the book has no cover art.
func (c *Controller) CoverPath(book *library.Book) string {
	path, err := library.EnsureCover(c.dataDir, book)
	if err != nil {
		return ""
	}
	return path
}

// reloadIfPlaying re-loads a book into the player after its chapters changed,
// so the player's queue matches the library. It keeps the saved position.
func (c *Controller) reloadIfPlaying(id string) error {
	if c.CurrentBookID() != id {
		return nil
	}
	return c.Load(id)
}

// RemoveBook deletes a book and its progress.
func (c *Controller) RemoveBook(id string) error {
	if c.CurrentBookID() == id {
		c.Stop()
	}
	if err := c.library.Remove(id); err != nil {
		return err
	}
	if err := c.progress.Delete(id); err != nil {
		return err
	}
	if c.prefs.String(prefLastBook) == id {
		c.prefs.SetString(prefLastBook, "")
	}
	return nil
}

// Load prepares a book for playback, seeking to its saved position.
func (c *Controller) Load(id string) error {
	book, err := c.library.Book(id)
	if err != nil {
		return err
	}
	if len(book.Chapters) == 0 {
		return errors.New("this book has no chapters")
	}

	entry, ok := c.progress.Get(id)
	if !ok {
		entry = progress.Entry{}
	}
	start := entry.ChapterIndex
	if start < 0 || start >= len(book.Chapters) {
		start = 0
	}
	// A finished book starts over from the first chapter.
	if entry.Finished {
		start = 0
		entry.PositionMS = 0
	}
	offset := time.Duration(entry.PositionMS) * time.Millisecond

	paths := make([]string, len(book.Chapters))
	for i, ch := range book.Chapters {
		paths[i] = ch.Path
	}
	if err := c.player.SetQueue(paths, start, offset); err != nil {
		return fmt.Errorf("opening %q: %w", book.Title, err)
	}

	c.mu.Lock()
	c.currentID = id
	c.mu.Unlock()
	c.prefs.SetString(prefLastBook, id)
	return nil
}

// Play starts or resumes playback of the loaded book.
func (c *Controller) Play() error {
	if c.CurrentBookID() == "" {
		book := c.LastBook()
		if book == nil {
			return errors.New("no book selected")
		}
		if err := c.Load(book.ID); err != nil {
			return err
		}
	}
	if err := c.player.Play(0); err != nil {
		return err
	}
	id := c.CurrentBookID()
	if e, ok := c.progress.Get(id); ok {
		if e.Finished {
			// Replaying a completed book starts it over.
			e = progress.Entry{}
		}
		e.PlayCount++
		_ = c.progress.Set(id, e)
	}
	return nil
}

// Pause pauses playback and saves progress.
func (c *Controller) Pause() {
	c.player.Pause()
	c.Save()
}

// TogglePlay pauses when playing and resumes otherwise.
func (c *Controller) TogglePlay() error {
	if c.player.State() == audio.StatePlaying {
		c.Pause()
		return nil
	}
	return c.Play()
}

// Stop stops playback and saves progress.
func (c *Controller) Stop() {
	c.player.Stop()
	c.Save()
}

// Seek moves within the current chapter.
func (c *Controller) Seek(d time.Duration) {
	c.player.Seek(d)
	c.Save()
}

// NextChapter advances to the next chapter, if there is one.
func (c *Controller) NextChapter() bool {
	if !c.player.Next(0) {
		return false
	}
	c.Save()
	return true
}

// PreviousChapter goes back a chapter, or rewinds the current one.
func (c *Controller) PreviousChapter() {
	c.player.Previous(0)
	c.Save()
}

// SelectChapter jumps to a chapter by index.
func (c *Controller) SelectChapter(index int) {
	c.player.SetTrack(index, 0)
	c.Save()
}

// Save writes the current playback position for the loaded book.
func (c *Controller) Save() {
	id := c.CurrentBookID()
	if id == "" {
		return
	}
	book, err := c.library.Book(id)
	if err != nil {
		return
	}
	pos := c.player.Position()

	total := book.TotalDuration().Milliseconds()
	var startMS int64
	for i := 0; i < pos.Track && i < len(book.Chapters); i++ {
		startMS += book.Chapters[i].DurationMS
	}
	var chapterMS int64
	if ch, ok := book.ChapterAt(pos.Track); ok {
		chapterMS = ch.DurationMS
	}

	entry := progress.Entry{
		ChapterIndex: pos.Track,
		PositionMS:   pos.Elapsed.Milliseconds(),
		DurationMS:   chapterMS,
		Percent:      progress.Percent(pos.Track, pos.Elapsed.Milliseconds(), startMS, total),
	}
	if prev, ok := c.progress.Get(id); ok {
		entry.PlayCount = prev.PlayCount
		entry.Finished = prev.Finished
	}
	if total > 0 && entry.Percent >= 0.999 {
		entry.Finished = true
	}
	_ = c.progress.Set(id, entry)
	c.mu.Lock()
	c.lastSave = time.Now()
	c.mu.Unlock()
}

// SaveThrottled saves at most once per saveInterval, for use from a polling
// loop.
func (c *Controller) SaveThrottled() {
	c.mu.Lock()
	due := time.Since(c.lastSave) >= c.saveInterval
	c.mu.Unlock()
	if due {
		c.Save()
	}
}

// HandleDrained is called when the player reports that a chapter finished. It
// advances to the next chapter, starting it when auto-advance is on, and marks
// the book finished once the last chapter is done.
func (c *Controller) HandleDrained() {
	id := c.CurrentBookID()
	if id == "" {
		return
	}
	book, err := c.library.Book(id)
	if err != nil {
		return
	}

	if c.player.Next(0) {
		if c.AutoplayNext() {
			if err := c.player.Play(0); err != nil {
				c.player.Complete()
			}
		}
		c.Save()
		return
	}

	// The last chapter finished.
	c.player.Complete()
	c.markFinished(book)
}

// markFinished records that a book has been listened to in full.
func (c *Controller) markFinished(book *library.Book) {
	entry := progress.Entry{Finished: true, Percent: 1}
	if prev, ok := c.progress.Get(book.ID); ok {
		entry.PlayCount = prev.PlayCount
	}
	if last := len(book.Chapters) - 1; last >= 0 {
		entry.ChapterIndex = last
		entry.PositionMS = book.Chapters[last].DurationMS
		entry.DurationMS = book.Chapters[last].DurationMS
	}
	_ = c.progress.Set(book.ID, entry)
}

// ContinueListening returns books with saved progress that are not finished,
// most recently played first.
func (c *Controller) ContinueListening() []*library.Book {
	var out []*library.Book
	for _, b := range c.library.Books() {
		e, ok := c.progress.Get(b.ID)
		if !ok || e.Finished || e.Percent <= 0 {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		ei, _ := c.progress.Get(out[i].ID)
		ej, _ := c.progress.Get(out[j].ID)
		return ei.UpdatedAt.After(ej.UpdatedAt)
	})
	return out
}

// Finished returns finished books, most recently finished first.
func (c *Controller) Finished() []*library.Book {
	var out []*library.Book
	for _, b := range c.library.Books() {
		if e, ok := c.progress.Get(b.ID); ok && e.Finished {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ei, _ := c.progress.Get(out[i].ID)
		ej, _ := c.progress.Get(out[j].ID)
		return ei.UpdatedAt.After(ej.UpdatedAt)
	})
	return out
}

// RecentBooks returns the most recently added books.
func (c *Controller) RecentBooks(limit int) []*library.Book {
	books := c.library.Books()
	if limit > 0 && len(books) > limit {
		books = books[:limit]
	}
	return books
}

// FormatDuration renders a duration as h:mm:ss or m:ss.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d.Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ChapterLabel returns a human-readable name for a chapter.
func ChapterLabel(index int, ch library.Chapter) string {
	if strings.TrimSpace(ch.Title) != "" {
		return ch.Title
	}
	return fmt.Sprintf("Chapter %d", index+1)
}
