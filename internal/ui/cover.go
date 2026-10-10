package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// newCoverImage returns a cover art image of the requested size. When no cover
// is available it returns a placeholder so layouts do not jump around as covers
// load.
func newCoverImage(path string, size float32) fyne.CanvasObject {
	if path == "" {
		return newCoverPlaceholder(size)
	}
	img := canvas.NewImageFromFile(path)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	img.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	return img
}

// newCoverPlaceholder draws a muted square with a book icon, used when a book
// has no cover art.
func newCoverPlaceholder(size float32) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.SetMinSize(fyne.NewSize(size, size))
	bg.CornerRadius = theme.Size(theme.SizeNameInputRadius)

	icon := widget.NewIcon(theme.DocumentIcon())
	icon.Resize(fyne.NewSize(size/3, size/3))

	stack := fyne.NewContainerWithLayout(
		layoutCenter,
		bg,
		icon,
	)
	stack.Resize(fyne.NewSize(size, size))
	return stack
}

// layoutCenter places a single object in the middle of its container.
var layoutCenter fyne.Layout = centerLayout{}

type centerLayout struct{}

func (centerLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var min fyne.Size
	for _, o := range objects {
		if o.Visible() {
			min = min.Max(o.MinSize())
		}
	}
	return min
}

func (centerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		min := o.MinSize()
		o.Resize(min)
		o.Move(fyne.NewPos((size.Width-min.Width)/2, (size.Height-min.Height)/2))
	}
}
