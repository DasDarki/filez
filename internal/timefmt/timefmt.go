// Package timefmt parses the compact duration format used by Filez, e.g.
// "20m", "2d", "2d20m", "1M". Units are case-sensitive because minutes (m)
// and months (M) share a letter. Supported units:
//
//	s = seconds
//	m = minutes
//	h = hours
//	d = days
//	w = weeks
//	M = months (fixed at 30 days)
//
// The format is shared by the server (DEFAULT_UPLOAD env value) and the CLI
// client (--temp flag), so it lives in a neutral package imported by both.
package timefmt

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	day   = 24 * time.Hour
	week  = 7 * day
	month = 30 * day
)

// ErrEmpty is returned when the input contains no duration at all.
var ErrEmpty = errors.New("timefmt: empty duration")

var units = map[byte]time.Duration{
	's': time.Second,
	'm': time.Minute,
	'h': time.Hour,
	'd': day,
	'w': week,
	'M': month,
}

// Parse turns a compact duration string into a time.Duration.
// Multiple unit segments may be combined, e.g. "2d20m" -> 48h20m.
func Parse(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrEmpty
	}

	var total time.Duration
	var num int64
	haveNum := false
	sawSegment := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			num = num*10 + int64(c-'0')
			haveNum = true
		default:
			unit, ok := units[c]
			if !ok {
				return 0, fmt.Errorf("timefmt: invalid unit %q in %q", string(c), s)
			}
			if !haveNum {
				return 0, fmt.Errorf("timefmt: unit %q without a number in %q", string(c), s)
			}
			total += time.Duration(num) * unit
			num = 0
			haveNum = false
			sawSegment = true
		}
	}

	if haveNum {
		return 0, fmt.Errorf("timefmt: trailing number without a unit in %q", s)
	}
	if !sawSegment {
		return 0, ErrEmpty
	}
	return total, nil
}

// Format renders a duration back into the compact format using the largest
// units that divide evenly. Primarily used for human-readable display.
func Format(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	var b strings.Builder
	for _, u := range []struct {
		suffix byte
		size   time.Duration
	}{
		{'M', month}, {'w', week}, {'d', day}, {'h', time.Hour}, {'m', time.Minute}, {'s', time.Second},
	} {
		if d >= u.size {
			n := d / u.size
			d -= n * u.size
			fmt.Fprintf(&b, "%d%c", n, u.suffix)
		}
	}
	if b.Len() == 0 {
		return "0m"
	}
	return b.String()
}
