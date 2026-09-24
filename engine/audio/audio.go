// Package audio mixes sounds in Go and plays them through the OS audio device
// (ebitengine/oto: WASAPI on Windows, no cgo). The Mixer is plain Go and can be
// tested without a device; Open connects one to the speakers.
package audio

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// SampleRate is the mixer's output rate. Everything is stereo float32.
const SampleRate = 48000

// Sound is decoded audio: interleaved stereo samples at SampleRate.
type Sound struct {
	samples []float32
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
}

// Mixer sums playing voices into one stereo stream. It is safe to call from
// the game thread while the audio device reads from it.
type Mixer struct {
	mu     sync.Mutex
	voices []voice
	next   Voice
	volume float32
}

func NewMixer() *Mixer { return &Mixer{volume: 1} }

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
	m.next++
	m.voices = append(m.voices, voice{
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
		for f := 0; f < frames; f++ {
			if v.pos >= len(src) {
				if !v.loop {
					break
				}
				v.pos = 0
			}
			out[2*f] += src[v.pos] * v.gainL * m.volume
			out[2*f+1] += src[v.pos+1] * v.gainR * m.volume
			v.pos += 2
		}
		if v.pos < len(src) || v.loop {
			alive = append(alive, v)
		}
	}
	clear(m.voices[len(alive):]) // drop references to finished sounds
	m.voices = alive
}

// Read implements io.Reader for the audio device: little-endian float32
// stereo, clamped to [-1, 1]. It always fills whole frames and never blocks.
func (m *Mixer) Read(p []byte) (int, error) {
	n := len(p) / 8 * 8
	buf := make([]float32, n/4)
	m.Mix(buf)
	for i, v := range buf {
		binary.LittleEndian.PutUint32(p[4*i:], math.Float32bits(max(-1, min(1, v))))
	}
	return n, nil
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
	p.Play()
	return &Device{player: p}, nil
}

func (d *Device) Close() error {
	if d == nil {
		return nil
	}
	return d.player.Close()
}
