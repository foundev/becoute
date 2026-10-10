package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
)

// writeTestMP3 writes a valid MPEG-1 Layer III stream made of silent frames.
// Each 417-byte frame decodes to 1152 samples at 44.1 kHz.
func writeTestMP3(t *testing.T, frames int) string {
	t.Helper()
	const frameSize = 417
	buf := make([]byte, 0, frameSize*frames)
	for i := 0; i < frames; i++ {
		f := make([]byte, frameSize)
		f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0x00
		buf = append(buf, f...)
	}
	path := filepath.Join(t.TempDir(), "tone.mp3")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestProbeDurationMatchesFrameCount(t *testing.T) {
	const frames = 80
	path := writeTestMP3(t, frames)

	got, err := ProbeDuration(path)
	if err != nil {
		t.Fatalf("ProbeDuration: %v", err)
	}
	// 80 frames * 1152 samples / 44100 Hz = 2.0898s
	want := 2.089795918367347
	if math.Abs(got.Seconds()-want) > 0.001 {
		t.Fatalf("ProbeDuration = %v, want ~%v", got.Seconds(), want)
	}
}

func TestProbeDurationRejectsNonAudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.mp3")
	if err := os.WriteFile(path, []byte("this is not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeDuration(path); err == nil {
		t.Fatal("expected an error for a non-audio file")
	}
}

func TestDecodeAndResampleRealFile(t *testing.T) {
	path := writeTestMP3(t, 40)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	stream, format, err := mp3.Decode(f)
	if err != nil {
		t.Fatalf("mp3.Decode: %v", err)
	}
	defer stream.Close()

	if format.SampleRate != 44100 {
		t.Fatalf("sample rate = %d, want 44100", format.SampleRate)
	}
	if stream.Len() == 0 {
		t.Fatal("decoded stream has no length")
	}

	resampled := beep.Resample(4, format.SampleRate, SpeakerSampleRate, stream)
	buf := make([][2]float64, 512)
	n, ok := resampled.Stream(buf)
	if n == 0 {
		t.Fatal("resampler produced no samples")
	}
	_ = ok

	// Seek works on the underlying stream, which is what the player relies on
	// for resuming progress.
	if err := stream.Seek(0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if stream.Position() != 0 {
		t.Fatalf("position after seek = %d", stream.Position())
	}
}
