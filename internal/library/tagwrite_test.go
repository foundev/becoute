package library

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// These helpers build real ID3v2.3 tags, because the tag reader used by Becoute
// is read-only and cover extraction must be tested against genuine tag bytes.

func syncsafe(n int) []byte {
	return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
}

func id3TextFrame(id, value string) []byte {
	body := append([]byte{0x00}, []byte(value)...) // ISO-8859-1
	frame := make([]byte, 0, 10+len(body))
	frame = append(frame, id...)
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(body)))
	frame = append(frame, size...)
	frame = append(frame, 0x00, 0x00) // flags
	return append(frame, body...)
}

func id3APICFrame(mime string, picture []byte) []byte {
	var body []byte
	body = append(body, 0x00)            // ISO-8859-1
	body = append(body, []byte(mime)...) // MIME type
	body = append(body, 0x00)            // NUL
	body = append(body, 0x03)            // picture type: front cover
	body = append(body, 0x00)            // empty description + NUL
	body = append(body, picture...)

	frame := make([]byte, 0, 10+len(body))
	frame = append(frame, "APIC"...)
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(body)))
	frame = append(frame, size...)
	frame = append(frame, 0x00, 0x00)
	return append(frame, body...)
}

// makePNG returns a tiny valid PNG image.
func makePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// writeTaggedMP3 writes an MP3 with a real ID3v2.3 tag in front of silent
// MPEG frames.
func writeTaggedMP3(t *testing.T, dir, name string, frames []byte, picture []byte) string {
	t.Helper()
	var tag []byte
	tag = append(tag, id3TextFrame("TIT2", "Chapter One")...)
	tag = append(tag, id3TextFrame("TPE1", "A Narrator")...)
	tag = append(tag, id3TextFrame("TALB", "The Album")...)
	if picture != nil {
		tag = append(tag, id3APICFrame("image/png", picture)...)
	}
	header := []byte("ID3\x03\x00\x00")
	header = append(header, syncsafe(len(tag))...)

	var out []byte
	out = append(out, header...)
	out = append(out, tag...)
	out = append(out, frames...)

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// silentMP3Frames returns valid MPEG-1 Layer III silent frames.
func silentMP3Frames(count int) []byte {
	const frameSize = 417
	buf := make([]byte, 0, frameSize*count)
	for i := 0; i < count; i++ {
		f := make([]byte, frameSize)
		f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0x00
		buf = append(buf, f...)
	}
	return buf
}
