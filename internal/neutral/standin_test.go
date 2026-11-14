package neutral

import (
	"math"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

func settled(t *testing.T, input map[string]any) (string, bool) {
	t.Helper()
	data, pending, un := ReadDataStandIn(input)
	if un != nil && !un.Refused {
		t.Fatalf("unsupported: %v", un)
	}
	if un != nil {
		return un.Code.ID(), un.Tool
	}
	var code *aprs.RefusalCode
	why := ""
	if _, err := aprs.Encode(data); err != nil {
		c := err.(*aprs.EncodeError).Code()
		code, why = &c, err.Error()
	}
	c, _, tool := Settle(pending, code, why)
	if pending == nil && code == nil {
		return "", false
	}
	return c.ID(), tool
}

func TestInputsThePackageCannotHoldAreRefusals(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		code  string
		tool  bool
	}{
		{"symbol of one character", map[string]any{"type": "position", "latitude": 49.5, "longitude": -72.5, "symbol": "/"}, "invalid-symbol", true},
		{"agrelo bearing rounds up at a half", map[string]any{"type": "agrelo-df", "bearing_degrees": 12.5, "quality": int64(5)}, "", false},
		{"agrelo bearing rounds to 361", map[string]any{"type": "agrelo-df", "bearing_degrees": 360.5, "quality": int64(5)}, "value-out-of-range", false},
		{"agrelo bearing negative", map[string]any{"type": "agrelo-df", "bearing_degrees": -1.0, "quality": int64(5)}, "value-out-of-range", true},
		{"agrelo quality 10", map[string]any{"type": "agrelo-df", "bearing_degrees": int64(5), "quality": int64(10)}, "value-out-of-range", true},
		{"agrelo bearing NaN and quality 10", map[string]any{"type": "agrelo-df", "bearing_degrees": math.NaN(), "quality": int64(10)}, "not-a-number", true},
	}
	for _, c := range cases {
		code, tool := settled(t, c.input)
		if code != c.code || (c.code != "" && tool != c.tool) {
			t.Errorf("%s: got %q (tool %v), want %q (tool %v)", c.name, code, tool, c.code, c.tool)
		}
	}
}

func TestAnEarlierEncoderCodeBeatsAPendingOne(t *testing.T) {
	// A message to a bad addressee (invalid-addressee) with a symbol-less status locator is not
	// needed here: a bad symbol on a position with a latitude out of range is value-out-of-range,
	// which comes before invalid-symbol.
	code, tool := settled(t, map[string]any{"type": "position", "latitude": 99.0, "longitude": -72.5, "symbol": "/"})
	if code != "value-out-of-range" || tool {
		t.Errorf("got %q (tool %v)", code, tool)
	}
}
