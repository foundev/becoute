package ui

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

func TestRenderPlayerTab(t *testing.T) {
	out := os.Getenv("BECOUTE_SHOT_PLAYER")
	if out == "" {
		t.Skip("set BECOUTE_SHOT_PLAYER=<path>")
	}
	app, _, lib, _ := newTestApp(t)
	book := addTestBook(t, lib, "Alpha", 3)
	book.Title = "The Fellowship of the Ring"
	book.Author = "J. R. R. Tolkien"
	if err := lib.Add(book); err != nil {
		t.Fatal(err)
	}
	app.openBook(book)
	app.win.Resize(fyne.NewSize(1100, 720))

	// Switch to the Now Playing tab.
	if tabs, ok := app.win.Content().(*container.AppTabs); ok {
		tabs.SelectIndex(1)
	}
	img := app.win.Canvas().Capture()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
	t.Logf("wrote %s", out)
}
