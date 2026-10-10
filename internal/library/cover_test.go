package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCoverFromTaggedFile(t *testing.T) {
	dir := t.TempDir()
	png := makePNG(t)
	path := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(10), png)

	data, ext, err := ExtractCover(path)
	if err != nil {
		t.Fatalf("ExtractCover: %v", err)
	}
	if ext != ".png" {
		t.Fatalf("ext = %q, want .png", ext)
	}
	if len(data) != len(png) {
		t.Fatalf("got %d bytes of art, want %d", len(data), len(png))
	}
}

func TestExtractCoverWithoutArt(t *testing.T) {
	dir := t.TempDir()
	path := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(10), nil)
	if _, _, err := ExtractCover(path); err == nil {
		t.Fatal("expected an error for a file with no embedded art")
	}
}

func TestSaveCoverWritesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	png := makePNG(t)

	first, err := SaveCover(dir, "book-1", png, ".png")
	if err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("cover was not written: %v", err)
	}

	// Saving a different format for the same book must not leave the old file.
	second, err := SaveCover(dir, "book-1", []byte{0xFF, 0xD8, 0xFF}, ".jpg")
	if err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	if second == first {
		t.Fatal("changing the cover extension should change the file name")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("the old cover file should have been removed")
	}

	if _, err := SaveCover(dir, "book-2", nil, ".png"); err == nil {
		t.Fatal("expected an error when saving empty cover data")
	}
}

func TestRemoveCover(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveCover(dir, "book-1", makePNG(t), ".png")
	if err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	if err := RemoveCover(dir, "book-1"); err != nil {
		t.Fatalf("RemoveCover: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cover file should be gone")
	}
	// Removing a missing cover is not an error.
	if err := RemoveCover(dir, "book-1"); err != nil {
		t.Fatalf("RemoveCover on missing file: %v", err)
	}
}

func TestFindSidecarCover(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "1.mp3")
	if err := os.WriteFile(audio, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindSidecarCover([]string{audio}); got != "" {
		t.Fatalf("expected no sidecar, got %q", got)
	}
	cover := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(cover, []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindSidecarCover([]string{audio}); got != cover {
		t.Fatalf("got %q, want %q", got, cover)
	}
}

func TestEnsureCoverFallsBackToSidecar(t *testing.T) {
	dir := t.TempDir()
	// No embedded art, but a cover image sits next to the audio, which is the
	// layout LibriVox downloads use.
	audio := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(10), nil)
	sidecar := filepath.Join(dir, "cover.png")
	if err := os.WriteFile(sidecar, makePNG(t), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &Book{ID: NewBookID(), Title: "Book", Chapters: []Chapter{{Path: audio}}}
	got, err := EnsureCover(dir, book)
	if err != nil {
		t.Fatalf("EnsureCover: %v", err)
	}
	if got == "" {
		t.Fatal("expected a cover to be produced from the sidecar image")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("cover was not written: %v", err)
	}
	if book.CoverArt != got {
		t.Fatalf("book.CoverArt = %q, want %q", book.CoverArt, got)
	}

	// Calling again should reuse the cached file rather than rewrite it.
	again, err := EnsureCover(dir, book)
	if err != nil {
		t.Fatalf("second EnsureCover: %v", err)
	}
	if again != got {
		t.Fatalf("second call returned %q, want %q", again, got)
	}
}

func TestEnsureCoverPrefersEmbeddedArt(t *testing.T) {
	dir := t.TempDir()
	embedded := makePNG(t)
	audio := writeTaggedMP3(t, dir, "Chapter One.mp3", silentMP3Frames(10), embedded)
	if err := os.WriteFile(filepath.Join(dir, "cover.png"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &Book{ID: NewBookID(), Title: "Book", Chapters: []Chapter{{Path: audio}}}
	got, err := EnsureCover(dir, book)
	if err != nil {
		t.Fatalf("EnsureCover: %v", err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("reading cover: %v", err)
	}
	if len(data) != len(embedded) {
		t.Fatalf("got %d bytes, want the %d bytes of embedded art", len(data), len(embedded))
	}
}

func TestEnsureCoverDropsStaleCachePath(t *testing.T) {
	dir := t.TempDir()
	book := &Book{ID: NewBookID(), Title: "Book", CoverArt: filepath.Join(dir, "covers", "gone.jpg")}
	if _, err := EnsureCover(dir, book); err != nil {
		t.Fatalf("EnsureCover: %v", err)
	}
	if book.CoverArt != "" {
		t.Fatalf("a missing cover file should be cleared, got %q", book.CoverArt)
	}
}
