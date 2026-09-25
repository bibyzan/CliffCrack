package audio

import (
	"math"
	"testing"
	"time"
)

func TestSynthLayersMixToVolume(t *testing.T) {
	s := Synth(0.8,
		Layer{Wave: Noise, Length: 80 * time.Millisecond, Decay: 40, Volume: 1, LowPass: 3000},
		Layer{Wave: Sine, Start: 200, End: 80, Delay: 20 * time.Millisecond, Length: 120 * time.Millisecond, Decay: 20, Volume: 1},
	)
	if want := int(0.14 * SampleRate); abs(s.Frames()-want) > 2 {
		t.Errorf("%d frames, want the longest layer's end (%d)", s.Frames(), want)
	}
	peak := float32(0)
	for _, v := range s.samples {
		if math.IsNaN(float64(v)) {
			t.Fatal("NaN in the mix")
		}
		peak = max(peak, abs32(v))
	}
	if math.Abs(float64(peak-0.8)) > 1e-3 {
		t.Errorf("peak %v, want the volume 0.8", peak)
	}
	// It ends quietly (faded, not cut off).
	if last := abs32(s.samples[len(s.samples)-1]); last > 0.01 {
		t.Errorf("last sample %v: want it faded out", last)
	}
}

func TestEchoRepeats(t *testing.T) {
	s := Synth(1, Layer{Wave: Sine, Start: 440, Length: 50 * time.Millisecond, Decay: 5, Volume: 1})
	e := Echo(s, 100*time.Millisecond, 0.5, 2)
	if want := s.Frames() + int(0.2*SampleRate); e.Frames() != want {
		t.Errorf("echoed %d frames, want %d", e.Frames(), want)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
