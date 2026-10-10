package ui

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
)

// TestRenderChapterManagerScreenshot renders the chapter dialog offscreen for
// visual inspection. Skipped unless BECOUTE_SHOT_CHAPTERS is set.
func TestRenderChapterManagerScreenshot(t *testing.T) {
	out := os.Getenv("BECOUTE_SHOT_CHAPTERS")
	if out == "" {
		t.Skip("set BECOUTE_SHOT_CHAPTERS=<path> to render a screenshot")
	}
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 4)
	book.Title = "The Fellowship of the Ring"
	book.Author = "J. R. R. Tolkien"
	if err := lib.Add(book); err != nil {
		t.Fatal(err)
	}
	app.ShowChapterManager(book)
	app.win.Resize(fyne.NewSize(1100, 720))

	img := app.win.Canvas().Capture()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}
