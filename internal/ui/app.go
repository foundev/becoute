package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/foundev/becoute/internal/audio"
	"github.com/foundev/becoute/internal/library"
)

// App is the Becoute desktop window.
type App struct {
	fyneApp fyne.App
	win     fyne.Window
	ctrl    *Controller

	// library view
	libraryList    *widget.List
	filter         *widget.Select
	sort           *widget.Select
	search         *widget.Entry
	details        *fyne.Container
	statusLabel    *widget.Label
	statsLabel     *widget.Label
	visibleBooks   []*library.Book
	selectedBookID string

	// now playing view
	nowTitle      *widget.Label
	nowAuthor     *widget.Label
	chapterSelect *widget.Select
	chapters      []library.Chapter
	loadedBookID  string

	seekSlider   *widget.Slider
	elapsedLabel *widget.Label
	totalLabel   *widget.Label
	bookProgress *widget.ProgressBar
	bookPercent  *widget.Label
	playButton   *widget.Button
	prevButton   *widget.Button
	nextButton   *widget.Button
	speedSelect  *widget.Select
	volumeSlider *widget.Slider
	autoplayChk  *widget.Check

	// tabs is the top-level tab container, kept so tests and future code can
	// reach it through the transport wrapper.
	tabs *container.AppTabs

	// persistent transport bar
	barPrev    *widget.Button
	barPlay    *widget.Button
	barNext    *widget.Button
	barTitle   *widget.Label
	barSeek    *widget.Slider
	barElapsed *widget.Label
	barTotal   *widget.Label

	stopTicker chan struct{}
	suppress   bool
}

// NewApp builds the window and starts the UI update loop.
func NewApp(fyneApp fyne.App, ctrl *Controller) *App {
	a := &App{
		fyneApp:    fyneApp,
		ctrl:       ctrl,
		stopTicker: make(chan struct{}),
	}
	a.win = fyneApp.NewWindow("Becoute")
	a.win.Resize(fyne.NewSize(1100, 720))

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Library", theme.StorageIcon(), a.buildLibraryTab()),
		container.NewTabItemWithIcon("Now Playing", theme.MediaPlayIcon(), a.buildPlayerTab()),
	)
	a.tabs = tabs
	// The transport sits under the tabs so playback can be controlled from any
	// tab, rather than only from Now Playing.
	a.win.SetContent(container.NewBorder(nil, a.buildTransportBar(), nil, nil, tabs))
	a.win.SetMaster()
	a.win.SetOnClosed(a.shutdown)

	a.refreshLibrary()
	a.refreshNowPlaying()
	a.refreshTransportBar()
	a.startTicker()
	return a
}

// Show displays the window.
func (a *App) Show() { a.win.Show() }

// shutdown persists progress and releases audio resources.
func (a *App) shutdown() {
	close(a.stopTicker)
	a.ctrl.Save()
	a.ctrl.Player().Close()
}

// startTicker runs a background loop that refreshes the transport display,
// saves progress periodically, and reacts to chapters finishing.
func (a *App) startTicker() {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-a.stopTicker:
				return
			case <-a.ctrl.Player().Drained():
				a.ctrl.HandleDrained()
				fyne.Do(func() {
					a.refreshNowPlaying()
					a.refreshLibrary()
				})
			case <-ticker.C:
				a.ctrl.SaveThrottled()
				fyne.Do(func() {
					a.updateTransport()
					a.refreshTransportBar()
				})
			}
		}
	}()
}

// --- Library tab ---

