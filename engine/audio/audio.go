// Package audio mixes sounds in Go and plays them through the OS audio device
// (ebitengine/oto: WASAPI on Windows, no cgo). The Mixer is plain Go and can be
// tested without a device; Open connects one to the speakers.
package audio

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// SampleRate is the mixer's output rate. Everything is stereo float32.
const SampleRate = 48000

// Sound is decoded audio: interleaved stereo samples at SampleRate.
type Sound struct {
	samples []float32
	alts    []*Sound // other takes of it (see Variants): each play picks one
}

// Variants makes n takes of a sound, by take(i), as one Sound: each time
// it's played a take is picked at random, so a sound heard over and over
// (a rifle's shots) doesn't repeat itself note for note.
func Variants(n int, take func(i int) *Sound) *Sound {
	first := take(0)
	for i := 1; i < n; i++ {
		first.alts = append(first.alts, take(i))
	}
	return first
}

// NewSound wraps interleaved stereo samples at SampleRate.
func NewSound(stereo []float32) *Sound { return &Sound{samples: stereo} }

// FromMono duplicates mono samples (at SampleRate) into both channels.
func FromMono(mono []float32) *Sound {
	s := make([]float32, 2*len(mono))
	for i, v := range mono {
		s[2*i], s[2*i+1] = v, v
	}
	return &Sound{samples: s}
}

// Frames is the number of stereo sample pairs.
func (s *Sound) Frames() int { return len(s.samples) / 2 }

func (s *Sound) Duration() time.Duration {
	return time.Duration(s.Frames()) * time.Second / SampleRate
}

// Voice identifies a playing sound. The zero Voice is never used.
type Voice uint64

type voice struct {
	id           Voice
	sound        *Sound
	pos          int // next sample index (interleaved)
	gainL, gainR float32
	loop         bool
	group        int // choke group (0: none)
	fade         int // frames left of a fade-out (0: not fading)
	wait         int // frames of silence before it starts (see Mixer.start)
}

// Mixing limits.
const (
	maxVoices  = 40                    // beyond this the oldest is faded out to make room
	fadeFrames = SampleRate * 6 / 1000 // a choked or stolen voice fades over 6 ms (no click)
	limitCeil  = 0.95                  // the limiter keeps the output under this
	limitRel   = 0.08                  // s for the limiter to recover after a peak
	busGain    = 0.8                   // headroom before the limiter

	playerBuffer = SampleRate * 20 / 1000 // frames the device reads ahead of what's heard (20 ms)
	maxWait      = SampleRate * 25 / 1000 // frames a sound is ever held back to keep its timing (see start)
)

// Mixer sums playing voices into one stereo stream. It is safe to call from
// the game thread while the audio device reads from it.
type Mixer struct {
	mu     sync.Mutex
	voices []voice
	next   Voice
	volume float32
	rng    *rand.Rand

	// The output limiter (see Read), used only by the audio device's reads.
	limit float32 // current gain, 0..1
	buf   []float32

	// When the device last read, and how much: a sound started between
	// reads is placed as far into the next read as it came after the last,
	// so sounds keep their timing rather than all starting on the next
	// block's first frame (see start).
	readAt     time.Time
	readFrames int
}

func NewMixer() *Mixer { return &Mixer{volume: 1, rng: rand.New(rand.NewPCG(7, 11)), limit: 1} }

// PlayChoked starts s once in choke group group (non-zero): whatever is
// still playing in that group fades out at once, as a new shot from a gun
// cuts off the tail of the last. Rapid fire then stays a clean run of
// shots rather than a smear of overlapping ones.
func (m *Mixer) PlayChoked(s *Sound, volume, pan float32, group int) Voice {
	m.mu.Lock()
	for i := range m.voices {
		if v := &m.voices[i]; v.group == group && v.fade == 0 {
			v.fade = fadeFrames
		}
	}
	m.mu.Unlock()
	id := m.start(s, volume, pan, false)
	m.mu.Lock()
	if n := len(m.voices); n > 0 && m.voices[n-1].id == id {
		m.voices[n-1].group = group
	}
	m.mu.Unlock()
	return id
}

// Play starts s once. volume is linear gain; pan is -1 (left) .. 1 (right).
func (m *Mixer) Play(s *Sound, volume, pan float32) Voice { return m.start(s, volume, pan, false) }

// Loop plays s repeatedly until Stop.
func (m *Mixer) Loop(s *Sound, volume, pan float32) Voice { return m.start(s, volume, pan, true) }

