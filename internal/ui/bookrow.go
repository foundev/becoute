package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// bookRow is one entry in the library list: title, metadata, and progress.
type bookRow struct {
	widget.BaseWidget

	title    *widget.Label
	subtitle *widget.Label
	progress *progressBar
	percent  *widget.Label
}

func newBookRow() *bookRow {
	r := &bookRow{
		title:    widget.NewLabel(""),
		subtitle: widget.NewLabel(""),
		progress: newProgressBar(),
		percent:  widget.NewLabel(""),
	}
	// Give the labels placeholder text so their minimum widths are realistic.
	// An empty label measures 16px wide, which would leave the percentage
	// squeezed into a sliver the first time a row is laid out.
	r.title.SetText(" ")
	r.percent.SetText("New")
	r.subtitle.SetText(" ")
	r.title.TextStyle = fyne.TextStyle{Bold: true}
	r.subtitle.TextStyle = fyne.TextStyle{Italic: true}
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.subtitle.Truncation = fyne.TextTruncateEllipsis
	r.ExtendBaseWidget(r)
	return r
}

func (r *bookRow) CreateRenderer() fyne.WidgetRenderer {
	// Two text lines keep rows compact: the title shares a line with the
	// percentage, and the author and chapter detail share the second line. A
	// thin progress bar runs underneath.
	heading := container.NewBorder(nil, nil, nil, r.percent, r.title)
	return widget.NewSimpleRenderer(container.NewPadded(container.NewVBox(
		heading,
		r.subtitle,
		r.progress,
	)))
}

// update fills the row with a book's current state.
func (r *bookRow) update(book *library.Book, ctrl *Controller) {
	r.title.SetText(book.Title)

	parts := []string{book.DisplayAuthor()}
	if n := len(book.Chapters); n == 1 {
		parts = append(parts, "1 chapter")
	} else {
		parts = append(parts, fmt.Sprintf("%d chapters", n))
	}
	parts = append(parts, FormatDuration(book.TotalDuration()))

	switch entry, ok := ctrl.Entry(book.ID); {
	case ok && entry.Finished:
		r.progress.SetValue(1)
		r.percent.SetText("Finished")
		parts = append(parts, "completed")
	case ok:
		r.progress.SetValue(entry.Percent)
		r.percent.SetText(fmt.Sprintf("%.0f%%", entry.Percent*100))
		remaining := book.TotalDuration() - durationAt(book, entry)
		parts = append(parts, fmt.Sprintf("%s left", FormatDuration(remaining)))
		parts = append(parts, fmt.Sprintf("ch %d/%d", entry.ChapterIndex+1, len(book.Chapters)))
	default:
		r.progress.SetValue(0)
		r.percent.SetText("New")
		parts = append(parts, "not started")
	}
	r.subtitle.SetText(strings.Join(parts, " · "))
}

// durationAt returns how much of a book has been listened to for an entry.
func durationAt(book *library.Book, e progress.Entry) time.Duration {
	var d time.Duration
	for i := 0; i < e.ChapterIndex && i < len(book.Chapters); i++ {
		d += book.Chapters[i].Duration()
	}
	return d + time.Duration(e.PositionMS)*time.Millisecond
}