func (a *App) buildLibraryTab() fyne.CanvasObject {
	importFile := widget.NewButtonWithIcon("Add file", theme.ContentAddIcon(), a.pickFile)
	importDir := widget.NewButtonWithIcon("Add folder", theme.FolderOpenIcon(), a.pickFolder)
	transfer := widget.NewButtonWithIcon("Transfer", theme.StorageIcon(), a.showTransferMenu)

	// Create every widget before wiring callbacks: SetSelected fires its
	// handler immediately, and that handler reads the widgets below.
	a.statusLabel = widget.NewLabel("Add an audiobook to get started.")
	a.statusLabel.Wrapping = fyne.TextWrapWord
	a.statsLabel = widget.NewLabel("")
	a.statsLabel.TextStyle = fyne.TextStyle{Italic: true}
	a.selectedBookID = ""

	a.search = widget.NewEntry()
	a.search.SetPlaceHolder("Search title, author, narrator, or chapter…")
	a.search.OnChanged = func(string) { a.refreshLibrary() }

	a.filter = widget.NewSelect([]string{"All books", "In progress", "Not started", "Finished"}, func(string) {
		a.refreshLibrary()
	})

	a.sort = widget.NewSelect(SortOptions, func(string) { a.refreshLibrary() })

	a.libraryList = widget.NewList(
		func() int { return len(a.visibleBooks) },
		func() fyne.CanvasObject { return newBookRow() },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < 0 || id >= len(a.visibleBooks) {
				return
			}
			o.(*bookRow).update(a.visibleBooks[id], a.ctrl)
		},
	)
	a.libraryList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(a.visibleBooks) {
			return
		}
		// Selecting a book both shows its details and loads it into the
		// player, so the transport below and the Now Playing tab are live
		// without a second step.
		a.selectedBookID = a.visibleBooks[id].ID
		a.refreshDetails()
		a.loadSelected()
	}

	a.details = container.NewStack(container.NewCenter(widget.NewLabel("Select a book to see its details.")))

	// Defaults are applied after the callbacks exist so the initial refresh
	// sees a fully built tab.
	a.filter.SetSelected("All books")
	a.sort.SetSelected(sortRecent)

	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(importFile, importDir, transfer),
		container.NewHBox(a.sort, a.filter),
	)
	searchRow := container.NewBorder(nil, nil, nil, nil, a.search)

	listPane := container.NewBorder(searchRow, nil, nil, nil, a.libraryList)
	split := container.NewHSplit(listPane, a.details)
	split.SetOffset(0.55)

	bottom := container.NewVBox(a.statsLabel, a.statusLabel)
	return container.NewBorder(container.NewVBox(toolbar, widget.NewSeparator()), bottom, nil, nil, split)
}

// refreshDetails re-renders the details panel for the selected book.
func (a *App) refreshDetails() {
	if a.details == nil {
		return
	}
	book := a.selectedBook()
	a.details.Objects = []fyne.CanvasObject{a.bookDetails(book)}
	a.details.Refresh()
}

// selectedBook returns the book currently selected in the list, if any.
func (a *App) selectedBook() *library.Book {
	if a.selectedBookID == "" {
		return nil
	}
	book, err := a.ctrl.Book(a.selectedBookID)
	if err != nil {
		a.selectedBookID = ""
		return nil
	}
	return book
}

// showTransferMenu offers library export and import.
func (a *App) showTransferMenu() {
	exportItem := fyne.NewMenuItem("Export library…", a.exportLibrary)
	importItem := fyne.NewMenuItem("Import library…", a.importLibrary)
	menu := fyne.NewMenu("Transfer", exportItem, importItem)
	// Show the menu over the library list rather than the window origin.
	var anchor fyne.CanvasObject = a.libraryList
	if a.libraryList == nil {
		anchor = a.win.Canvas().Content()
	}
	popup := widget.NewPopUpMenu(menu, a.win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	popup.ShowAtPosition(pos)
}

func (a *App) exportLibrary() {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			a.setStatus("Export failed: " + err.Error())
			return
		}
		if w == nil {
			return
		}
		path := w.URI().Path()
		w.Close()
		if err := a.ctrl.ExportLibrary(path); err != nil {
			a.setStatus("Export failed: " + err.Error())
			return
		}
		a.setStatus("Exported library to " + path)
	}, a.win)
	d.SetFileName("becoute-library.json")
	d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	d.Show()
}

