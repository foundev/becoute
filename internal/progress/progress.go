// Package progress tracks and persists how far the listener has gotten in each
// book. Progress is stored as JSON keyed by book ID so it can be written
// frequently without rewriting the whole library.
package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is the saved listening position for a single book.
type Entry struct {
	// ChapterIndex is the zero-based index of the chapter being listened to.
	ChapterIndex int `json:"chapter_index"`
	// PositionMS is the playback offset within that chapter, in milliseconds.
	PositionMS int64 `json:"position_ms"`
	// DurationMS is the known length of the current chapter, in milliseconds.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Percent is how far through the whole book the listener is, 0 to 1.
	Percent float64 `json:"percent"`
	// Finished is set once the last chapter completes.
	Finished bool `json:"finished"`
	// PlayCount is the number of times playback of this book was started.
	PlayCount int `json:"play_count"`
	// UpdatedAt is when this entry was last written.
	UpdatedAt time.Time `json:"updated_at"`
}

// Store holds progress for every book and persists it to disk.
type Store struct {
	mu      sync.RWMutex
	path    string
	entries map[string]Entry
}

// Open loads the progress store from path, creating an empty one when the file
// does not exist yet.
func Open(path string) (*Store, error) {
	s := &Store{path: path, entries: map[string]Entry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.entries); err != nil {
		return nil, err
	}
	if s.entries == nil {
		s.entries = map[string]Entry{}
	}
	return s, nil
}

// Get returns the saved entry for a book. The second return value is false when
// the book has no saved progress.
func (s *Store) Get(bookID string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[bookID]
	return e, ok
}

// Set stores an entry and writes the whole store to disk.
func (s *Store) Set(bookID string, e Entry) error {
	s.mu.Lock()
	e.UpdatedAt = time.Now().UTC()
	s.entries[bookID] = e
	s.mu.Unlock()
	return s.save()
}

// Delete removes a book's progress and writes the store to disk.
func (s *Store) Delete(bookID string) error {
	s.mu.Lock()
	delete(s.entries, bookID)
	s.mu.Unlock()
	return s.save()
}

// All returns a copy of every entry, keyed by book ID.
func (s *Store) All() map[string]Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Entry, len(s.entries))
	for k, v := range s.entries {
		out[k] = v
	}
	return out
}

// save writes the store atomically: it writes a temporary file in the same
// directory and renames it over the destination.
func (s *Store) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.entries, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Percent computes how far through a book a position is. totalMS is the summed
// duration of all chapters; currentStartMS is the summed duration of every
// chapter before chapterIndex.
func Percent(chapterIndex int, positionMS, currentStartMS, totalMS int64) float64 {
	if totalMS <= 0 {
		return 0
	}
	elapsed := currentStartMS + positionMS
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > totalMS {
		elapsed = totalMS
	}
	return float64(elapsed) / float64(totalMS)
}
