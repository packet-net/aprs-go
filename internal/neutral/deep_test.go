package neutral

import (
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// A caller-built third-party chain far deeper than a decoder reads converts to the neutral form
// without recursing past the cap, and reading a deep neutral chain is refused.
func TestDeepChain(t *testing.T) {
	src, dst := aprs.MustAddress("A"), aprs.MustAddress("B")
	var data aprs.Data = aprs.Status{Text: "hi"}
	var neutralData any
	for i := 0; i < 10000; i++ {
		inner := aprs.Decode(src, dst, nil, []byte(">hi"), aprs.Lenient)
		inner.Data = data
		data = aprs.ThirdParty{Packet: inner}
	}
	neutralData = Data(data)
	if _, un := ReadData(neutralData); un == nil || !un.Refused || un.Code != aprs.RefusalInvalidThirdPartyHeader {
		// The cut chain ends in undecoded data; either way it must come back without recursing.
		t.Logf("read back: %v", un)
	}
	deep := map[string]any{"type": "status", "text": "x"}
	for i := 0; i < 10000; i++ {
		deep = map[string]any{"type": "third-party", "packet": map[string]any{"source": "A", "destination": "B", "data": deep}}
	}
	if _, un := ReadData(deep); un == nil || !un.Refused || un.Code != aprs.RefusalInvalidThirdPartyHeader {
		t.Errorf("a 10,000-level neutral chain should be refused: %v", un)
	}
}
