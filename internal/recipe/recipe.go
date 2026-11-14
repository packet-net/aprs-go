// Package recipe builds a builder recipe (the vectors' "build" input form, as tools/generate.py
// --recipes writes it) through this library's real builder API and says what came of it. The dump
// tool's build mode and the vectors test runner both use it, so the mapping from a recipe to the
// builder lives in one place.
package recipe

import (
	"math"

	aprs "github.com/packet-net/aprs-go"
)

// Outcome is what building a recipe gave: a packet, a refusal, or "unsupported" (the builder has no
// way to say something the recipe asks for).
type Outcome struct {
	Packet      *aprs.Packet
	Refusal     string // the refusal code id, when the builder refused
	Reason      string // the refusal's text
	Unsupported string // why the recipe could not be built at all
}

// unitPairs are the quantities a recipe may give in two units. A recipe is one input, so one that
// gives both is value-given-twice, ranked after the checks on each value and the recipe's other
// faults (the vectors README, Builder rules).
var unitPairs = [][2]string{
	{"speed_knots", "speed_kmh"}, {"altitude_feet", "altitude_m"}, {"temperature_f", "temperature_c"},
	{"rain_1h_in", "rain_1h_mm"}, {"rain_24h_in", "rain_24h_mm"}, {"rain_midnight_in", "rain_midnight_mm"},
}

// refusalOrder is the README's Refusal reasons order.
var refusalOrder = []string{
	"not-encodable", "invalid-address", "ax25-cannot-carry", "tnc2-cannot-carry", "invalid-third-party-header",
	"invalid-addressee", "invalid-name", "invalid-timestamp", "not-a-number", "value-out-of-range", "value-has-no-form",
	"invalid-code", "invalid-symbol", "invalid-character", "too-long", "padding-space", "malformed", "invalid-message-id",
	"invalid-nmea", "extension-conflict", "field-not-allowed", "symbol-mismatch", "missing-field", "value-given-twice",
	"text-reads-as-field", "does-not-read-back",
}

func refusalRank(id string) int {
	for i, c := range refusalOrder {
		if c == id {
			return i
		}
	}
	return len(refusalOrder)
}

// twoUnit builds a recipe that gives a quantity in two units once with each unit (every value is
// checked on its own) and reports the first code in the order among those refusals and
// value-given-twice. It never builds from one unit and never gives unsupported.
func twoUnit(input map[string]any, args map[string]any) (Outcome, bool) {
	var present [][2]string
	for _, pair := range unitPairs {
		if _, a := args[pair[0]]; a {
			if _, b := args[pair[1]]; b {
				present = append(present, pair)
			}
		}
	}
	if len(present) == 0 {
		return Outcome{}, false
	}
	best, reason := "value-given-twice", "a quantity given in two units"
	for side := 0; side < 2; side++ {
		a := make(map[string]any, len(args))
		for k, v := range args {
			a[k] = v
		}
		for _, pair := range present {
			delete(a, pair[1-side])
		}
		copied := make(map[string]any, len(input))
		for k, v := range input {
			copied[k] = v
		}
		copied["args"] = a
		if _, err := build(copied); err != nil {
			if e, ok := err.(*aprs.EncodeError); ok && refusalRank(e.Code().ID()) < refusalRank(best) {
				best, reason = e.Code().ID(), e.Error()
			} else if ia, ok := err.(*aprs.InvalidAddressError); ok && refusalRank("invalid-address") < refusalRank(best) {
				best, reason = "invalid-address", ia.Error()
			}
		}
	}
	return Outcome{Refusal: best, Reason: reason}, true
}

// Build builds a recipe (an object with station, report and args). An ambiguity that is not a
// finite number is not-a-number at its place in the order: it is left out of the build and ranked
// with whatever else the recipe refuses.
func Build(input map[string]any) Outcome {
	args, ok := input["args"].(map[string]any)
	if !ok {
		return buildFinite(input)
	}
	v, ok := args["ambiguity"].(float64)
	if !ok || (!math.IsNaN(v) && !math.IsInf(v, 0)) {
		return buildFinite(input)
	}
	rest := make(map[string]any, len(args))
	for k, x := range args {
		if k != "ambiguity" {
			rest[k] = x
		}
	}
	copied := make(map[string]any, len(input))
	for k, x := range input {
		copied[k] = x
	}
	copied["args"] = rest
	r := buildFinite(copied)
	if r.Refusal != "" && refusalRank(r.Refusal) < refusalRank("not-a-number") {
		return r
	}
	return Outcome{Refusal: "not-a-number", Reason: "ambiguity is not a number"}
}

func buildFinite(input map[string]any) Outcome {
	if args, ok := input["args"].(map[string]any); ok {
		if r, two := twoUnit(input, args); two {
			return r
		}
	}
	p, err := build(input)
	if err != nil {
		switch e := err.(type) {
		case *notSupported:
			return Outcome{Unsupported: e.reason}
		case *aprs.EncodeError:
			return Outcome{Refusal: e.Code().ID(), Reason: e.Error()}
		case *missingField:
			return Outcome{Refusal: aprs.RefusalMissingField.ID(), Reason: e.reason}
		case *aprs.InvalidAddressError:
			return Outcome{Refusal: aprs.RefusalInvalidAddress.ID(), Reason: e.Error()}
		}
		panic(err)
	}
	return Outcome{Packet: p}
}
