package audio

import "testing"

func TestIsAudioFile(t *testing.T) {
	cases := map[string]bool{
		"chapter1.mp3": true,
		"CHAPTER.MP3":  true,
		"book.mp4":     false,
		"notes.txt":    false,
		"noextension":  false,
		"book.mp3.bak": false,
	}
	for name, want := range cases {
		if got := IsAudioFile(name); got != want {
			t.Errorf("IsAudioFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestVolumeToExponent(t *testing.T) {
	if got := volumeToExponent(1); got != 0 {
		t.Errorf("full volume exponent = %v, want 0", got)
	}
	if got := volumeToExponent(0); got >= 0 {
		t.Errorf("muted exponent = %v, want a large negative value", got)
	}
	if got := volumeToExponent(0.5); got != -1 {
		t.Errorf("half volume exponent = %v, want -1", got)
	}
}