func (a *App) importLibrary() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			a.setStatus("Import failed: " + err.Error())
			return
		}
		if r == nil {
			return
		}
		path := r.URI().Path()
		r.Close()
		added, merged, err := a.ctrl.ImportLibrary(path)
		if err != nil {
			a.setStatus("Import failed: " + err.Error())
			return
		}
		a.refreshLibrary()
		a.setStatus(fmt.Sprintf("Imported %d new book(s) and merged %d.", added, merged))
	}, a.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	d.Show()
}

func (a *App) pickFile() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			a.setStatus("Open failed: " + err.Error())
			return
		}
		if r == nil {
			return
		}
		path := r.URI().Path()
		r.Close()
		a.setStatus(fmt.Sprintf("Importing %s…", path))
		book, err := a.ctrl.ImportFile(path)
		if err != nil {
			a.setStatus("Import failed: " + err.Error())
			return
		}
		a.refreshLibrary()
		a.setStatus(fmt.Sprintf("Imported %q (%d chapter(s)).", book.Title, len(book.Chapters)))
	}, a.win)
	d.SetFilter(storage.NewExtensionFileFilter(audio.SupportedExtensions))
	d.Show()
}

func (a *App) pickFolder() {
	d := dialog.NewFolderOpen(func(u fyne.ListableURI, err error) {
		if err != nil {
			a.setStatus("Open failed: " + err.Error())
			return
		}
		if u == nil {
			return
		}
		a.setStatus(fmt.Sprintf("Scanning %s…", u.Path()))
		books, err := a.ctrl.ImportDir(u.Path())
		if err != nil {
			a.setStatus("Import failed: " + err.Error())
			return
		}
		a.refreshLibrary()
		if len(books) == 0 {
			a.setStatus("No MP3 files found in that folder.")
			return
		}
		a.setStatus(fmt.Sprintf("Imported %d book(s).", len(books)))
	}, a.win)
	d.Show()
}

// refreshLibrary recomputes the visible book list from the current filter.
func (a *App) refreshLibrary() {
	filter := "All books"
	if a.filter != nil {
		filter = a.filter.Selected
	}
	var books []*library.Book
	switch filter {
	case "In progress":
		books = a.ctrl.ContinueListening()
	case "Finished":
		books = a.ctrl.Finished()
	case "Not started":
		for _, b := range a.ctrl.Books() {
			if _, ok := a.ctrl.Entry(b.ID); !ok {
				books = append(books, b)
			}
		}
	default:
		// All books: books in progress first, then the rest.
		seen := map[string]bool{}
		for _, b := range a.ctrl.ContinueListening() {
			books = append(books, b)
			seen[b.ID] = true
		}
		for _, b := range a.ctrl.Books() {
			if !seen[b.ID] {
				books = append(books, b)
			}
		}
	}
	// A query narrows whatever the filter produced.
	if query := a.searchQuery(); query != "" {
		kept := books[:0]
		for _, b := range books {
			if matchesQuery(b, query) {
				kept = append(kept, b)
			}
		}
		books = kept
	}

	mode := sortRecent
	if a.sort != nil && a.sort.Selected != "" {
		mode = a.sort.Selected
	}
	ApplySort(books, mode, a.ctrl.Entry)

	a.visibleBooks = books
	if a.libraryList != nil {
		a.libraryList.Refresh()
	}
	// Keep the details panel in step with the list, dropping the selection when
	// the selected book is filtered out.
	if a.selectedBookID != "" {
		found := false
		for _, b := range books {
			if b.ID == a.selectedBookID {
				found = true
				break
			}
		}
		if !found {
			a.selectedBookID = ""
			a.libraryList.UnselectAll()
		}
	}
	a.refreshDetails()
	a.updateStats()
}

// searchQuery returns the trimmed contents of the search box.
func (a *App) searchQuery() string {
	if a.search == nil {
		return ""
	}
	return strings.TrimSpace(a.search.Text)
}

// matchesQuery reports whether a book matches a free-text query.
func matchesQuery(b *library.Book, query string) bool {
	q := strings.ToLower(query)
	for _, field := range []string{b.Title, b.Author, b.Narrator, b.Album} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	for _, ch := range b.Chapters {
		if strings.Contains(strings.ToLower(ch.Title), q) {
			return true
		}
	}
	return false
}

