# Becoute

A dedicated audiobook reader written in Go with [Fyne](https://fyne.io/).

Becoute is built for listening to audiobooks the way you read books: it remembers
exactly where you stopped, in which chapter, and how far through the book you are.
It runs on macOS, Linux, Windows, Android, and iOS, and will eventually sync your
library and progress peer to peer between your own devices.

## Features

- MP3 playback with play/pause, chapter navigation, seeking, speed control, and volume
- A transport bar pinned below the tabs, so playback is controllable from any view
- A book is one or more ordered MP3 chapters, read from ID3 tags
- Import a single file (siblings in the same folder and album are grouped
  automatically) or a whole folder
- Progress tracking per book: chapter, position, percent complete, and last played
- Resumes where you left off and auto-advances to the next chapter
- Library views for in-progress, not started, and finished books
- Cover art from ID3 tags or images beside the audio files
- Search and sort across a large library
- Editable metadata, chapter management, and safe re-imports
- Local-first: no account and no server required

## Managing your library

- **Add file / Add folder** imports MP3s. Files that share an album are grouped
  into one book with chapters in natural order, so "Chapter 2" comes before
  "Chapter 10". Importing the same book twice merges rather than duplicates.
- **Search** matches titles, authors, narrators, albums, and chapter titles.
- **Sort** by recently added, title, author, duration, or recently played.
- **Details** shows the cover, length, and progress of the selected book, with
  actions to play, edit metadata, manage chapters, rescan, merge, or remove.
  Selecting a book also loads it, so the transport bar is ready immediately.
- **Chapters** can be reordered, renamed, or removed. A manual order is kept
  when the book is rescanned.
- **Rescan files** refreshes durations and titles, drops files that have moved
  away, and picks up new chapters added to the same folder.
- **Transfer** exports your library to a JSON file and imports one back.

Cover art is read from embedded ID3 pictures, falling back to a `cover.jpg` or
similar image stored next to the audio files. Extracted covers are cached under
the data directory. Becoute never modifies or moves your audio files.

## Status

Phase 1 (a local audiobook player that tracks progress) and Phase 2 (library
management) are implemented. See [PLAN.md](PLAN.md) for the full roadmap,
including LibriVox downloads, peer-to-peer sync, listening statistics, and
mobile packaging.

## Requirements

- Go 1.27 or newer
- A C toolchain, because Fyne uses OpenGL through cgo
  - macOS: Xcode command line tools (`xcode-select --install`)
  - Linux: `gcc`, `pkg-config`, and the OpenGL/X11 headers, for example
    `sudo apt-get install gcc pkg-config libgl1-mesa-dev xorg-dev`
  - Windows: TDM-GCC or MSYS2 with `gcc`

## Build and run

```sh
go build ./...
go run ./cmd/becoute
```

You can also import at startup by passing files or folders:

```sh
go run ./cmd/becoute ~/Audiobooks/SomeBook.mp3
go run ./cmd/becoute ~/Audiobooks
```

## Where your data lives

Becoute keeps a JSON library and a JSON progress file in your user config
directory:

- macOS: `~/Library/Application Support/Becoute/`
- Linux: `~/.config/Becoute/`
- Windows: `%AppData%\Becoute\`

Set `BECOUTE_DATA_DIR` to override the location (see `.env.example`). Cover art
is cached in a `covers/` subdirectory. The audio files themselves are never
modified or moved.

## Tests

```sh
go test ./...
go test -race ./...
```

Two integration tests drive real audio output. They skip themselves when no
audio device is available (for example inside a sandbox) and under `-race`,
because the `oto` audio driver used for playback races with itself while
starting up, which the detector reports inside that library rather than in
Becoute. Run them without `-race` to exercise playback end to end:

```sh
go test ./internal/audio/ -run TestRealPlayback -v
BECOUTE_AUDIO_IT=1 go test ./internal/ui/ -run TestRealPlayerDrivesUI -v
```

## Continuous integration

`.github/workflows/ci.yml` runs formatting checks, `go vet`, and the race-enabled
test suite on macOS, Linux, and Windows for every push and pull request.

`.github/workflows/release.yml` packages the app with the `fyne` CLI for macOS
(arm64 and amd64), Linux (amd64 and arm64), and Windows, and publishes the
artifacts to a GitHub release when a `v*` tag is pushed.

## License

MIT. See [LICENSE](LICENSE).
