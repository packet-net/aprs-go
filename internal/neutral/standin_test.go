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

func micE(destination string, extra map[string]any) map[string]any {
	d := map[string]any{"type": "mic-e", "mic_e_message": "returning", "latitude": 33.42733333333334, "longitude": -12.129, "symbol": "/j", "course_degrees": int64(251), "speed_knots": int64(20)}
	for k, v := range extra {
		d[k] = v
	}
	packet := map[string]any{"source": "N0CALL", "destination": destination, "data": d}
	return map[string]any{"type": "third-party", "packet": packet}
}

// Undecoded data within four levels is not-encodable, first in the order, whatever header fault
// an inner packet above it has (no destination, a source or a path nobody can hold).
func TestUndecodedInnerDataBeatsAnInnerHeaderFault(t *testing.T) {
	undecoded := map[string]any{"type": "unrecognized", "reason": "malformed"}
	nest := func(inner map[string]any, headers ...map[string]any) map[string]any {
		data := map[string]any{"type": "third-party", "packet": inner}
		for _, h := range headers {
			h["data"] = data
			data = map[string]any{"type": "third-party", "packet": h}
		}
		return data
	}
	bottom := map[string]any{"source": "A", "destination": "B", "data": undecoded}
	noDestination := map[string]any{"source": "AB"}
	badSource := map[string]any{"source": "A>B", "destination": "B"}
	for name, input := range map[string]map[string]any{
		"no destination two levels above": nest(bottom, map[string]any{"source": "A", "destination": "B"}, noDestination),
		"unusable source":                 nest(bottom, badSource),
	} {
		if code, _ := settled(t, input); code != "not-encodable" {
			t.Errorf("%s: got %q", name, code)
		}
	}
	// Five deep is invalid-third-party-header, and what lies below is never reached.
	five := nest(bottom, map[string]any{"source": "A", "destination": "B"}, map[string]any{"source": "A", "destination": "B"}, map[string]any{"source": "A", "destination": "B"}, map[string]any{"source": "A", "destination": "B"})
	if code, _ := settled(t, five); code != "invalid-third-party-header" {
		t.Errorf("five deep: got %q", code)
	}
}

// An inner Mic-E destination that does not match the data is the read-back's finding, so it comes
// after every rule check: an inner header fault beside it wins, and with nothing else it is
// not-encodable.
func TestInnerMicEDestinationMismatchComesAfterTheRuleChecks(t *testing.T) {
	if code, _ := settled(t, micE("S32U6T", nil)); code != "" {
		t.Errorf("its own destination: got %q", code)
	}
	if code, _ := settled(t, micE("APRS", nil)); code != "not-encodable" {
		t.Errorf("another destination: got %q", code)
	}
	withBadPath := micE("S32U6T", nil)
	withBadPath["packet"].(map[string]any)["destination"] = "APRS"
	withBadPath["packet"].(map[string]any)["path"] = []any{"TC+IP*"}
	if code, _ := settled(t, withBadPath); code != "invalid-third-party-header" {
		t.Errorf("a bad path beside a wrong destination: got %q", code)
	}
	// A data rule fault beside the wrong destination wins too.
	bad := micE("APRS", map[string]any{"speed_knots": int64(999999)})
	if code, _ := settled(t, bad); code != "value-out-of-range" {
		t.Errorf("a data fault beside a wrong destination: got %q", code)
	}
}
