package audio

import (
	"math"
	"math/rand/v2"
	"time"
)

// Wave is a layer's source: a tone of some shape, or noise.
type Wave int

const (
	Sine Wave = iota
	Triangle
	Square
	Saw
	Noise
)

// Layer is one part of a synthesised sound: a tone sweeping in pitch, or
// filtered noise, shaped by an envelope and starting after Delay. Layers
// are summed, so a gunshot is a noise crack over a low thump over a click.
type Layer struct {
	Wave       Wave
	Start, End float32 // Hz, swept exponentially (for noise, ignored)
	Delay      time.Duration
	Length     time.Duration
	Attack     time.Duration
	Decay      float32 // 1/s: the envelope falls as exp(-Decay t) after the attack
	Volume     float32
	LowPass    float32 // Hz, 0 for none: rounds off the top (a thump, a rumble)
	HighPass   float32 // Hz, 0 for none: thins out the bottom (a crack, a hiss)
	Tremolo    float32 // Hz of wobble in volume, 0 for none
}

// Synth mixes layers into a sound, softly saturated and scaled so its
// loudest moment is at volume. Noise is seeded, so a recipe always sounds
// the same.
func Synth(volume float32, layers ...Layer) *Sound { return SynthTake(0, volume, layers...) }

// SynthTake is Synth with its noise seeded by take: another take of the same
// recipe, alike but not identical (for Variants).
func SynthTake(take int, volume float32, layers ...Layer) *Sound {
	var n int
	for _, l := range layers {
		n = max(n, int((l.Delay+l.Length).Seconds()*SampleRate))
	}
	mono := make([]float32, n)
	rng := rand.New(rand.NewPCG(uint64(n), 0x5eed+uint64(take)))
	for _, l := range layers {
		addLayer(mono, l, rng)
	}
	peak := float32(0)
	for i, v := range mono {
		v = float32(math.Tanh(float64(v))) // soft saturation: layers can stack without clipping
		mono[i] = v
		peak = max(peak, abs32(v))
	}
	if peak > 0 {
		for i := range mono {
			mono[i] *= volume / peak
		}
	}
	return FromMono(mono)
}

func addLayer(out []float32, l Layer, rng *rand.Rand) {
	start := int(l.Delay.Seconds() * SampleRate)
	n := int(l.Length.Seconds() * SampleRate)
	attack := max(l.Attack.Seconds()*SampleRate, SampleRate*0.001)
	release := SampleRate * 0.006 // fade the last few ms so nothing clicks off
	lowA := onePole(l.LowPass)
	highA := onePole(l.HighPass)
	var phase, low, highLow float64
	for i := 0; i < n && start+i < len(out); i++ {
		t := float64(i) / SampleRate
		frac := float64(i) / float64(max(n-1, 1))
		var v float64
		if l.Wave == Noise {
			v = rng.Float64()*2 - 1
		} else {
			f0, f1 := float64(l.Start), float64(max(l.End, 1))
			freq := f0
			if f0 > 0 && f1 != f0 {
				freq = f0 * math.Pow(f1/f0, frac)
			}
			phase += freq / SampleRate
			phase -= math.Floor(phase)
			v = wave(l.Wave, phase)
		}
		if lowA > 0 {
			low += lowA * (v - low)
			v = low
		}
		if highA > 0 {
			highLow += highA * (v - highLow)
			v -= highLow
		}
		env := math.Min(1, float64(i)/attack)
		if i > int(attack) {
			env = math.Exp(-float64(l.Decay) * (t - attack/SampleRate))
		}
		env *= math.Min(1, float64(n-i)/release)
		if l.Tremolo > 0 {
			env *= 0.6 + 0.4*math.Sin(2*math.Pi*float64(l.Tremolo)*t)
		}
		out[start+i] += float32(v * env * float64(l.Volume))
	}
}

// onePole is the coefficient of a one-pole filter at cutoff Hz (0: off).
func onePole(cutoff float32) float64 {
	if cutoff <= 0 {
		return 0
	}
	return 1 - math.Exp(-2*math.Pi*float64(cutoff)/SampleRate)
}

func wave(w Wave, phase float64) float64 {
	switch w {
	case Triangle:
		return 4*math.Abs(phase-0.5) - 1
	case Square:
		if phase < 0.5 {
			return 0.7
		}
		return -0.7
	case Saw:
		return 2*phase - 1
	}
	return math.Sin(2 * math.Pi * phase)
}

// Echo adds repeats of a sound, each delay later and feedback quieter: the
// slap back off a canyon wall.
func Echo(s *Sound, delay time.Duration, feedback float32, repeats int) *Sound {
	step := int(delay.Seconds()*SampleRate) * 2 // stereo frames
	out := make([]float32, len(s.samples)+step*repeats)
	copy(out, s.samples)
	gain := float32(1)
	for r := 1; r <= repeats; r++ {
		gain *= feedback
		for i, v := range s.samples {
			out[i+step*r] += v * gain
		}
	}
	return NewSound(out)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
