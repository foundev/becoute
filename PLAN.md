# Becoute Roadmap

Becoute is a dedicated audiobook player written in Go with [Fyne](https://fyne.io/).
It is designed to be cross-platform (macOS, Linux, Windows, Android, iOS) and to keep
your library and listening progress in sync across devices without a central server.

## Phase 1 — Local audiobook player with progress (this milestone)

Goal: a working desktop audiobook player that remembers where you stopped.

- [x] Project scaffold, `.gitignore`, CI for macOS / Linux / Windows
- [x] MP3 playback engine (decode, play, pause, seek, speed, volume)
- [x] Book model: a book is one or more ordered MP3 chapters with ID3 metadata
- [x] Library persisted as JSON, with add (file/folder) and remove
- [x] Progress tracking per book: chapter, position, percent complete, last played
- [x] Resume playback at the saved position, auto-advance to the next chapter
- [x] "Currently listening" + "Recently added" views with progress bars
- [x] Auto-save progress on pause, seek, chapter change, and app exit

## Phase 2 — Library management (this milestone)

Goal: make the library usable with a real collection of audiobooks.

- [x] Cover art extraction and display (ID3 APIC frames, plus sidecar images)
- [x] Manual metadata editing (title, author, narrator, album)
- [x] Search across title, author, narrator, album, and chapter titles
- [x] Sort by recently added, title, author, duration, or recently played
- [x] Chapter management: reorder, rename, remove, and rescan from disk
- [x] Merge books that were imported separately
- [x] Import/export the library as a portable JSON file
- [x] Details panel with cover, statistics, and per-book actions
- [x] Imports are idempotent: re-adding a book merges instead of duplicating
- [ ] Drag-and-drop reordering (arrows are used for now)
- [ ] Playlists / collections / tags
- [ ] Watch a folder for new files

## Phase 3 — LibriVox integration

- [ ] Search the LibriVox catalog from inside the app
- [ ] Download a book (all chapters) with a progress indicator
- [ ] Resume interrupted downloads
- [ ] Cache/verify downloaded files and clean up failed downloads
- [ ] Browse by genre, author, and "most popular"

## Phase 4 — Peer-to-peer sync

- [ ] Sync protocol for library metadata and progress (CRDT or last-write-wins)
- [ ] Device discovery on the local network
- [ ] Optional relay for syncing across networks
- [ ] Conflict resolution when two devices change the same book
- [ ] End-to-end encryption of synced payloads

## Phase 5 — Listening statistics

- [ ] Time listened per day/week/month
- [ ] Listening streaks
- [ ] Books finished, total hours, average speed
- [ ] Estimated time remaining per book
- [ ] Charts and an exportable listening history

## Phase 6 — Mobile and packaging

- [ ] Touch-optimized layouts for phone and tablet
- [ ] Android build and signing
- [ ] iOS build and signing
- [ ] Background playback and lock-screen / media-key controls
- [ ] Signed desktop installers (.dmg, .msi, .AppImage) and release automation
- [ ] Auto-update

## Phase 7 — Quality of life

- [ ] Sleep timer and "stop at end of chapter"
- [ ] Bookmarks and notes
- [ ] Variable speed presets (0.8x – 3x)
- [ ] Keyboard shortcuts and media-key support
- [ ] Multiple simultaneous libraries / profiles
- [ ] Accessibility pass (screen readers, scalable UI)
