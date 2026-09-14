package wav

import (
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	samples := make([]int16, 1000)
	for i := range samples {
		samples[i] = int16(i % 100)
	}
	in := &PCM{Samples: samples, SampleRate: 24000}

	path := filepath.Join(t.TempDir(), "test.wav")
	if err := Write(path, in); err != nil {
		t.Fatal(err)
	}

	out, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.SampleRate != in.SampleRate {
		t.Errorf("sample rate = %d, want %d", out.SampleRate, in.SampleRate)
	}
	if len(out.Samples) != len(in.Samples) {
		t.Fatalf("len(samples) = %d, want %d", len(out.Samples), len(in.Samples))
	}
	for i := range in.Samples {
		if out.Samples[i] != in.Samples[i] {
			t.Fatalf("sample %d = %d, want %d", i, out.Samples[i], in.Samples[i])
		}
	}
}

func TestTrimSilence(t *testing.T) {
	sr := 24000
	silence := make([]int16, sr/2) // 0.5s
	loud := make([]int16, sr)      // 1s at max volume
	for i := range loud {
		loud[i] = 32767
	}
	samples := append(append(append([]int16{}, silence...), loud...), silence...)
	p := &PCM{Samples: samples, SampleRate: sr}

	trimmed := TrimSilence(p, -40)
	if len(trimmed.Samples) >= len(p.Samples) {
		t.Fatalf("expected trimming to shrink the sample count, got %d >= %d", len(trimmed.Samples), len(p.Samples))
	}
	if len(trimmed.Samples) < sr {
		t.Fatalf("trimmed too aggressively: %d samples, want at least the %d loud ones", len(trimmed.Samples), sr)
	}
}

func TestNormalize(t *testing.T) {
	p := &PCM{Samples: []int16{100, -200, 300, -50}, SampleRate: 24000}
	out := Normalize(p, 0.9)

	var peak int32
	for _, s := range out.Samples {
		if a := abs32(int32(s)); a > peak {
			peak = a
		}
	}
	targetPeak := 0.9
	want := int32(targetPeak * 32767)
	if diff := peak - want; diff < -1 || diff > 1 {
		t.Errorf("peak after normalize = %d, want ~%d", peak, want)
	}
}

func TestNormalizeSilentInputUnchanged(t *testing.T) {
	p := &PCM{Samples: []int16{0, 0, 0}, SampleRate: 24000}
	out := Normalize(p, 0.9)
	for _, s := range out.Samples {
		if s != 0 {
			t.Fatalf("expected silence to stay silent, got %v", out.Samples)
		}
	}
}

func TestConcat(t *testing.T) {
	a := &PCM{Samples: []int16{1, 2, 3}, SampleRate: 24000}
	b := &PCM{Samples: []int16{4, 5}, SampleRate: 24000}

	out := Concat([]*PCM{a, b}, 100)

	wantGap := 24000 * 100 / 1000
	wantLen := len(a.Samples) + wantGap + len(b.Samples)
	if len(out.Samples) != wantLen {
		t.Fatalf("len = %d, want %d", len(out.Samples), wantLen)
	}
	for i, s := range a.Samples {
		if out.Samples[i] != s {
			t.Errorf("sample %d = %d, want %d (from first clip)", i, out.Samples[i], s)
		}
	}
	for i, s := range b.Samples {
		got := out.Samples[len(a.Samples)+wantGap+i]
		if got != s {
			t.Errorf("sample after gap %d = %d, want %d (from second clip)", i, got, s)
		}
	}
}

func TestConcatEmpty(t *testing.T) {
	if got := Concat(nil, 100); got != nil {
		t.Fatalf("got %v, want nil for empty input", got)
	}
}
