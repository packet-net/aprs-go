package main

import "testing"

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
	if r := buildRecord(recipe(nil)); r["refusal"] != "value-given-twice" {
		t.Errorf("two clean units: %v", r)
	}
	if r := buildRecord(recipe(map[string]any{"speed_kmh": 1851259.2})); r["refusal"] != "value-out-of-range" {
		t.Errorf("one unit out of range: %v", r)
	}
	if r := buildRecord(recipe(map[string]any{"altitude_feet": 1.0, "altitude_m": 1.0e9})); r["result"] != "refused" || r["refusal"] != "value-out-of-range" {
		t.Errorf("a second pair: %v", r)
	}
}
