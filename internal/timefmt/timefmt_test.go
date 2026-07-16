package timefmt

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		err  bool
	}{
		{"20m", 20 * time.Minute, false},
		{"2d", 48 * time.Hour, false},
		{"2d20m", 48*time.Hour + 20*time.Minute, false},
		{"1M", 30 * day, false},
		{"1h30m", 90 * time.Minute, false},
		{"1w", week, false},
		{"90s", 90 * time.Second, false},
		{"", 0, true},
		{"5", 0, true},   // number without unit
		{"m", 0, true},   // unit without number
		{"5x", 0, true},  // invalid unit
		{"5mm", 0, true}, // second m has no preceding number
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if c.err {
			if err == nil {
				t.Errorf("Parse(%q) expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFormatRoundTrip(t *testing.T) {
	for _, in := range []string{"20m", "2d20m", "1h30m", "1M"} {
		d, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		out := Format(d)
		d2, err := Parse(out)
		if err != nil {
			t.Fatalf("Parse(Format(%q))=Parse(%q): %v", in, out, err)
		}
		if d2 != d {
			t.Errorf("round trip %q -> %v -> %q -> %v", in, d, out, d2)
		}
	}
}
