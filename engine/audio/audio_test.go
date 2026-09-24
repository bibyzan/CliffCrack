package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func constant(frames int, v float32) *Sound {
	mono := make([]float32, frames)
	for i := range mono {
		mono[i] = v
	}
	return FromMono(mono)
}

func TestMixAndPan(t *testing.T) {
	m := NewMixer()
	m.Play(constant(4, 0.5), 1, -1) // hard left
	m.Play(constant(2, 0.5), 1, 1)  // hard right, shorter

	out := make([]float32, 8) // 4 frames
	m.Mix(out)
	for f := 0; f < 4; f++ {
		wantR := float32(0)
		if f < 2 {
			wantR = 0.5
		}
		if !near(out[2*f], 0.5) || !near(out[2*f+1], wantR) {
			t.Errorf("frame %d = (%v, %v), want (0.5, %v)", f, out[2*f], out[2*f+1], wantR)
		}
	}
	if m.Playing() != 0 {
		t.Errorf("%d voices still playing after both sounds ended", m.Playing())
	}

	// Centre pan is equal power: each side at cos(pi/4).
	m.Play(constant(1, 1), 1, 0)
	out = make([]float32, 2)
	m.Mix(out)
	if want := float32(math.Sqrt2 / 2); !near(out[0], want) || !near(out[1], want) {
		t.Errorf("centre pan = %v, want both %v", out, want)
	}
}

func TestStopLoopAndVolume(t *testing.T) {
	m := NewMixer()
	v := m.Loop(constant(3, 1), 0.5, -1)
	out := make([]float32, 20) // 10 frames: loops past the 3-frame sound
	m.Mix(out)
	for f := 0; f < 10; f++ {
		if !near(out[2*f], 0.5) {
			t.Fatalf("looping voice silent at frame %d", f)
		}
	}
	m.SetVolume(0.5)
	out = make([]float32, 2)
	m.Mix(out)
	if !near(out[0], 0.25) {
		t.Errorf("with master volume 0.5, sample = %v, want 0.25", out[0])
	}
	m.Stop(v)
	if m.Playing() != 0 {
		t.Error("Stop should remove the voice")
	}
	m.Stop(12345) // unknown: no-op
}

func TestReadClampsAndFillsWholeFrames(t *testing.T) {
	m := NewMixer()
	m.Play(constant(2, 0.9), 1, -1)
	m.Play(constant(2, 0.9), 1, -1) // sums to 1.8: must clamp to 1
	p := make([]byte, 21)           // 2 whole frames (16 bytes) + junk
	n, err := m.Read(p)
	if err != nil || n != 16 {
		t.Fatalf("Read = %d, %v; want 16, nil", n, err)
	}
	if l := math.Float32frombits(binary.LittleEndian.Uint32(p[0:])); l != 1 {
		t.Errorf("left sample = %v, want clamped 1", l)
	}
}

// wav builds a minimal RIFF/WAVE file with 16-bit PCM samples.
func wav(rate uint32, channels uint16, samples []int16) []byte {
	var b bytes.Buffer
	data := len(samples) * 2
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(4+8+16+8+2+8+data))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	for _, v := range []any{uint32(16), uint16(1), channels, rate, rate * uint32(channels) * 2, channels * 2, uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("LIST") // an unrelated chunk to skip
	binary.Write(&b, binary.LittleEndian, uint32(2))
	b.Write([]byte{0, 0})
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(data))
	binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

func TestLoadWAV(t *testing.T) {
	// Stereo at the mixer rate: decoded verbatim.
	s, err := LoadWAV(bytes.NewReader(wav(SampleRate, 2, []int16{16384, -16384, 32767, 0})))
	if err != nil {
		t.Fatal(err)
	}
	if s.Frames() != 2 || !near(s.samples[0], 0.5) || !near(s.samples[1], -0.5) {
		t.Errorf("stereo decode = %v", s.samples)
	}

	// Mono at half the rate: duplicated to both channels and resampled to double length.
	s, err = LoadWAV(bytes.NewReader(wav(SampleRate/2, 1, []int16{0, 16384, 16384, 0})))
	if err != nil {
		t.Fatal(err)
	}
	if s.Frames() != 8 {
		t.Fatalf("resampled frames = %d, want 8", s.Frames())
	}
	if !near(s.samples[2], 0.25) || s.samples[2] != s.samples[3] {
		t.Errorf("interpolated frame 1 = (%v, %v), want (0.25, 0.25)", s.samples[2], s.samples[3])
	}

	if _, err := LoadWAV(bytes.NewReader([]byte("not a wav file at all"))); err == nil {
		t.Error("garbage should fail to decode")
	}
}

func TestBlip(t *testing.T) {
	s := Blip(100*time.Millisecond, 800, 400, 0.5)
	if s.Duration() != 100*time.Millisecond {
		t.Errorf("duration = %v", s.Duration())
	}
	var peak float32
	for _, v := range s.samples {
		peak = max(peak, float32(math.Abs(float64(v))))
	}
	if peak <= 0.1 || peak > 0.5 {
		t.Errorf("peak = %v, want (0.1, 0.5]", peak)
	}
}