func (a *App) updateStats() {
	if a.statsLabel == nil {
		return
	}
	all := a.ctrl.Books()
	var total time.Duration
	for _, b := range all {
		total += b.TotalDuration()
	}
	finished := len(a.ctrl.Finished())
	a.statsLabel.SetText(fmt.Sprintf("%d book(s) · %s of audio · %d finished",
		len(all), FormatDuration(total), finished))
}

// --- Now Playing tab ---

func (a *App) buildPlayerTab() fyne.CanvasObject {
	a.nowTitle = widget.NewLabel("Nothing loaded")
	a.nowTitle.TextStyle = fyne.TextStyle{Bold: true}
	a.nowTitle.Wrapping = fyne.TextWrapWord
	a.nowAuthor = widget.NewLabel("")
	a.nowAuthor.Wrapping = fyne.TextWrapWord

	a.chapterSelect = widget.NewSelect(nil, func(s string) {
		if a.suppress {
			return
		}
		for i, ch := range a.chapters {
			if ChapterLabel(i, ch) == s {
				a.ctrl.SelectChapter(i)
				break
			}
		}
		a.refreshNowPlaying()
	})

	a.seekSlider = widget.NewSlider(0, 1)
	a.seekSlider.Step = 0.001
	a.seekSlider.OnChangeEnded = func(v float64) {
		if a.suppress || a.loadedBookID == "" {
			return
		}
		pos := a.ctrl.Player().Position()
		if pos.Total <= 0 {
			return
		}
		a.ctrl.Seek(time.Duration(v * float64(pos.Total)))
	}
	a.elapsedLabel = widget.NewLabel("0:00")
	a.totalLabel = widget.NewLabel("0:00")

	a.bookProgress = widget.NewProgressBar()
	a.bookPercent = widget.NewLabel("0%")

	a.playButton = widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), func() {
		if err := a.ctrl.TogglePlay(); err != nil {
			a.setStatus(err.Error())
		}
		a.refreshNowPlaying()
	})
	a.prevButton = widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() {
		a.ctrl.PreviousChapter()
		a.refreshNowPlaying()
	})
	a.nextButton = widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() {
		if !a.ctrl.NextChapter() {
			a.setStatus("This is the last chapter.")
		}
		a.refreshNowPlaying()
		a.refreshTransportBar()
	})

	a.speedSelect = widget.NewSelect([]string{"0.75x", "1x", "1.25x", "1.5x", "1.75x", "2x", "2.5x", "3x"}, func(s string) {
		if a.suppress {
			return
		}
		var speed float64
		fmt.Sscanf(s, "%gx", &speed)
		if speed > 0 {
			a.ctrl.SetSpeed(speed)
		}
	})
	a.speedSelect.SetSelected(fmt.Sprintf("%gx", a.ctrl.Speed()))

	a.volumeSlider = widget.NewSlider(0, 1)
	a.volumeSlider.Step = 0.01
	a.volumeSlider.Value = a.ctrl.Volume()
	a.volumeSlider.OnChanged = func(v float64) {
		if a.suppress {
			return
		}
		a.ctrl.SetVolume(v)
	}

	a.autoplayChk = widget.NewCheck("Play next chapter automatically", func(v bool) {
		if a.suppress {
			return
		}
		a.ctrl.SetAutoplayNext(v)
	})
	a.autoplayChk.SetChecked(a.ctrl.AutoplayNext())

	transport := container.NewHBox(a.prevButton, a.playButton, a.nextButton)
	seekRow := container.NewBorder(nil, nil, a.elapsedLabel, a.totalLabel, a.seekSlider)
	progressRow := container.NewBorder(nil, nil, widget.NewLabel("Book progress"), a.bookPercent, a.bookProgress)
	controls := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Speed"), container.NewHBox(a.volumeSlider, a.autoplayChk), a.speedSelect),
	)

	header := container.NewVBox(a.nowTitle, a.nowAuthor)
	body := container.NewVBox(
		header,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel("Chapter"), nil, a.chapterSelect),
		seekRow,
		container.NewCenter(transport),
		widget.NewSeparator(),
		progressRow,
		widget.NewSeparator(),
		controls,
	)
	return container.NewVScroll(container.NewPadded(body))
}

