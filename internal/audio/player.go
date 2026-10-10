package audio

import (
	"errors"
	"math"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
)

// SpeakerSampleRate is the fixed output rate the speaker device is opened at.
// Every decoded file is resampled to this rate, so playback speed can be
// changed on the fly without re-initializing the device.
const SpeakerSampleRate beep.SampleRate = 44100

// speakerBufferSamples is how many samples of headroom the device buffer keeps.
var speakerBufferSamples = SpeakerSampleRate.N(time.Second / 2)

// State describes what the player is currently doing.
type State int

const (
	StateStopped State = iota
	StatePlaying
	StatePaused
)

// Position reports the current playback position within a track.
type Position struct {
	// Track is the index of the playing track within the current queue.
	Track int
	// Elapsed is the playback offset within that track.
	Elapsed time.Duration
	// Total is the track length, or zero when unknown.
	Total time.Duration
}

// track is one decoded, seekable audio file in the queue.
type track struct {
	path   string
	stream beep.StreamSeekCloser
	format beep.Format
	resamp *beep.Resampler
	volume *effects.Volume
	ctrl   *beep.Ctrl
	// armed is true once the track has been handed to the speaker. A drained
	// track must be reopened before it can be played again.
	armed   bool
	drained bool
}

func (t *track) duration() time.Duration {
	if t.format.SampleRate <= 0 {
		return 0
	}
	return t.format.SampleRate.D(t.stream.Len())
}

// Player plays a queue of audio files, one after another, through the system
// speakers. It is safe for concurrent use.
//
// Playback advances one track at a time. When a track drains, the audio
// goroutine pushes its index onto a channel without taking any lock; the owner
// of the player reads Drained and calls Next to continue. This keeps the audio
// callback free of locks, which is required because the speaker holds its own
// lock while streaming.
type Player struct {
	mu     sync.Mutex
	tracks []*track
	index  int
	state  State
	speed  float64
	volume float64

	// drainedChan receives the index of a track that has just finished
	// streaming. It is written from the audio goroutine without taking any
	// lock, and read by the owner of the player, which then decides whether to
	// advance.
	drainedChan chan int
}

var speakerOnce sync.Once
var speakerErr error

// InitSpeaker initializes the audio device once per process.
func InitSpeaker() error {
	speakerOnce.Do(func() {
		speakerErr = speaker.Init(SpeakerSampleRate, speakerBufferSamples)
	})
	return speakerErr
}

// NewPlayer returns an idle player at normal speed and full volume.
func NewPlayer() *Player {
	return &Player{speed: 1.0, volume: 1.0, drainedChan: make(chan int, 64)}
}

// Drained returns a channel that receives the index of a track each time that
// track finishes streaming.
func (p *Player) Drained() <-chan int {
	return p.drainedChan
}

// SetQueue replaces the player's queue, stops playback, and positions at track
// 0. When startAt names a track index and offset, the queue begins there.
func (p *Player) SetQueue(paths []string, startAt int, offset time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	speaker.Clear()

	var tracks []*track
	for _, path := range paths {
		t, err := p.openLocked(path)
		if err != nil {
			for _, opened := range tracks {
				_ = opened.stream.Close()
			}
			return err
		}
		tracks = append(tracks, t)
	}
	p.tracks = tracks
	if startAt < 0 || startAt >= len(tracks) {
		startAt = 0
	}
	p.index = startAt
	p.applySettingsLocked()
	if len(tracks) > 0 && offset > 0 {
		p.seekLocked(offset)
	}
	return nil
}

// openLocked decodes a file and wraps it in the speed and volume stages.
func (p *Player) openLocked(path string) (*track, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stream, format, err := mp3.Decode(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	t := &track{path: path, stream: stream, format: format}
	t.resamp = beep.Resample(4, format.SampleRate, SpeakerSampleRate, stream)
	t.volume = &effects.Volume{Streamer: t.resamp, Base: 2, Volume: volumeToExponent(p.volume), Silent: p.volume <= 0}
	t.ctrl = &beep.Ctrl{Streamer: t.volume, Paused: true}
	return t, nil
}

// Play starts or resumes playback of the current track, first seeking to offset
// when it is greater than zero.
func (p *Player) Play(offset time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tracks) == 0 {
		return errors.New("nothing to play")
	}
	if p.state == StatePlaying {
		return nil
	}
	t := p.tracks[p.index]
	if t.drained {
		if err := p.reopenLocked(p.index); err != nil {
			return err
		}
		t = p.tracks[p.index]
	}
	if offset > 0 {
		p.seekLocked(offset)
	}
	if !t.armed {
		index := p.index
		speaker.Play(beep.Seq(t.ctrl, beep.Callback(func() { p.signalDrained(index) })))
		t.armed = true
	}
	speaker.Lock()
	t.ctrl.Paused = false
	speaker.Unlock()
	p.state = StatePlaying
	return nil
}

// Pause pauses playback, keeping the current position.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != StatePlaying || len(p.tracks) == 0 {
		return
	}
	t := p.tracks[p.index]
	speaker.Lock()
	t.ctrl.Paused = true
	speaker.Unlock()
	p.state = StatePaused
}

// Stop pauses playback and rewinds the current track to the beginning.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Player) stopLocked() {
	if len(p.tracks) == 0 {
		p.state = StateStopped
		return
	}
	t := p.tracks[p.index]
	p.disarmLocked()
	_ = t.stream.Seek(0)
	p.state = StateStopped
}

