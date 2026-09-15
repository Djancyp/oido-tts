// Package wav reads and writes canonical 16-bit PCM mono WAV files —
// just enough to clean up a recorded voice-cloning reference clip
// (trim silence, normalize level) without pulling in an audio library.
package wav

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// PCM holds decoded 16-bit mono samples.
type PCM struct {
	Samples    []int16
	SampleRate int
}

// Concat joins PCM clips in order, inserting a silence gap between them
// (for chunked synthesis: each chunk is a separate llama-tts run, so
// without a gap consecutive chunks can run together with no breath between
// sentences). gapsMs[i] is the timing before clips[i+1], so it must have
// len(clips)-1 entries — lets a caller give some boundaries a longer pause
// (an inline [pause:2s] tag) than others, or none at all. A negative entry
// instead overlaps that many ms of clips[i+1] back into the tail of the
// clip before it — two speakers talking over each other — by summing
// samples in the overlap window (clamped to int16 range) rather than
// concatenating; the overlap is capped to the shorter of the two clips so
// it can never run past either one. All clips must share the same sample
// rate. Returns nil for an empty input.
func Concat(clips []*PCM, gapsMs []int) *PCM {
	if len(clips) == 0 {
		return nil
	}
	sampleRate := clips[0].SampleRate

	capHint := 0
	for _, c := range clips {
		capHint += len(c.Samples)
	}

	out := make([]int16, 0, capHint)
	out = append(out, clips[0].Samples...)
	for i := 1; i < len(clips); i++ {
		next := clips[i].Samples
		gapSamples := sampleRate * gapsMs[i-1] / 1000

		if gapSamples >= 0 {
			out = append(out, make([]int16, gapSamples)...)
			out = append(out, next...)
			continue
		}

		overlap := min(-gapSamples, len(clips[i-1].Samples), len(next))
		base := len(out) - overlap
		for j := range overlap {
			out[base+j] = addClamped(out[base+j], next[j])
		}
		out = append(out, next[overlap:]...)
	}
	return &PCM{Samples: out, SampleRate: sampleRate}
}

// addClamped sums two samples, saturating at int16 range instead of
// wrapping — an overlap-mixed sum that would otherwise wrap around is a far
// worse artifact (a sharp crackle) than the mild clipping saturation gives.
func addClamped(a, b int16) int16 {
	sum := int32(a) + int32(b)
	return int16(min(max(sum, math.MinInt16), math.MaxInt16))
}

// Read parses a canonical WAV file (PCM, 16-bit, mono).
func Read(path string) (*PCM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a RIFF/WAVE file")
	}

	var sampleRate int
	var bitsPerSample int
	var channels int
	var pcmData []byte

	offset := 12
	for offset+8 <= len(data) {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		body := offset + 8
		if body+size > len(data) {
			size = len(data) - body // tolerate a truncated final chunk
		}

		switch id {
		case "fmt ":
			if size < 16 {
				return nil, fmt.Errorf("fmt chunk too small")
			}
			channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			bitsPerSample = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
		case "data":
			pcmData = data[body : body+size]
		}

		offset = body + size + size%2 // chunks are word-aligned
	}

	if pcmData == nil {
		return nil, fmt.Errorf("no data chunk found")
	}
	if channels != 1 || bitsPerSample != 16 {
		return nil, fmt.Errorf("only 16-bit mono WAV is supported (got %d channel(s), %d-bit)", channels, bitsPerSample)
	}

	samples := make([]int16, len(pcmData)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcmData[i*2 : i*2+2]))
	}
	return &PCM{Samples: samples, SampleRate: sampleRate}, nil
}

// Write encodes 16-bit mono PCM samples as a canonical WAV file.
func Write(path string, p *PCM) error {
	var buf bytes.Buffer
	dataSize := len(p.Samples) * 2

	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&buf, binary.LittleEndian, uint32(p.SampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(p.SampleRate*2)) // byte rate
	binary.Write(&buf, binary.LittleEndian, uint16(2))              // block align
	binary.Write(&buf, binary.LittleEndian, uint16(16))             // bits per sample

	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	for _, s := range p.Samples {
		binary.Write(&buf, binary.LittleEndian, s)
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// TrimSilence drops leading/trailing samples below thresholdDb, keeping
// a small pad on each side. Returns the input unchanged if it's silent
// throughout (nothing safe to trim to).
func TrimSilence(p *PCM, thresholdDb float64) *PCM {
	amp := int32(math.Pow(10, thresholdDb/20) * 32767)
	pad := p.SampleRate / 7 // ~150ms

	start := 0
	for start < len(p.Samples) && abs32(int32(p.Samples[start])) < amp {
		start++
	}
	end := len(p.Samples) - 1
	for end > start && abs32(int32(p.Samples[end])) < amp {
		end--
	}
	if start >= end {
		return p
	}

	from := max(0, start-pad)
	to := min(len(p.Samples), end+pad)
	return &PCM{Samples: p.Samples[from:to], SampleRate: p.SampleRate}
}

// Normalize scales peak amplitude to targetPeak (0..1, e.g. 0.9).
func Normalize(p *PCM, targetPeak float64) *PCM {
	var peak int32
	for _, s := range p.Samples {
		if a := abs32(int32(s)); a > peak {
			peak = a
		}
	}
	if peak == 0 {
		return p
	}
	scale := targetPeak * 32767 / float64(peak)

	out := make([]int16, len(p.Samples))
	for i, s := range p.Samples {
		v := float64(s) * scale
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		out[i] = int16(v)
	}
	return &PCM{Samples: out, SampleRate: p.SampleRate}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
