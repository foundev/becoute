package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
)

// Sort options offered by the library view.
const (
	sortRecent   = "Recently added"
	sortTitle    = "Title"
	sortAuthor   = "Author"
	sortDuration = "Duration"
	sortPlayed   = "Recently played"
)

// SortOptions lists the library sort orders.
var SortOptions = []string{sortRecent, sortTitle, sortAuthor, sortDuration, sortPlayed}

// ApplySort orders books according to one of the SortOptions. Entries are read
// through the supplied lookup so progress-aware sorts work.
func ApplySort(books []*library.Book, mode string, entry func(string) (progress.Entry, bool)) {
	switch mode {
	case sortTitle:
		sort.SliceStable(books, func(i, j int) bool { return library.NaturalLess(books[i].Title, books[j].Title) })
	case sortAuthor:
		sort.SliceStable(books, func(i, j int) bool {
			ai, aj := books[i].DisplayAuthor(), books[j].DisplayAuthor()
			if strings.EqualFold(ai, aj) {
				return library.NaturalLess(books[i].Title, books[j].Title)
			}
			return library.NaturalLess(ai, aj)
		})
	case sortDuration:
		sort.SliceStable(books, func(i, j int) bool { return books[i].TotalDuration() > books[j].TotalDuration() })
	case sortPlayed:
		sort.SliceStable(books, func(i, j int) bool {
			ei, oki := entry(books[i].ID)
			ej, okj := entry(books[j].ID)
			switch {
			case oki && okj:
				return ei.UpdatedAt.After(ej.UpdatedAt)
			case oki:
				return true
			case okj:
				return false
			default:
				return library.NaturalLess(books[i].Title, books[j].Title)
			}
		})
	default:
		sort.SliceStable(books, func(i, j int) bool { return books[i].AddedAt.After(books[j].AddedAt) })
	}
}

// --- Metadata editing ---

// ShowEditMetadata opens a dialog for editing a book's descriptive fields.
func (a *App) ShowEditMetadata(book *library.Book) {
	title := widget.NewEntry()
	title.SetText(book.Title)
	author := widget.NewEntry()
	author.SetText(book.Author)
	narrator := widget.NewEntry()
	narrator.SetText(book.Narrator)
	album := widget.NewEntry()
	album.SetText(book.Album)

	form := widget.NewForm(
		widget.NewFormItem("Title", title),
		widget.NewFormItem("Author", author),
		widget.NewFormItem("Narrator", narrator),
		widget.NewFormItem("Album", album),
	)

	d := dialog.NewCustomConfirm("Edit book", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		if strings.TrimSpace(title.Text) == "" {
			a.setStatus("A book needs a title.")
			return
		}
		meta := library.Metadata{
			Title:    title.Text,
			Author:   author.Text,
			Narrator: narrator.Text,
			Album:    album.Text,
		}
		if err := a.ctrl.UpdateMetadata(book.ID, meta); err != nil {
			a.setStatus("Could not save: " + err.Error())
			return
		}
		a.refreshLibrary()
		a.refreshNowPlaying()
		a.setStatus(fmt.Sprintf("Saved %q.", strings.TrimSpace(title.Text)))
	}, a.win)
	d.Resize(fyne.NewSize(460, 320))
	d.Show()
}

// --- Chapter management ---

