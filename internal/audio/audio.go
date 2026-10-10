// Package audio handles decoding, playback, and duration probing of audiobook
// audio files.
package audio

import (
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/go-mp3"
)

// SupportedExtensions lists the file extensions Becoute can play.
var SupportedExtensions = []string{".mp3"}

// IsAudioFile reports whether name has a supported audio extension.
func IsAudioFile(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range SupportedExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// ProbeDuration returns the length of an MP3 file. It decodes the file's frame
// headers to compute the exact length, so it is proportional to the file size.
func ProbeDuration(path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	d, err := mp3.NewDecoder(f)
	if err != nil {
		return 0, err
	}
	length := d.Length()
	if length <= 0 {
		return 0, nil
	}
	const bytesPerFrame = 4 // 16-bit, 2 channels
	samples := length / bytesPerFrame
	return time.Second * time.Duration(samples) / time.Duration(d.SampleRate()), nil
}
