package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/tag"

	"github.com/foundev/becoute/internal/audio"
)

// ErrNotFound is returned when a book ID is not present in the library.
var ErrNotFound = errors.New("book not found")

// Store is the in-memory library, persisted to a JSON file.
type Store struct {
	mu   sync.RWMutex
	path string
	// dataDir is where cached cover art lives. It is empty when cover caching
	// is unavailable, in which case covers are simply not stored.
	dataDir string
	books   map[string]*Book
}

// Open loads the library from path, creating an empty one when the file does
// not exist yet. Cover art is cached next to the library file.
func Open(path string) (*Store, error) {
	return OpenWithDataDir(path, filepath.Dir(path))
}

// OpenWithDataDir is like Open but caches cover art under dataDir.
func OpenWithDataDir(path, dataDir string) (*Store, error) {
	s := &Store{path: path, dataDir: dataDir, books: map[string]*Book{}}
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
	var books []*Book
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, err
	}
	for _, b := range books {
		if b == nil || b.ID == "" {
			continue
		}
		s.books[b.ID] = b
	}
	return s, nil
}

// Books returns every book, sorted with the most recently added first.
func (s *Store) Books() []*Book {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Book, 0, len(s.books))
	for _, b := range s.books {
		out = append(out, b.Clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	return out
}

// Book returns a single book by ID.
func (s *Store) Book(id string) (*Book, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.books[id]
	if !ok {
		return nil, ErrNotFound
	}
	return b.Clone(), nil
}

// Len returns the number of books in the library.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.books)
}

// Add inserts or replaces a book and persists the library.
func (s *Store) Add(b *Book) error {
	if b == nil || b.ID == "" {
		return errors.New("book must have an ID")
	}
	s.mu.Lock()
	if existing, ok := s.books[b.ID]; ok {
		b.AddedAt = existing.AddedAt
	}
	if b.AddedAt.IsZero() {
		b.AddedAt = time.Now().UTC()
	}
	s.books[b.ID] = b
	s.mu.Unlock()
	return s.save()
}

// Remove deletes a book and persists the library.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	delete(s.books, id)
	s.mu.Unlock()
	return s.save()
}

// save writes the library atomically.
func (s *Store) save() error {
	s.mu.RLock()
	books := make([]*Book, 0, len(s.books))
	for _, b := range s.books {
		books = append(books, b)
	}
	s.mu.RUnlock()
	sort.Slice(books, func(i, j int) bool { return books[i].AddedAt.Before(books[j].AddedAt) })

	data, err := json.MarshalIndent(books, "", "  ")
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

// ImportFile builds a Book from a single audio file and adds it to the store.
// The file's directory is scanned for sibling audio files with the same album,
// so that importing "Chapter 1.mp3" picks up the rest of the book.
func (s *Store) ImportFile(path string) (*Book, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	chapter, err := ReadChapter(abs)
	if err != nil {
		return nil, err
	}

	book := &Book{Title: strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))}
	if meta, err := readTags(abs); err == nil && meta != nil {
		if t := strings.TrimSpace(meta.Title()); t != "" {
			book.Title = t
		}
		book.Author = strings.TrimSpace(meta.AlbumArtist())
		if book.Author == "" {
			book.Author = strings.TrimSpace(meta.Artist())
		}
		book.Album = strings.TrimSpace(meta.Album())
		if c := strings.TrimSpace(meta.Composer()); c != "" {
			book.Narrator = c
		}
	}

	// Group siblings that share the file's album, falling back to the whole
	// directory when the files carry no album tag. Without this fallback, a
	// book whose ID3 tags were stripped would be imported one file at a time.
	chapters := []Chapter{chapter}
	if siblings, err := scanSiblings(abs, book.Album); err == nil && len(siblings) > 1 {
		chapters = siblings
		if book.Album != "" {
			book.Title = book.Album
		}
	}
	book.Chapters = chapters
	book.Source = "local"

	// Importing a file that is already part of a book should not create a
	// duplicate: the same audiobook is very often picked up again by accident.
	// Match on chapter paths, which are what actually identify the audio.
	if id, ok := s.findIDByChapterPaths(chapters); ok {
		return s.mergeInto(id, book)
	}

	book.ID = NewBookID()
	if err := s.Add(book); err != nil {
		return nil, err
	}
	s.cacheCover(book.ID)
	return s.Book(book.ID)
}

