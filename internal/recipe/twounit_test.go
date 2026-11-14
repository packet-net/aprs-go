package recipe

import (
	"math"
	"testing"
)

// A recipe giving a quantity in two units is refused, ranked among the checks on each value and
// value-given-twice, and never built from one unit.
func TestTwoUnitRecipeIsRanked(t *testing.T) {
	recipe := func(extra map[string]any) map[string]any {
		args := map[string]any{"latitude": 49.5, "longitude": -72.5, "symbol": "/>", "speed_knots": 10.0, "speed_kmh": 18.52}
		for k, v := range extra {
			args[k] = v
		}
		return map[string]any{"n": int64(1), "station": map[string]any{"source": "N0CALL"}, "report": "position", "args": args}
	}
	if r := Build(recipe(nil)); r.Refusal != "value-given-twice" {
		t.Errorf("two clean units: %v", r)
	}
	if r := Build(recipe(map[string]any{"speed_kmh": 1851259.2})); r.Refusal != "value-out-of-range" {
		t.Errorf("one unit out of range: %v", r)
	}
	if r := Build(recipe(map[string]any{"altitude_feet": 1.0, "altitude_m": 1.0e9})); r.Refusal != "value-out-of-range" {
		t.Errorf("a second pair: %v", r)
	}
}

func TestNonFiniteAmbiguityRanksAsNotANumber(t *testing.T) {
	args := map[string]any{"latitude": 49.5, "longitude": -72.5, "symbol": "/>", "ambiguity": math.NaN(), "altitude_feet": 1.0, "altitude_m": 1.0}
	r := Build(map[string]any{"n": int64(1), "station": map[string]any{"source": "N0CALL"}, "report": "position", "args": args})
	if r.Refusal != "not-a-number" {
		t.Errorf("%v", r)
	}
}

// A telemetry sequence or bulletin id the builder cannot take is a refusal made by the tool,
// ranked with the build's own.
func TestUntakeableInputsAreRefusalsNotUnsupported(t *testing.T) {
	station := map[string]any{"source": "N0CALL"}
	r := Build(map[string]any{"n": int64(1), "station": station, "report": "telemetry", "args": map[string]any{"sequence": int64(-1), "analog": []any{int64(1), int64(2), int64(3), int64(4), int64(5)}}})
	if r.Refusal != "value-out-of-range" || !r.Tool || r.Unsupported != "" {
		t.Errorf("sequence -1: %+v", r)
	}
	r = Build(map[string]any{"n": int64(1), "station": station, "report": "telemetry", "args": map[string]any{"sequence": math.Inf(1), "analog": []any{int64(1), int64(2), int64(3), int64(4), int64(5)}}})
	if r.Refusal != "not-a-number" || !r.Tool {
		t.Errorf("sequence Infinity: %+v", r)
	}
	r = Build(map[string]any{"n": int64(1), "station": station, "report": "bulletin", "args": map[string]any{"id": "AB", "text": "x"}})
	if r.Refusal != "invalid-addressee" || !r.Tool {
		t.Errorf("bulletin id AB: %+v", r)
	}
	// An earlier code in the build wins over the tool's: an invalid source is invalid-address.
	r = Build(map[string]any{"n": int64(1), "station": map[string]any{"source": "bad call"}, "report": "bulletin", "args": map[string]any{"id": "AB", "text": "x"}})
	if r.Refusal != "invalid-address" || r.Tool {
		t.Errorf("bad source and bulletin id: %+v", r)
	}
}