// openBook loads a book into the player and switches to the Now Playing tab.
func (a *App) openBook(book *library.Book) {
	if err := a.ctrl.Load(book.ID); err != nil {
		a.setStatus("Could not load book: " + err.Error())
		return
	}
	a.refreshNowPlaying()
	a.setStatus(fmt.Sprintf("Loaded %q. Press Play to listen.", book.Title))
}

// refreshNowPlaying syncs every control with the player's current state.
func (a *App) refreshNowPlaying() {
	book := a.ctrl.CurrentBook()
	if book == nil {
		a.loadedBookID = ""
		a.suppress = true
		a.nowTitle.SetText("Nothing loaded")
		a.nowAuthor.SetText("Pick a book from the Library tab.")
		a.chapterSelect.Disable()
		a.seekSlider.Disable()
		a.playButton.Disable()
		a.prevButton.Disable()
		a.nextButton.Disable()
		a.bookProgress.SetValue(0)
		a.bookPercent.SetText("0%")
		a.elapsedLabel.SetText("0:00")
		a.totalLabel.SetText("0:00")
		a.suppress = false
		return
	}

	a.loadedBookID = book.ID
	a.suppress = true
	a.nowTitle.SetText(book.Title)
	author := book.DisplayAuthor()
	if book.Narrator != "" {
		author += " · narrated by " + book.Narrator
	}
	a.nowAuthor.SetText(author)

	a.chapters = book.Chapters
	labels := make([]string, len(book.Chapters))
	for i, ch := range book.Chapters {
		labels[i] = ChapterLabel(i, ch)
	}
	a.chapterSelect.Options = labels
	a.chapterSelect.Enable()

	pos := a.ctrl.Player().Position()
	if pos.Track >= 0 && pos.Track < len(labels) {
		a.chapterSelect.SetSelected(labels[pos.Track])
	}

	a.playButton.Enable()
	a.prevButton.Enable()
	a.nextButton.Enable()
	a.seekSlider.Enable()

	if pos.Total > 0 {
		a.seekSlider.Value = float64(pos.Elapsed) / float64(pos.Total)
	}
	a.elapsedLabel.SetText(FormatDuration(pos.Elapsed))
	a.totalLabel.SetText(FormatDuration(pos.Total))

	entry, ok := a.ctrl.Entry(book.ID)
	if ok {
		a.bookProgress.SetValue(entry.Percent)
		a.bookPercent.SetText(fmt.Sprintf("%.0f%%", entry.Percent*100))
	} else {
		a.bookProgress.SetValue(0)
		a.bookPercent.SetText("0%")
	}
	a.suppress = false

	a.updatePlayButton()
}

// updateTransport refreshes only the fast-changing playback widgets.
func (a *App) updateTransport() {
	if a.loadedBookID == "" {
		return
	}
	pos := a.ctrl.Player().Position()
	a.suppress = true
	if pos.Total > 0 {
		a.seekSlider.Value = float64(pos.Elapsed) / float64(pos.Total)
	}
	a.elapsedLabel.SetText(FormatDuration(pos.Elapsed))
	a.totalLabel.SetText(FormatDuration(pos.Total))
	a.suppress = false
	a.updatePlayButton()

	if book := a.ctrl.CurrentBook(); book != nil {
		if entry, ok := a.ctrl.Entry(book.ID); ok {
			a.bookProgress.SetValue(entry.Percent)
			a.bookPercent.SetText(fmt.Sprintf("%.0f%%", entry.Percent*100))
		}
	}
}

func (a *App) updatePlayButton() {
	if a.ctrl.Player().State() == audio.StatePlaying {
		a.playButton.SetText("Pause")
		a.playButton.SetIcon(theme.MediaPauseIcon())
	} else {
		a.playButton.SetText("Play")
		a.playButton.SetIcon(theme.MediaPlayIcon())
	}
}