// mergeInto folds a freshly imported book into the existing one with the given
// ID, adding any chapters it does not have yet. Metadata the listener may have
// edited on the existing book is left alone. All mutation happens under the
// store's write lock.
func (s *Store) mergeInto(id string, fresh *Book) (*Book, error) {
	s.mu.Lock()
	existing, ok := s.books[id]
	if !ok {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	for _, ch := range fresh.Chapters {
		if !existing.HasChapter(ch.Path) {
			existing.Chapters = append(existing.Chapters, ch)
		}
	}
	existing.SortChapters()
	// Fill in metadata that is still empty rather than overwriting edits.
	if strings.TrimSpace(existing.Author) == "" {
		existing.Author = fresh.Author
	}
	if strings.TrimSpace(existing.Album) == "" {
		existing.Album = fresh.Album
	}
	if strings.TrimSpace(existing.Narrator) == "" {
		existing.Narrator = fresh.Narrator
	}
	merged := existing.Clone()
	s.mu.Unlock()

	if err := s.save(); err != nil {
		return nil, err
	}
	s.cacheCover(id)
	return merged, nil
}

// FindByChapterPaths returns a copy of the book that contains any of the given
// chapter paths, or nil when no book does.
func (s *Store) FindByChapterPaths(chapters []Chapter) *Book {
	id, ok := s.findIDByChapterPaths(chapters)
	if !ok {
		return nil
	}
	b, err := s.Book(id)
	if err != nil {
		return nil
	}
	return b
}

// findIDByChapterPaths returns the ID of the book holding any of the given
// chapter paths. Returning the ID rather than the book keeps callers from
// mutating a shared book outside the store's lock.
func (s *Store) findIDByChapterPaths(chapters []Chapter) (string, bool) {
	if len(chapters) == 0 {
		return "", false
	}
	wanted := map[string]bool{}
	for _, ch := range chapters {
		wanted[ch.Path] = true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Iterate in a stable order so a book matching several paths resolves the
	// same way every time.
	ids := make([]string, 0, len(s.books))
	for id := range s.books {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, ch := range s.books[id].Chapters {
			if wanted[ch.Path] {
				return id, true
			}
		}
	}
	return "", false
}

// cacheCover extracts and caches cover art for a book, ignoring failures
// because a book without cover art is perfectly usable. The extracted path is
// written back under the store's write lock.
func (s *Store) cacheCover(id string) {
	if s.dataDir == "" {
		return
	}
	b, err := s.Book(id)
	if err != nil {
		return
	}
	path, err := EnsureCover(s.dataDir, b)
	if err != nil || path == "" {
		return
	}
	s.mu.Lock()
	if stored, ok := s.books[id]; ok {
		stored.CoverArt = path
	}
	s.mu.Unlock()
	if err := s.save(); err != nil {
		return
	}
}

// ImportDir scans a directory for audio files and imports each distinct book it
// finds. Files that share an album are grouped into one book.
func (s *Store) ImportDir(dir string) ([]*Book, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	groups := map[string][]Chapter{}
	meta := map[string]*Book{}
	for _, e := range entries {
		if e.IsDir() || !audio.IsAudioFile(e.Name()) {
			continue
		}
		p := filepath.Join(abs, e.Name())
		ch, err := ReadChapter(p)
		if err != nil {
			continue
		}
		album, author := "", ""
		if m, err := readTags(p); err == nil && m != nil {
			album = strings.TrimSpace(m.Album())
			author = strings.TrimSpace(m.AlbumArtist())
			if author == "" {
				author = strings.TrimSpace(m.Artist())
			}
		}
		// Key on album and author only, so every file in a book lands in the
		// same group regardless of which chapters have been scanned. Files with
		// no album tag have nothing to group on, so they are treated as one book
		// per folder, named after the folder.
		key := BookKey(album, author)
		if album == "" {
			key = "dir:" + abs
		}
		groups[key] = append(groups[key], ch)
		if _, ok := meta[key]; !ok {
			b := &Book{Title: album, Author: author, Album: album, Source: "local"}
			if b.Title == "" {
				b.Title = filepath.Base(abs)
			}
			meta[key] = b
		}
	}

	var imported []*Book
	for key, chapters := range groups {
		sort.Slice(chapters, func(i, j int) bool { return NaturalLess(chapters[i].Title, chapters[j].Title) })
		b := meta[key]
		b.Chapters = chapters
		if id, ok := s.findIDByChapterPaths(chapters); ok {
			merged, err := s.mergeInto(id, b)
			if err != nil {
				return imported, err
			}
			imported = append(imported, merged)
			continue
		}
		b.ID = NewBookID()
		if err := s.Add(b); err != nil {
			return imported, err
		}
		s.cacheCover(b.ID)
		imported = append(imported, b)
	}
	sort.Slice(imported, func(i, j int) bool { return NaturalLess(imported[i].Title, imported[j].Title) })
	return imported, nil
}

// scanSiblings returns every audio file in the same directory as path whose ID3
// album matches album (or every audio file when album is empty).
func scanSiblings(path, album string) ([]Chapter, error) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var chapters []Chapter
	for _, e := range entries {
		if e.IsDir() || !audio.IsAudioFile(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if album != "" {
			m, err := readTags(p)
			if err != nil || m == nil || !strings.EqualFold(strings.TrimSpace(m.Album()), album) {
				continue
			}
		}
		ch, err := ReadChapter(p)
		if err != nil {
			continue
		}
		chapters = append(chapters, ch)
	}
	sort.Slice(chapters, func(i, j int) bool { return NaturalLess(chapters[i].Title, chapters[j].Title) })
	return chapters, nil
}

// ReadChapter reads a single audio file into a Chapter, pulling the title and
// duration from its metadata when possible.
func ReadChapter(path string) (Chapter, error) {
	ch := Chapter{Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	if m, err := readTags(path); err == nil && m != nil {
		if t := strings.TrimSpace(m.Title()); t != "" {
			ch.Title = t
		}
	}
	if d, err := audio.ProbeDuration(path); err == nil {
		ch.DurationMS = d.Milliseconds()
	}
	return ch, nil
}

func readTags(path string) (tag.Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return tag.ReadFrom(f)
}
