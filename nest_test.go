package aprs_test

import (
	"bytes"
	"runtime"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// Third-party packets nested inside each other share the outer packet's information field, so a
// deeply nested line costs memory in proportion to its length, not its square.
func TestNestedThirdPartyMemoryIsLinear(t *testing.T) {
	const levels = 200000 // a 1 MB line
	line := append(bytes.Repeat([]byte("A>B:}"), levels), []byte("A>B:>hi")...)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	p, err := aprs.DecodeTNC2(line, aprs.Lenient)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("%d levels, %d bytes: %d MB allocated", levels, len(line), allocated>>20)
	if allocated > 128<<20 {
		t.Errorf("decoding a 1 MB nested line allocated %d MB", allocated>>20)
	}
	depth := 0
	for {
		tp, ok := p.Data.(aprs.ThirdParty)
		if !ok {
			break
		}
		p = tp.Packet
		depth++
	}
	eq(t, depth, levels)
	eq(t, p.Data.(aprs.Status).Text, "hi")
}
