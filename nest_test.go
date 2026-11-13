package aprs_test

import (
	"bytes"
	"encoding/hex"
	"runtime"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// Third-party packets nested inside each other share the outer packet's information field, so a
// deeply nested line costs memory in proportion to its length, not its square, and at most four
// levels are read.
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
	// Four levels are read; the fifth } is not (the packet whose information field it starts is
	// unrecognized).
	eq(t, depth, 4)
	if _, ok := p.Data.(aprs.Unrecognized); !ok {
		t.Errorf("the fifth level is %T, expected Unrecognized", p.Data)
	}
}

// An AX.25 digipeater of spaces is an empty path entry: tolerated, and dropped from the path with
// its used bit, so it neither appears nor marks the entry after it.
func TestAX25AllSpaceDigipeaterIsDropped(t *testing.T) {
	frame, _ := hex.DecodeString("82A0B4606062609C60868298986040404040404" + "0E0" + "AE92888A62406103F03E78")
	p, err := aprs.DecodeAX25(frame, aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, len(p.Path), 1)
	eq(t, p.Path[0].Address.String(), "WIDE1")
	eq(t, p.Path[0].Used, false)
	found := false
	for _, d := range p.Diagnostics {
		found = found || d.Code == aprs.CodeEmptyPathEntry
	}
	eq(t, found, true)
	_, err = aprs.DecodeAX25(frame, aprs.Strict)
	if err == nil {
		t.Error("strict should reject the empty path entry")
	}
}

// A chain of third-party data built by a caller, far deeper than a decoder reads, is refused
// without recursion, and converting or writing it does not blow up.
func TestDeepThirdPartyChainIsRefused(t *testing.T) {
	src, dst := aprs.MustAddress("A"), aprs.MustAddress("B")
	var data aprs.Data = aprs.Status{Text: "hi"}
	for i := 0; i < 10000; i++ {
		inner := aprs.Decode(src, dst, nil, []byte(">hi"), aprs.Lenient)
		inner.Data = data
		data = aprs.ThirdParty{Packet: inner}
	}
	_, err := aprs.Encode(data)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	eq(t, err.(*aprs.EncodeError).Code(), aprs.RefusalInvalidThirdPartyHeader)
	_, err = aprs.NewPacket(src, dst, nil, data)
	if err == nil {
		t.Fatal("expected a refusal from NewPacket")
	}
}
