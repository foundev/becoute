package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
)

// TestRenderScreenshot is a development aid: it renders the window offscreen to
// a PNG so the layout can be eyeballed while working on the UI. It is skipped
// unless BECOUTE_SHOT names an output path.
//
//	BECOUTE_SHOT=/tmp/becoute.png go test ./internal/ui/ -run TestRenderScreenshot
func TestRenderScreenshot(t *testing.T) {
	out := os.Getenv("BECOUTE_SHOT")
	if out == "" {
		t.Skip("set BECOUTE_SHOT=<path> to render a screenshot")
	}
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 3)
	book.Title = "The Fellowship of the Ring"
	book.Author = "J. R. R. Tolkien"
	if err := lib.Add(book); err != nil {
		t.Fatal(err)
	}
	addTestBook(t, lib, "Beta", 2)
	app.refreshLibrary()
	app.selectedBookID = book.ID
	app.refreshDetails()
	app.loadSelected()
	app.refreshNowPlaying()
	app.refreshTransportBar()

	app.win.Resize(fyne.NewSize(1100, 720))
	img := app.win.Canvas().Capture()
	if img == nil {
		t.Fatal("no image rendered")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", filepath.Clean(out))
}