// Toggle pauses when playing and resumes when paused or stopped.
func (p *Player) Toggle(offset time.Duration) {
	if p.State() == StatePlaying {
		p.Pause()
		return
	}
	_ = p.Play(offset)
}

// Next moves to the next track in the queue and pauses playback. It reports
// whether there was a next track.
func (p *Player) Next(offset time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index+1 >= len(p.tracks) {
		return false
	}
	p.disarmLocked()
	p.index++
	p.state = StateStopped
	if offset > 0 {
		p.seekLocked(offset)
	}
	return true
}

// Previous moves to the previous track, or rewinds the current one when already
// at the first track.
func (p *Player) Previous(offset time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tracks) == 0 {
		return
	}
	p.disarmLocked()
	if p.index > 0 {
		p.index--
	} else {
		_ = p.tracks[0].stream.Seek(0)
	}
	p.state = StateStopped
	if offset > 0 {
		p.seekLocked(offset)
	}
}

// disarmLocked removes every track from the speaker and marks them unarmed, so
// that switching chapters does not leave paused streamers accumulating in the
// mixer. Clearing the mixer does not run a streamer's completion callback, so
// this never looks like a chapter finishing. The caller must hold p.mu.
func (p *Player) disarmLocked() {
	speaker.Clear()
	for _, t := range p.tracks {
		t.armed = false
	}
}

// SetTrack switches to a different track in the queue. When the player was
// playing, the new track starts playing from offset.
func (p *Player) SetTrack(index int, offset time.Duration) {
	p.mu.Lock()
	if index < 0 || index >= len(p.tracks) {
		p.mu.Unlock()
		return
	}
	playing := p.state == StatePlaying
	p.disarmLocked()
	p.index = index
	p.state = StateStopped
	if offset > 0 {
		p.seekLocked(offset)
	}
	p.mu.Unlock()

	if playing {
		_ = p.Play(0)
	}
}

// Seek moves the play head within the current track.
func (p *Player) Seek(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seekLocked(d)
}

func (p *Player) seekLocked(d time.Duration) {
	if len(p.tracks) == 0 {
		return
	}
	t := p.tracks[p.index]
	if t.format.SampleRate <= 0 || t.drained {
		return
	}
	target := t.format.SampleRate.N(d)
	if target < 0 {
		target = 0
	}
	if target > t.stream.Len() {
		target = t.stream.Len()
	}
	_ = t.stream.Seek(target)
}

// reopenLocked decodes the track at index again, which is necessary before
// replaying a track that has already drained.
func (p *Player) reopenLocked(index int) error {
	old := p.tracks[index]
	path := old.path
	fresh, err := p.openLocked(path)
	if err != nil {
		return err
	}
	_ = old.stream.Close()
	p.tracks[index] = fresh
	return nil
}

// SetSpeed changes the playback rate (1.0 is normal).
func (p *Player) SetSpeed(speed float64) {
	if speed <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.speed = speed
	p.applySettingsLocked()
}

// SetVolume sets the output volume from 0 (muted) to 1 (full).
func (p *Player) SetVolume(volume float64) {
	volume = math.Max(0, math.Min(1, volume))
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = volume
	p.applySettingsLocked()
}

// Speed returns the current playback rate.
func (p *Player) Speed() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.speed
}

// Volume returns the current output volume.
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

// applySettingsLocked pushes speed and volume onto the active track. The caller
// must hold p.mu.
func (p *Player) applySettingsLocked() {
	if len(p.tracks) == 0 {
		return
	}
	t := p.tracks[p.index]
	speaker.Lock()
	t.resamp.SetRatio(p.speed)
	t.volume.Volume = volumeToExponent(p.volume)
	t.volume.Silent = p.volume <= 0
	speaker.Unlock()
}

// signalDrained is invoked from the audio goroutine when a track drains. It
// must not block or take any lock, because the speaker holds its lock while
// streaming, so the send is best-effort onto a buffered channel.
func (p *Player) signalDrained(index int) {
	select {
	case p.drainedChan <- index:
	default:
	}
}

// Complete marks the currently playing track as drained, which happens when the
// owner of the player observes Drained and the queue is at its last track.
func (p *Player) Complete() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tracks) == 0 {
		return
	}
	p.tracks[p.index].drained = true
	p.state = StateStopped
}

// Position returns where playback currently is.
func (p *Player) Position() Position {
	p.mu.Lock()
	defer p.mu.Unlock()
	pos := Position{Track: p.index}
	if len(p.tracks) == 0 {
		return pos
	}
	t := p.tracks[p.index]
	pos.Total = t.duration()
	if t.format.SampleRate > 0 {
		pos.Elapsed = t.format.SampleRate.D(t.stream.Position())
	}
	return pos
}

// State returns the current playback state.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// TrackCount returns the number of tracks in the queue.
func (p *Player) TrackCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.tracks)
}

// TrackPath returns the path of the track at index, or "" when out of range.
func (p *Player) TrackPath(index int) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if index < 0 || index >= len(p.tracks) {
		return ""
	}
	return p.tracks[index].path
}

// Close stops playback and releases every decoded file.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	speaker.Clear()
	for _, t := range p.tracks {
		if t.stream != nil {
			_ = t.stream.Close()
		}
	}
	p.tracks = nil
}

// volumeToExponent maps a 0..1 volume to the effects.Volume exponent for a base
// of 2, giving a natural-feeling taper.
func volumeToExponent(v float64) float64 {
	switch {
	case v <= 0:
		return -60
	case v >= 1:
		return 0
	default:
		return math.Log2(v)
	}
}
