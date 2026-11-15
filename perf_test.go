package aprs_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	aprs "github.com/packet-net/aprs-go"
)

// Hostile input: a long comment made of one candidate element over and over must decode in time
// that grows with its size, never with its square. The test compares the time at two sizes (a
// ratio), not a wall-clock limit: four times the bytes may take about four times as long, and a
// quadratic search takes sixteen.
var hostileElements = []string{"{", "PHG1", "|!!|", "/A=000001", "!W12!", "\"4e}", "RNG1", "DFS1", "}", "/A=", "|", "!"}

var hostileReports = []struct{ name, prefix string }{
	{"position", "N0CALL>APRS:!4903.50N/07201.75W-"},
	{"signpost", "N0CALL>APRS:!4903.50N\\07201.75Wm"},
	{"weather", "N0CALL>APRS:!4903.50N/07201.75W_"},
	{"compressed", "N0CALL>APRS:!/5L!!<*e7>7P["},
	{"mic-e", "N0CALL>S32UVT:`(_fn\"Oj/`"},
	{"mic-e-signpost", "N0CALL>S32UVT:`(_fn\"Om\\`"},
	{"mic-e-locator", "N0CALL>S32UVT:`(_fn\"Oj/`IO91/G "},
}

func decodeHostile(prefix, element string, size int) time.Duration {
	line := append([]byte(prefix), bytes.Repeat([]byte(element), size/len(element)+1)[:size]...)
	best := time.Duration(1 << 62)
	for i := 0; i < 3; i++ {
		start := time.Now()
		_, _ = aprs.DecodeTNC2(line, aprs.Lenient)
		if d := time.Since(start); d < best {
			best = d
		}
	}
	return best
}

func TestLongRunsOfOneElementDecodeInLinearTime(t *testing.T) {
	report := os.Getenv("APRS_PERF_REPORT") != ""
	only := os.Getenv("APRS_PERF_ONLY")
	for _, r := range hostileReports {
		if only != "" && r.name != only {
			continue
		}
		for _, element := range hostileElements {
			small := decodeHostile(r.prefix, element, 50_000)
			large := decodeHostile(r.prefix, element, 200_000)
			if report {
				fmt.Printf("%-14s %-10q 50KB %10v 200KB %10v\n", r.name, element, small, large)
			}
			// Linear growth is four times; a floor keeps a sub-millisecond reading from
			// deciding it. Quadratic growth is sixteen times.
			floor := 3 * time.Millisecond
			if small < floor {
				small = floor
			}
			if large > 8*small {
				t.Errorf("%s with %q repeated: %v at 50 KB but %v at 200 KB (more than 8x for 4x the size)", r.name, element, small, large)
			}
		}
	}
}
