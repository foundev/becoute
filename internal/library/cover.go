package library

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// CoverExtensions lists the image extensions Becoute will write cover art to.
var CoverExtensions = []string{".jpg", ".jpeg", ".png"}

// coverDirName is the subdirectory of the data directory that holds cached
// cover images.
const coverDirName = "covers"

// CoverDir returns the directory cover art is cached in, given the directory
// that holds the library file.
func CoverDir(dataDir string) string {
	return filepath.Join(dataDir, coverDirName)
}

// ExtractCover reads embedded cover art from an audio file. It returns the raw
// image bytes and the file extension, or an error when the file has no picture.
func ExtractCover(path string) (data []byte, ext string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	meta, err := tag.ReadFrom(f)
	if err != nil {
		return nil, "", err
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, "", fmt.Errorf("%s has no embedded cover art", filepath.Base(path))
	}
	return pic.Data, coverExtension(pic), nil
}

// coverExtension picks a file extension for a picture, preferring the actual
// image format over the tag's declared type.
func coverExtension(pic *tag.Picture) string {
	if mime := strings.ToLower(strings.TrimSpace(pic.MIMEType)); mime != "" {
		switch {
		case strings.Contains(mime, "png"):
			return ".png"
		case strings.Contains(mime, "jpeg"), strings.Contains(mime, "jpg"):
			return ".jpg"
		}
	}
	switch ext := strings.ToLower(strings.TrimSpace(pic.Ext)); ext {
	case "png":
		return ".png"
	case "jpg", "jpeg":
		return ".jpg"
	}
	// Fall back to sniffing the first bytes of the payload.
	if len(pic.Data) >= 8 && string(pic.Data[1:4]) == "PNG" {
		return ".png"
	}
	if len(pic.Data) >= 3 && pic.Data[0] == 0xFF && pic.Data[1] == 0xD8 {
		return ".jpg"
	}
	return ".jpg"
}

// coverFileName returns the cache file name for a book's cover art. It is
// derived from the book ID, so re-extracting a cover overwrites the old file.
func coverFileName(bookID, ext string) string {
	h := sha1.Sum([]byte(bookID))
	return hex.EncodeToString(h[:])[:16] + ext
}

// SaveCover writes cover art into the cover cache and returns its path. Any
// previous cover for the same book is removed, so switching between a .jpg and
// a .png does not leave the stale file behind.
func SaveCover(dataDir, bookID string, data []byte, ext string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("no cover art data for book %s", bookID)
	}
	if ext == "" {
		ext = ".jpg"
	}
	dir := CoverDir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	removeCachedCover(dataDir, bookID)
	path := filepath.Join(dir, coverFileName(bookID, ext))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// RemoveCover deletes a book's cached cover art, if any.
func RemoveCover(dataDir, bookID string) error {
	return removeCachedCover(dataDir, bookID)
}

func removeCachedCover(dataDir, bookID string) error {
	for _, ext := range CoverExtensions {
		p := filepath.Join(CoverDir(dataDir), coverFileName(bookID, ext))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// FindSidecarCover looks for a cover image next to a book's audio files, which
// is how audiobooks downloaded from LibriVox are usually packaged. It returns
// the path of the first image found, or "" when there is none.
func FindSidecarCover(chapterPaths []string) string {
	seen := map[string]bool{}
	for _, p := range chapterPaths {
		dir := filepath.Dir(p)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.png", "front.jpg", "front.png"} {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

// EnsureCover makes sure a book has cover art, extracting it from the book's
// first chapter that carries an embedded picture, or falling back to a cover
// image stored beside the audio files. It returns the cover path, which is ""
// when no art could be found.
func EnsureCover(dataDir string, book *Book) (string, error) {
	if book == nil {
		return "", nil
	}
	// Already cached and still on disk.
	if book.CoverArt != "" {
		if info, err := os.Stat(book.CoverArt); err == nil && !info.IsDir() {
			return book.CoverArt, nil
		}
		// The cached file is gone; forget the stale path so callers do not keep
		// pointing at a file that no longer exists.
		book.CoverArt = ""
	}

	var lastErr error
	for _, ch := range book.Chapters {
		data, ext, err := ExtractCover(ch.Path)
		if err != nil {
			lastErr = err
			continue
		}
		path, err := SaveCover(dataDir, book.ID, data, ext)
		if err != nil {
			return "", err
		}
		book.CoverArt = path
		return path, nil
	}

	// Fall back to a sidecar image shipped alongside the audio.
	if sidecar := FindSidecarCover(chapterPaths(book)); sidecar != "" {
		data, err := os.ReadFile(sidecar)
		if err == nil {
			ext := strings.ToLower(filepath.Ext(sidecar))
			path, err := SaveCover(dataDir, book.ID, data, ext)
			if err == nil {
				book.CoverArt = path
				return path, nil
			}
		}
	}
	return "", lastErr
}

func chapterPaths(book *Book) []string {
	paths := make([]string, 0, len(book.Chapters))
	for _, ch := range book.Chapters {
		paths = append(paths, ch.Path)
	}
	return paths
}