// ShowChapterManager opens a dialog for reordering, renaming, and removing the
// chapters of a book.
func (a *App) ShowChapterManager(book *library.Book) {
	// Work on a copy: edits are applied to the store as they happen, and the
	// dialog re-reads the book so it always shows the stored state.
	list := widget.NewList(
		func() int { return len(a.chaptersFor(book.ID)) },
		func() fyne.CanvasObject { return newChapterRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			chapters := a.chaptersFor(book.ID)
			if i < 0 || i >= len(chapters) {
				return
			}
			o.(*chapterRow).update(i, chapters[i], len(chapters))
		},
	)

	content := container.NewBorder(
		widget.NewLabelWithStyle("Reorder with the arrows. Renaming a chapter only changes what Becoute displays.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		nil, nil, nil, list,
	)

	d := dialog.NewCustom("Chapters", "Close", content, a.win)
	d.Resize(fyne.NewSize(640, 520))

	// Rebuild the row widgets whenever the book changes.
	refresh := func() {
		list.Refresh()
	}
	chapterRowHooks = &chapterHooks{
		move: func(from, to int) {
			if err := a.ctrl.MoveChapter(book.ID, from, to); err != nil {
				a.setStatus("Reorder failed: " + err.Error())
				return
			}
			refresh()
		},
		rename: func(index int, title string) {
			if err := a.ctrl.UpdateChapterTitle(book.ID, index, title); err != nil {
				a.setStatus("Rename failed: " + err.Error())
				return
			}
			a.refreshLibrary()
		},
		remove: func(index int) {
			chapters := a.chaptersFor(book.ID)
			if index < 0 || index >= len(chapters) {
				return
			}
			name := ChapterLabel(index, chapters[index])
			dialog.ShowConfirm("Remove chapter", fmt.Sprintf("Remove %q from this book?\n\nThe audio file is not deleted.", name), func(ok bool) {
				if !ok {
					return
				}
				if err := a.ctrl.RemoveChapter(book.ID, index); err != nil {
					a.setStatus("Remove failed: " + err.Error())
					return
				}
				a.refreshLibrary()
				a.refreshNowPlaying()
				if a.chaptersFor(book.ID) == nil {
					d.Hide()
					a.setStatus("The last chapter was removed, so the book was removed too.")
					return
				}
				refresh()
			}, a.win)
		},
	}
	d.SetOnClosed(func() { chapterRowHooks = nil })
	d.Show()
}

// chapterHooks carries the callbacks chapter rows need. The chapter dialog is
// modal, so a package-level slot is simpler than threading the callbacks
// through every row widget.
var chapterRowHooks *chapterHooks

type chapterHooks struct {
	move   func(from, to int)
	rename func(index int, title string)
	remove func(index int)
}

// chaptersFor returns a book's chapters, or nil when it no longer exists.
func (a *App) chaptersFor(id string) []library.Chapter {
	book, err := a.ctrl.Book(id)
	if err != nil {
		return nil
	}
	return book.Chapters
}

// chapterRow is one row of the chapter manager.
type chapterRow struct {
	widget.BaseWidget

	index  int
	total  int
	label  *widget.Label
	detail *widget.Label
	entry  *widget.Entry
	up     *widget.Button
	down   *widget.Button
	del    *widget.Button
	rename *widget.Button

	renaming bool
}

func newChapterRow() *chapterRow {
	r := &chapterRow{
		label:  widget.NewLabel(""),
		detail: widget.NewLabel(""),
		entry:  widget.NewEntry(),
	}
	r.label.TextStyle = fyne.TextStyle{Bold: true}
	r.detail.TextStyle = fyne.TextStyle{Monospace: true}
	r.detail.Importance = widget.LowImportance

	r.up = widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() {
		if chapterRowHooks != nil && r.index > 0 {
			chapterRowHooks.move(r.index, r.index-1)
		}
	})
	r.down = widget.NewButtonWithIcon("", theme.MoveDownIcon(), func() {
		if chapterRowHooks != nil && r.index < r.total-1 {
			chapterRowHooks.move(r.index, r.index+1)
		}
	})
	r.del = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		if chapterRowHooks != nil {
			chapterRowHooks.remove(r.index)
		}
	})
	r.entry.OnSubmitted = func(string) { r.commitRename() }
	r.rename = widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		r.toggleRename()
	})

	r.ExtendBaseWidget(r)
	return r
}

func (r *chapterRow) toggleRename() {
	if r.renaming {
		r.commitRename()
		return
	}
	r.renaming = true
	r.entry.SetText(r.label.Text)
	r.entry.Show()
	r.label.Hide()
	r.Refresh()
}

func (r *chapterRow) commitRename() {
	if !r.renaming {
		return
	}
	r.renaming = false
	name := strings.TrimSpace(r.entry.Text)
	r.entry.Hide()
	r.label.Show()
	if name != "" && name != r.label.Text && chapterRowHooks != nil {
		chapterRowHooks.rename(r.index, name)
	}
	r.Refresh()
}

func (r *chapterRow) CreateRenderer() fyne.WidgetRenderer {
	buttons := container.NewHBox(r.up, r.down, r.rename, r.del)
	text := container.NewVBox(r.label, r.entry, r.detail)
	r.entry.Hide()
	return widget.NewSimpleRenderer(container.NewPadded(container.NewBorder(nil, nil, nil, buttons, text)))
}

func (r *chapterRow) update(index int, ch library.Chapter, total int) {
	r.index = index
	r.total = total
	r.label.SetText(fmt.Sprintf("%d. %s", index+1, ch.Title))
	detail := FormatDuration(ch.Duration())
	if ch.Path != "" {
		detail += " · " + ch.Path
	}
	r.detail.SetText(detail)
	r.up.Enable()
	r.down.Enable()
	r.del.Enable()
	if index == 0 {
		r.up.Disable()
	}
	if index == total-1 {
		r.down.Disable()
	}
	// Leaving edit mode when the row is recycled keeps the list honest.
	if r.renaming {
		r.renaming = false
		r.entry.Hide()
		r.label.Show()
	}
}

// --- Merge and transfer ---