func (m *Mixer) start(s *Sound, volume, pan float32, loop bool) Voice {
	if s == nil || len(s.samples) == 0 {
		return 0
	}
	// Equal-power pan: centre is -3 dB per side, hard left/right is full gain.
	p := (max(-1, min(1, pan)) + 1) * math.Pi / 4
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(s.alts) > 0 {
		if k := m.rng.IntN(len(s.alts) + 1); k > 0 {
			s = s.alts[k-1]
		}
	}
	// Too many at once: fade the oldest one-shot out to make room.
	if live := len(m.voices); live >= maxVoices {
		for i := range m.voices {
			if v := &m.voices[i]; !v.loop && v.fade == 0 {
				v.fade = fadeFrames
				break
			}
		}
	}
	// Where in the next block to start: as far in as it is since the last
	// read began. The device reads whole blocks (tens of ms), and without
	// this a gun firing every 75 ms would have its shots land on the
	// block boundaries, unevenly: a stutter.
	wait := 0
	if !m.readAt.IsZero() {
		wait = min(int(time.Since(m.readAt).Seconds()*SampleRate), m.readFrames, maxWait)
	}
	m.next++
	m.voices = append(m.voices, voice{
		wait:  wait,
		id:    m.next,
		sound: s,
		gainL: volume * float32(math.Cos(float64(p))),
		gainR: volume * float32(math.Sin(float64(p))),
		loop:  loop,
	})
	return m.next
}

// Stop ends a voice early. Unknown or finished voices are ignored.
func (m *Mixer) Stop(v Voice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.voices {
		if m.voices[i].id == v {
			m.voices = append(m.voices[:i], m.voices[i+1:]...)
			return
		}
	}
}

// Playing is the number of active voices.
func (m *Mixer) Playing() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voices)
}

// SetVolume sets the master gain (linear).
func (m *Mixer) SetVolume(v float32) {
	m.mu.Lock()
	m.volume = v
	m.mu.Unlock()
}

// Mix adds the next len(out)/2 stereo frames into out (interleaved) and
// advances every voice. out should be zeroed by the caller for a fresh mix.
func (m *Mixer) Mix(out []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	frames := len(out) / 2
	alive := m.voices[:0]
	for _, v := range m.voices {
		src := v.sound.samples
		done := false
		f := 0
		if v.wait > 0 {
			skip := min(v.wait, frames)
			v.wait -= skip
			f = skip
		}
		for ; f < frames; f++ {
			if v.pos >= len(src) {
				if !v.loop {
					break
				}
				v.pos = 0
			}
			g := m.volume
			if v.fade > 0 {
				g *= float32(v.fade) / fadeFrames
				v.fade--
				if v.fade == 0 {
					done = true
					break
				}
			}
			out[2*f] += src[v.pos] * v.gainL * g
			out[2*f+1] += src[v.pos+1] * v.gainR * g
			v.pos += 2
		}
		if !done && (v.pos < len(src) || v.loop) {
			alive = append(alive, v)
		}
	}
	clear(m.voices[len(alive):]) // drop references to finished sounds
	m.voices = alive
}

// Read implements io.Reader for the audio device: little-endian float32
// stereo. It always fills whole frames and never blocks. The mix goes
// through a limiter rather than being clipped: when many sounds pile up
// (a burst of fire, its hits and splats) the whole mix is turned down at
// once and eased back up over limitRel, instead of every peak being
// sheared off, which crackles.
func (m *Mixer) Read(p []byte) (int, error) {
	n := len(p) / 8 * 8
	if cap(m.buf) < n/4 {
		m.buf = make([]float32, n/4) // (kept: no garbage on the audio thread)
	}
	buf := m.buf[:n/4]
	clear(buf)
	m.mu.Lock()
	m.readAt, m.readFrames = time.Now(), n/8
	m.mu.Unlock()
	m.Mix(buf)
	m.limitStereo(buf)
	for i, v := range buf {
		binary.LittleEndian.PutUint32(p[4*i:], math.Float32bits(max(-1, min(1, v))))
	}
	return n, nil
}

// limitStereo applies the bus gain and the limiter to interleaved frames in
// place: the gain drops instantly to keep each frame under limitCeil, and
// recovers smoothly.
func (m *Mixer) limitStereo(buf []float32) {
	rel := float32(1 - math.Exp(-1/(limitRel*SampleRate)))
	for f := 0; f+1 < len(buf); f += 2 {
		l, r := buf[f]*busGain, buf[f+1]*busGain
		peak := max(abs32(l), abs32(r))
		want := float32(1)
		if peak > limitCeil {
			want = limitCeil / peak
		}
		if want < m.limit {
			m.limit = want
		} else {
			m.limit += (want - m.limit) * rel
		}
		buf[f], buf[f+1] = l*m.limit, r*m.limit
	}
}

// Device plays a Mixer on the default output device.
type Device struct {
	player *oto.Player
}

// Open starts playback of m. Only one Device can exist per process.
func Open(m *Mixer) (*Device, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   SampleRate,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		BufferSize:   40 * time.Millisecond, // latency vs. robustness
	})
	if err != nil {
		return nil, err
	}
	<-ready
	p := ctx.NewPlayer(m)
	// oto's player reads ahead from the mixer into a buffer of its own,
	// half a second of sound by default, and a sound started now would only
	// be heard behind all of it: a gunshot half a second (or more) late.
	// Keep it to a few ms, just enough to not run dry.
	p.SetBufferSize(playerBuffer * 8) // stereo float32 frames
	p.Play()
	return &Device{player: p}, nil
}

func (d *Device) Close() error {
	if d == nil {
		return nil
	}
	return d.player.Close()
}
