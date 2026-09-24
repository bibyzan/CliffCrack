package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// Blip synthesises a short pitched "blip": a sine sweeping from startHz to
// endHz with a fast attack and exponential decay. Handy for UI and gameplay
// feedback without shipping audio files.
func Blip(d time.Duration, startHz, endHz, volume float32) *Sound {
	n := int(d.Seconds() * SampleRate)
	mono := make([]float32, n)
	phase := 0.0
	attack := float64(SampleRate) * 0.004
	for i := range mono {
		t := float64(i) / float64(n)
		freq := float64(startHz) + (float64(endHz)-float64(startHz))*t
		phase += 2 * math.Pi * freq / SampleRate
		env := math.Exp(-5*t) * min(1, float64(i)/attack)
		mono[i] = volume * float32(math.Sin(phase)*env)
	}
	return FromMono(mono)
}

// LoadWAV decodes a RIFF/WAVE file: PCM 8/16/24/32-bit or IEEE float 32-bit,
// mono or stereo, any sample rate (linearly resampled to SampleRate).
func LoadWAV(r io.Reader) (*Sound, error) {
	var riff [12]byte
	if _, err := io.ReadFull(r, riff[:]); err != nil {
		return nil, fmt.Errorf("wav: %w", err)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return nil, errors.New("wav: not a RIFF/WAVE file")
	}

	var (
		format, channels, bits uint16
		rate                   uint32
		haveFmt                bool
	)
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, errors.New("wav: no data chunk")
		}
		id, size := string(hdr[:4]), binary.LittleEndian.Uint32(hdr[4:])
		switch id {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil || size < 16 {
				return nil, errors.New("wav: bad fmt chunk")
			}
			format = binary.LittleEndian.Uint16(buf[0:])
			channels = binary.LittleEndian.Uint16(buf[2:])
			rate = binary.LittleEndian.Uint32(buf[4:])
			bits = binary.LittleEndian.Uint16(buf[14:])
			if format == 0xFFFE && size >= 26 { // WAVE_FORMAT_EXTENSIBLE: real format in the sub-GUID
				format = binary.LittleEndian.Uint16(buf[24:])
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return nil, errors.New("wav: data before fmt")
			}
			data := make([]byte, size)
			if _, err := io.ReadFull(r, data); err != nil {
				return nil, fmt.Errorf("wav: short data: %w", err)
			}
			return decodePCM(data, format, channels, bits, rate)
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size+size&1)); err != nil { // chunks are word-aligned
				return nil, fmt.Errorf("wav: %w", err)
			}
		}
		if size&1 == 1 && (id == "fmt ") {
			if _, err := io.CopyN(io.Discard, r, 1); err != nil {
				return nil, err
			}
		}
	}
}

func decodePCM(data []byte, format, channels, bits uint16, rate uint32) (*Sound, error) {
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("wav: %d channels not supported", channels)
	}
	if rate == 0 {
		return nil, errors.New("wav: zero sample rate")
	}
	bytesPer := int(bits / 8)
	var sample func(b []byte) float32
	switch {
	case format == 1 && bits == 8:
		sample = func(b []byte) float32 { return (float32(b[0]) - 128) / 128 }
	case format == 1 && bits == 16:
		sample = func(b []byte) float32 { return float32(int16(binary.LittleEndian.Uint16(b))) / 32768 }
	case format == 1 && bits == 24:
		sample = func(b []byte) float32 {
			v := int32(uint32(b[0])<<8|uint32(b[1])<<16|uint32(b[2])<<24) >> 8
			return float32(v) / 8388608
		}
	case format == 1 && bits == 32:
		sample = func(b []byte) float32 { return float32(int32(binary.LittleEndian.Uint32(b))) / 2147483648 }
	case format == 3 && bits == 32:
		sample = func(b []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b)) }
	default:
		return nil, fmt.Errorf("wav: format %d with %d bits not supported", format, bits)
	}

	frameBytes := bytesPer * int(channels)
	frames := len(data) / frameBytes
	stereo := make([]float32, 2*frames)
	for f := 0; f < frames; f++ {
		b := data[f*frameBytes:]
		l := sample(b)
		r := l
		if channels == 2 {
			r = sample(b[bytesPer:])
		}
		stereo[2*f], stereo[2*f+1] = l, r
	}
	return &Sound{samples: resample(stereo, int(rate))}, nil
}

// resample converts interleaved stereo from rate to SampleRate (linear interpolation).
func resample(in []float32, rate int) []float32 {
	if rate == SampleRate || len(in) < 4 {
		return in
	}
	inFrames := len(in) / 2
	outFrames := int(int64(inFrames) * SampleRate / int64(rate))
	out := make([]float32, 2*outFrames)
	step := float64(rate) / SampleRate
	for f := 0; f < outFrames; f++ {
		pos := float64(f) * step
		i := int(pos)
		frac := float32(pos - float64(i))
		j := min(i+1, inFrames-1)
		for c := 0; c < 2; c++ {
			a, b := in[2*i+c], in[2*j+c]
			out[2*f+c] = a + (b-a)*frac
		}
	}
	return out
}