// ShowMergeDialog asks which book to merge the given book into.
func (a *App) ShowMergeDialog(source *library.Book) {
	var candidates []*library.Book
	for _, b := range a.ctrl.Books() {
		if b.ID != source.ID {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		a.setStatus("There is no other book to merge into.")
		return
	}

	labels := make([]string, len(candidates))
	byLabel := map[string]*library.Book{}
	for i, b := range candidates {
		label := fmt.Sprintf("%s — %s", b.Title, b.DisplayAuthor())
		if _, taken := byLabel[label]; taken {
			label = fmt.Sprintf("%s (%s)", label, b.ID[:6])
		}
		labels[i] = label
		byLabel[label] = b
	}
	sort.Strings(labels)

	pick := widget.NewSelect(labels, nil)
	pick.SetSelected(labels[0])
	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Merge %q into:", source.Title)),
		pick,
		widget.NewLabelWithStyle("Chapters are combined and listening progress is kept. The other book is removed.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
	)

	d := dialog.NewCustomConfirm("Merge books", "Merge", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		target := byLabel[pick.Selected]
		if target == nil {
			return
		}
		if err := a.ctrl.MergeBooks(target.ID, source.ID); err != nil {
			a.setStatus("Merge failed: " + err.Error())
			return
		}
		a.refreshLibrary()
		a.refreshNowPlaying()
		a.setStatus(fmt.Sprintf("Merged into %q.", target.Title))
	}, a.win)
	d.Resize(fyne.NewSize(520, 300))
	d.Show()
}

// --- Details panel ---

// bookDetails renders a summary panel for the selected book.
func (a *App) bookDetails(book *library.Book) fyne.CanvasObject {
	if book == nil {
		return container.NewCenter(widget.NewLabel("Select a book to see its details."))
	}

	cover := newCoverImage(a.ctrl.CoverPath(book), 160)

	title := widget.NewLabelWithStyle(book.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapWord
	author := widget.NewLabel(book.DisplayAuthor())
	author.Wrapping = fyne.TextWrapWord

	rows := []fyne.CanvasObject{
		widget.NewLabel(fmt.Sprintf("Chapters: %d", len(book.Chapters))),
		widget.NewLabel(fmt.Sprintf("Length: %s", FormatDuration(book.TotalDuration()))),
	}
	if book.Narrator != "" {
		rows = append(rows, widget.NewLabel("Narrator: "+book.Narrator))
	}
	if book.Album != "" {
		rows = append(rows, widget.NewLabel("Album: "+book.Album))
	}
	rows = append(rows, widget.NewLabel("Added: "+book.AddedAt.Local().Format(time.RFC1123[:11])))

	if entry, ok := a.ctrl.Entry(book.ID); ok {
		rows = append(rows, widget.NewLabel(fmt.Sprintf("Progress: %.0f%%", entry.Percent*100)))
		if entry.PlayCount > 0 {
			rows = append(rows, widget.NewLabel(fmt.Sprintf("Played: %d time(s)", entry.PlayCount)))
		}
	} else {
		rows = append(rows, widget.NewLabel("Progress: not started"))
	}

	actions := container.NewVBox(
		widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), func() {
			a.openBook(book)
			if err := a.ctrl.Play(); err != nil {
				a.setStatus(err.Error())
			}
			a.refreshNowPlaying()
		}),
		widget.NewButtonWithIcon("Edit details", theme.DocumentCreateIcon(), func() {
			a.ShowEditMetadata(book)
		}),
		widget.NewButtonWithIcon("Chapters", theme.ListIcon(), func() {
			a.ShowChapterManager(book)
		}),
		widget.NewButtonWithIcon("Rescan files", theme.ViewRefreshIcon(), func() {
			changed, err := a.ctrl.RescanChapters(book.ID)
			if err != nil {
				a.setStatus("Rescan failed: " + err.Error())
				return
			}
			a.refreshLibrary()
			a.refreshNowPlaying()
			if changed == 0 {
				a.setStatus("Rescan found no changes.")
				return
			}
			a.setStatus(fmt.Sprintf("Rescan updated %d chapter(s).", changed))
		}),
		widget.NewButtonWithIcon("Merge into…", theme.ContentCopyIcon(), func() {
			a.ShowMergeDialog(book)
		}),
		widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
			a.removeBook(book)
		}),
	)

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		container.NewCenter(cover),
		container.NewCenter(title),
		container.NewCenter(author),
		widget.NewSeparator(),
		container.NewVBox(rows...),
		widget.NewSeparator(),
		actions,
	)))
}

// removeBook asks for confirmation before removing a book.
func (a *App) removeBook(book *library.Book) {
	dialog.ShowConfirm("Remove book", fmt.Sprintf("Remove %q from your library?\n\nThe audio files are not deleted.", book.Title), func(ok bool) {
		if !ok {
			return
		}
		if err := a.ctrl.RemoveBook(book.ID); err != nil {
			a.setStatus("Remove failed: " + err.Error())
			return
		}
		a.selectedBookID = ""
		a.refreshLibrary()
		a.refreshNowPlaying()
		a.setStatus(fmt.Sprintf("Removed %q.", book.Title))
	}, a.win)
}
