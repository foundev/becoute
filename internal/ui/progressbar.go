package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// barHeight is the thickness of the thin progress bars used in list rows.
const barHeight float32 = 6

// progressBar is a slim progress bar. The stock widget reserves a full line of
// text height even when it has no label, which makes list rows far taller than
// they need to be, so this draws just the bar.
type progressBar struct {
	widget.BaseWidget

	value float64
	// fillColor overrides the theme's primary colour when set.
	fillColor color.Color
}

func newProgressBar() *progressBar {
	p := &progressBar{}
	p.ExtendBaseWidget(p)
	return p
}

func (p *progressBar) SetValue(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	if p.value == v {
		return
	}
	p.value = v
	p.Refresh()
}

// Value returns the current fraction, 0 to 1.
func (p *progressBar) Value() float64 { return p.value }

func (p *progressBar) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	fill := canvas.NewRectangle(p.fill())
	fill.CornerRadius = barHeight / 2
	bg.CornerRadius = barHeight / 2
	return &progressBarRenderer{bar: p, bg: bg, fill: fill}
}

func (p *progressBar) fill() color.Color {
	if p.fillColor != nil {
		return p.fillColor
	}
	return theme.Color(theme.ColorNamePrimary)
}

func (p *progressBar) MinSize() fyne.Size {
	return fyne.NewSize(theme.Padding()*10, barHeight)
}

type progressBarRenderer struct {
	bar  *progressBar
	bg   *canvas.Rectangle
	fill *canvas.Rectangle
}

func (r *progressBarRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	// Keep a sliver visible so a started book reads as non-empty.
	width := float32(r.bar.value) * size.Width
	if r.bar.value > 0 && width < barHeight {
		width = barHeight
	}
	r.fill.Resize(fyne.NewSize(width, size.Height))
	r.fill.Move(fyne.NewPos(0, 0))
}

func (r *progressBarRenderer) MinSize() fyne.Size { return r.bar.MinSize() }

func (r *progressBarRenderer) Refresh() {
	r.fill.FillColor = r.bar.fill()
	r.fill.CornerRadius = barHeight / 2
	r.bg.CornerRadius = barHeight / 2
	r.Layout(r.bar.Size())
	canvas.Refresh(r.bar)
}

func (r *progressBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.fill}
}

func (r *progressBarRenderer) Destroy() {}
