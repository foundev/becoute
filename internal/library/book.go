// Package library models an audiobook library: books, their chapters, and the
// JSON file they are persisted to.
package library

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ChapterNumberRe matches the leading number in a chapter title, tolerating the
// prefixes audiobook files commonly use.
var ChapterNumberRe = regexp.MustCompile(`(?i)\b(?:chapter|part|track|ch)?[\s._-]*(\d{1,4})\b`)

// Chapter is a single audio file that is part of a Book.
type Chapter struct {
	// Title is the chapter title, taken from ID3 metadata when available and
	// otherwise derived from the file name.
	Title string `json:"title"`
	// Path is an absolute path to the audio file.
	Path string `json:"path"`
	// DurationMS is the chapter length in milliseconds. Zero means unknown.
	DurationMS int64 `json:"duration_ms"`
}

// Duration returns the chapter length as a time.Duration.
func (c Chapter) Duration() time.Duration {
	return time.Duration(c.DurationMS) * time.Millisecond
}

// Book is an audiobook, made up of one or more chapters played in order.
type Book struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Author   string    `json:"author"`
	Narrator string    `json:"narrator,omitempty"`
	Album    string    `json:"album,omitempty"`
	Chapters []Chapter `json:"chapters"`

	AddedAt  time.Time `json:"added_at"`
	Source   string    `json:"source,omitempty"`
	CoverArt string    `json:"cover_art,omitempty"`

	// CustomOrder records that the listener arranged the chapters by hand.
	// Rescans and merges then leave that order alone instead of re-sorting.
	CustomOrder bool `json:"custom_order,omitempty"`
}

// TotalDuration returns the summed duration of all chapters.
func (b *Book) TotalDuration() time.Duration {
	var total time.Duration
	for _, c := range b.Chapters {
		total += c.Duration()
	}
	return total
}

// DisplayAuthor returns the author, or "Unknown author" when it is empty.
func (b *Book) DisplayAuthor() string {
	if strings.TrimSpace(b.Author) == "" {
		return "Unknown author"
	}
	return b.Author
}

// ChapterAt returns the chapter at index i, or false when out of range.
func (b *Book) ChapterAt(i int) (Chapter, bool) {
	if i < 0 || i >= len(b.Chapters) {
		return Chapter{}, false
	}
	return b.Chapters[i], true
}

// BookKey identifies the book a set of chapters belongs to, ignoring which
// chapters are present. Two files from the same album share a key, which is how
// imports group sibling files into one book.
func BookKey(title, author string) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s\x00%s", strings.ToLower(strings.TrimSpace(title)), strings.ToLower(strings.TrimSpace(author)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// NewBookID returns a fresh, stable identifier for a book.
//
// The ID deliberately does not depend on the book's title, author, or chapter
// list: those all change as the listener edits metadata or adds and removes
// chapters, and progress is keyed by ID, so deriving the ID from them would
// discard listening progress on every edit. Books are matched to each other by
// their chapter paths instead (see Store.FindByChapterPaths).
func NewBookID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should not fail; fall back to a time-based ID rather than
		// returning an empty one.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Clone returns a deep copy of the book. The store hands out clones so that a
// caller reading a book can never race with a concurrent mutation, and so that
// mutating a book requires going through the store.
func (b *Book) Clone() *Book {
	if b == nil {
		return nil
	}
	c := *b
	c.Chapters = append([]Chapter(nil), b.Chapters...)
	return &c
}

// ChapterPaths returns the absolute paths of every chapter in the book.
func (b *Book) ChapterPaths() []string {
	paths := make([]string, 0, len(b.Chapters))
	for _, c := range b.Chapters {
		paths = append(paths, c.Path)
	}
	return paths
}

// HasChapter reports whether path is already one of the book's chapters.
func (b *Book) HasChapter(path string) bool {
	for _, c := range b.Chapters {
		if c.Path == path {
			return true
		}
	}
	return false
}

// SortChapters orders chapters naturally, so "Chapter 2" precedes
// "Chapter 10". Books the listener has ordered by hand are left untouched.
func (b *Book) SortChapters() {
	if b.CustomOrder {
		return
	}
	sort.SliceStable(b.Chapters, func(i, j int) bool {
		return NaturalLess(b.Chapters[i].Title, b.Chapters[j].Title)
	})
}

// NaturalLess compares two strings so that "chapter 2" sorts before
// "chapter 10".
func NaturalLess(a, b string) bool {
	al, bl := strings.ToLower(a), strings.ToLower(b)
	an := ChapterNumberRe.FindStringSubmatch(al)
	bn := ChapterNumberRe.FindStringSubmatch(bl)
	if len(an) == 2 && len(bn) == 2 {
		ai, _ := strconv.Atoi(an[1])
		bi, _ := strconv.Atoi(bn[1])
		if ai != bi {
			return ai < bi
		}
	}
	return al < bl
}