func (a *App) setStatus(msg string) {
	if a.statusLabel != nil {
		a.statusLabel.SetText(strings.TrimSpace(msg))
	}
}

// --- Persistent transport ---

// buildTransportBar builds the always-visible playback bar shown under the
// tabs. It mirrors the Now Playing transport so playback can be controlled
// without switching tabs.
func (a *App) buildTransportBar() fyne.CanvasObject {
	a.barPrev = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), func() {
		a.ctrl.PreviousChapter()
		a.refreshNowPlaying()
		a.refreshTransportBar()
	})
	a.barPlay = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		if err := a.ctrl.TogglePlay(); err != nil {
			a.setStatus(err.Error())
			return
		}
		a.refreshNowPlaying()
		a.refreshTransportBar()
	})
	a.barNext = widget.NewButtonWithIcon("", theme.MediaSkipNextIcon(), func() {
		if !a.ctrl.NextChapter() {
			a.setStatus("This is the last chapter.")
		}
		a.refreshNowPlaying()
		a.refreshTransportBar()
	})

	a.barTitle = widget.NewLabel("Nothing loaded")
	a.barTitle.Truncation = fyne.TextTruncateEllipsis
	a.barTitle.TextStyle = fyne.TextStyle{Bold: true}

	a.barSeek = widget.NewSlider(0, 1)
	a.barSeek.Step = 0.001
	a.barSeek.OnChangeEnded = func(v float64) {
		if a.suppress || a.loadedBookID == "" {
			return
		}
		pos := a.ctrl.Player().Position()
		if pos.Total <= 0 {
			return
		}
		a.ctrl.Seek(time.Duration(v * float64(pos.Total)))
	}

	a.barElapsed = widget.NewLabel("0:00")
	a.barTotal = widget.NewLabel("0:00")

	controls := container.NewHBox(a.barPrev, a.barPlay, a.barNext)
	times := container.NewHBox(a.barElapsed, widget.NewLabel("/"), a.barTotal)
	// The title and seek bar share the centre column so the title gets the
	// width it needs, with the transport buttons and timecodes flanking them.
	centre := container.NewVBox(a.barTitle, a.barSeek)
	return container.NewVBox(
		widget.NewSeparator(),
		container.NewPadded(container.NewBorder(nil, nil, controls, times, centre)),
	)
}

// loadSelected loads the book selected in the library into the player.
func (a *App) loadSelected() {
	book := a.selectedBook()
	if book == nil {
		return
	}
	if a.ctrl.CurrentBookID() == book.ID {
		return
	}
	if err := a.ctrl.Load(book.ID); err != nil {
		a.setStatus("Could not load book: " + err.Error())
		return
	}
	a.refreshNowPlaying()
	a.refreshTransportBar()
}

// refreshTransportBar syncs the persistent transport with the player.
func (a *App) refreshTransportBar() {
	if a.barPlay == nil {
		return
	}
	book := a.ctrl.CurrentBook()
	a.suppress = true
	defer func() { a.suppress = false }()

	if book == nil {
		a.barTitle.SetText("Nothing loaded")
		a.barElapsed.SetText("0:00")
		a.barTotal.SetText("0:00")
		a.barSeek.Value = 0
		a.barPrev.Disable()
		a.barNext.Disable()
		a.barSeek.Disable()
	} else {
		a.barTitle.SetText(book.Title)
		pos := a.ctrl.Player().Position()
		if pos.Total > 0 {
			a.barSeek.Value = float64(pos.Elapsed) / float64(pos.Total)
		}
		a.barElapsed.SetText(FormatDuration(pos.Elapsed))
		a.barTotal.SetText(FormatDuration(pos.Total))
		a.barPrev.Enable()
		a.barNext.Enable()
		a.barSeek.Enable()
	}

	if a.ctrl.Player().State() == audio.StatePlaying {
		a.barPlay.SetIcon(theme.MediaPauseIcon())
	} else {
		a.barPlay.SetIcon(theme.MediaPlayIcon())
	}
}
