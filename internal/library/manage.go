package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/foundev/becoute/internal/audio"
)

// Metadata holds the editable descriptive fields of a book.
type Metadata struct {
	Title    string
	Author   string
	Narrator string
	Album    string
}

// UpdateMetadata applies edited metadata to a book and persists the library.
// The book keeps its ID, so listening progress is preserved across an edit.
func (s *Store) UpdateMetadata(id string, meta Metadata) error {
	s.mu.Lock()
	b, ok := s.books[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	b.Title = strings.TrimSpace(meta.Title)
	b.Author = strings.TrimSpace(meta.Author)
	b.Narrator = strings.TrimSpace(meta.Narrator)
	b.Album = strings.TrimSpace(meta.Album)
	s.mu.Unlock()
	return s.save()
}

// UpdateChapterTitle renames a single chapter and persists the library.
func (s *Store) UpdateChapterTitle(id string, index int, title string) error {
	s.mu.Lock()
	b, ok := s.books[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	if index < 0 || index >= len(b.Chapters) {
		s.mu.Unlock()
		return fmt.Errorf("chapter %d is out of range", index)
	}
	b.Chapters[index].Title = strings.TrimSpace(title)
	s.mu.Unlock()
	return s.save()
}

// MoveChapter moves a chapter to a new position in the book.
func (s *Store) MoveChapter(id string, from, to int) error {
	s.mu.Lock()
	b, ok := s.books[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	if from < 0 || from >= len(b.Chapters) || to < 0 || to >= len(b.Chapters) {
		s.mu.Unlock()
		return fmt.Errorf("chapter move %d -> %d is out of range", from, to)
	}
	if from == to {
		s.mu.Unlock()
		return nil
	}
	ch := b.Chapters[from]
	rest := append(append([]Chapter{}, b.Chapters[:from]...), b.Chapters[from+1:]...)
	moved := append(rest[:to], append([]Chapter{ch}, rest[to:]...)...)
	b.Chapters = moved
	// The listener has expressed an order explicitly; stop re-sorting it.
	b.CustomOrder = true
	s.mu.Unlock()
	return s.save()
}

// RemoveChapter drops a chapter from a book. Removing the last chapter removes
// the book itself, since a book with no chapters cannot be played.
func (s *Store) RemoveChapter(id string, index int) error {
	s.mu.Lock()
	b, ok := s.books[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	if index < 0 || index >= len(b.Chapters) {
		s.mu.Unlock()
		return fmt.Errorf("chapter %d is out of range", index)
	}
	b.Chapters = append(b.Chapters[:index:index], b.Chapters[index+1:]...)
	empty := len(b.Chapters) == 0
	s.mu.Unlock()

	if empty {
		return s.Remove(id)
	}
	return s.save()
}

// RescanChapters re-reads a book's chapters from disk, refreshing titles and
// durations, and picks up any new audio files in the chapter's folder. It
// returns how many chapters were added or updated.
func (s *Store) RescanChapters(id string) (int, error) {
	b, err := s.Book(id)
	if err != nil {
		return 0, err
	}

	changed := 0
	seen := map[string]bool{}
	var chapters []Chapter
	for _, ch := range b.Chapters {
		if _, err := os.Stat(ch.Path); err != nil {
			// The file is gone; drop it from the book.
			changed++
			continue
		}
		fresh, err := ReadChapter(ch.Path)
		if err != nil {
			chapters = append(chapters, ch)
			continue
		}
		if fresh.DurationMS != ch.DurationMS || fresh.Title != ch.Title {
			changed++
		}
		// Keep a title the listener renamed by hand: only adopt the file's
		// title when it differs from the file name it was derived from.
		if ch.Title != "" && ch.Title != defaultChapterTitle(ch.Path) {
			fresh.Title = ch.Title
		}
		chapters = append(chapters, fresh)
		seen[ch.Path] = true
	}

	// Adopt new audio files that appeared in the same folders.
	dirs := map[string]bool{}
	for _, ch := range b.Chapters {
		dirs[filepath.Dir(ch.Path)] = true
	}
	album := b.Album
	for dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !audio.IsAudioFile(e.Name()) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if seen[p] {
				continue
			}
			if album != "" {
				if m, err := readTags(p); err != nil || m == nil || !strings.EqualFold(strings.TrimSpace(m.Album()), album) {
					continue
				}
			}
			ch, err := ReadChapter(p)
			if err != nil {
				continue
			}
			chapters = append(chapters, ch)
			seen[p] = true
			changed++
		}
	}

	if !b.CustomOrder {
		sort.Slice(chapters, func(i, j int) bool { return NaturalLess(chapters[i].Title, chapters[j].Title) })
	}

	// Assign onto the book held by the store, not onto the copy returned by
	// Book(), or the rescan would be silently discarded.
	s.mu.Lock()
	stored, ok := s.books[id]
	if !ok {
		s.mu.Unlock()
		return changed, ErrNotFound
	}
	stored.Chapters = chapters
	s.mu.Unlock()
	if err := s.save(); err != nil {
		return changed, err
	}
	if len(chapters) > 0 {
		s.cacheCover(id)
	}
	return changed, nil
}

// defaultChapterTitle is the title a chapter gets when it has no ID3 title.
func defaultChapterTitle(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// MergeBooks folds the source book into the target, appending its chapters and
// removing the source. Progress for the source is not carried over, because the
// caller is expected to reconcile progress itself.
func (s *Store) MergeBooks(targetID, sourceID string) error {
	if targetID == sourceID {
		return errors.New("cannot merge a book into itself")
	}
	s.mu.Lock()
	target, ok := s.books[targetID]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	source, ok := s.books[sourceID]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	for _, ch := range source.Chapters {
		if !target.HasChapter(ch.Path) {
			target.Chapters = append(target.Chapters, ch)
		}
	}
	target.SortChapters()
	if strings.TrimSpace(target.Author) == "" {
		target.Author = source.Author
	}
	delete(s.books, sourceID)
	s.mu.Unlock()
	return s.save()
}

// --- Export and import ---

// ExportFile is the portable representation of a library.
type ExportFile struct {
	Version int       `json:"version"`
	Books   []*Book   `json:"books"`
	SavedAt time.Time `json:"saved_at"`
}

// ExportVersion is the current export format version.
const ExportVersion = 1

// Export writes the library to path in a portable form.
func (s *Store) Export(path string) error {
	// Clone under the lock: marshalling the shared books afterwards would race
	// with a concurrent edit or import.
	s.mu.RLock()
	books := make([]*Book, 0, len(s.books))
	for _, b := range s.books {
		books = append(books, b.Clone())
	}
	s.mu.RUnlock()
	sort.Slice(books, func(i, j int) bool { return NaturalLess(books[i].Title, books[j].Title) })

	data, err := json.MarshalIndent(ExportFile{Version: ExportVersion, Books: books, SavedAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Import reads a library export and merges its books into the store. Books are
// matched by chapter paths so importing the same file twice is harmless. It
// returns the number of books added and merged.
func (s *Store) Import(path string) (added, merged int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	var file ExportFile
	if err := json.Unmarshal(data, &file); err != nil {
		return 0, 0, fmt.Errorf("not a Becoute library export: %w", err)
	}
	if file.Version > ExportVersion {
		return 0, 0, fmt.Errorf("library export version %d is newer than this build supports", file.Version)
	}

	for _, incoming := range file.Books {
		if incoming == nil || len(incoming.Chapters) == 0 {
			continue
		}
		if id, ok := s.findIDByChapterPaths(incoming.Chapters); ok {
			if _, err := s.mergeInto(id, incoming); err != nil {
				return added, merged, err
			}
			merged++
			continue
		}
		if incoming.ID == "" {
			incoming.ID = NewBookID()
		}
		// Reassign on the off chance the ID collides with a different book.
		if _, err := s.Book(incoming.ID); err == nil {
			incoming.ID = NewBookID()
		}
		if err := s.Add(incoming); err != nil {
			return added, merged, err
		}
		added++
	}
	return added, merged, nil
}

// Search returns books whose title, author, narrator, or chapter titles contain
// the query. An empty query matches every book.
func (s *Store) Search(query string) []*Book {
	books := s.Books()
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return books
	}
	var out []*Book
	for _, b := range books {
		if bookMatches(b, query) {
			out = append(out, b)
		}
	}
	return out
}

func bookMatches(b *Book, query string) bool {
	for _, field := range []string{b.Title, b.Author, b.Narrator, b.Album} {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	for _, ch := range b.Chapters {
		if strings.Contains(strings.ToLower(ch.Title), query) {
			return true
		}
	}
	return false
}
