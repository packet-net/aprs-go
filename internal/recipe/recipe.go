// Package recipe builds a builder recipe (the vectors' "build" input form, as tools/generate.py
// --recipes writes it) through this library's real builder API and says what came of it. The dump
// tool's build mode and the vectors test runner both use it, so the mapping from a recipe to the
// builder lives in one place.
package recipe

import (
	"math"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
)

// Outcome is what building a recipe gave: a packet, a refusal, or "unsupported" (the builder has no
// way to say something the recipe asks for).
type Outcome struct {
	Packet      *aprs.Packet
	Refusal     string // the refusal code id, when the builder refused
	Reason      string // the refusal's text
	Unsupported string // why the recipe could not be built at all
	Tool        bool   // the refusal is the dump tool's reader's, not the library's
}

// unitPairs are the quantities a recipe may give in two units. A recipe is one input, so one that
// gives both is value-given-twice, ranked after the checks on each value and the recipe's other
// faults (the vectors README, Builder rules).
var unitPairs = [][2]string{
	{"speed_knots", "speed_kmh"}, {"altitude_feet", "altitude_m"}, {"temperature_f", "temperature_c"},
	{"rain_1h_in", "rain_1h_mm"}, {"rain_24h_in", "rain_24h_mm"}, {"rain_midnight_in", "rain_midnight_mm"},
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
	bestRank := neutral.RefusalRank(best)
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
			if e, ok := err.(*aprs.EncodeError); ok && neutral.RefusalRank(e.Code().ID()) < bestRank {
				best, reason = e.Code().ID(), e.Error()
				bestRank = neutral.RefusalRank(best)
			} else if ia, ok := err.(*aprs.InvalidAddressError); ok && neutral.RefusalRank("invalid-address") < bestRank {
				best, reason = "invalid-address", ia.Error()
				bestRank = neutral.RefusalRank(best)
			}
		}
	}
	// value-given-twice is the tool's own refusal (the builder takes one unit at a time).
	return Outcome{Refusal: best, Reason: reason, Tool: best == "value-given-twice"}, true
}

// standIn is an input of a recipe the builder cannot take, whose value alone decides a refusal
// code (the vectors README, Encode records): it is replaced by a valid stand-in, and the recipe's
// result is the first code in the order among the build's and these.
type standIn struct{ code, reason string }

// Build builds a recipe (an object with station, report and args). An ambiguity, a telemetry
// sequence or a bulletin id that the builder cannot take is a refusal at its place in the order
// (not-a-number or value-out-of-range, or invalid-addressee): it is left out or given a valid
// stand-in for the build and ranked with whatever else the recipe refuses.
func Build(input map[string]any) Outcome {
	args, ok := input["args"].(map[string]any)
	if !ok {
		return buildFinite(input)
	}
	rest := make(map[string]any, len(args))
	for k, x := range args {
		rest[k] = x
	}
	var pending []standIn
	if v, ok := args["ambiguity"].(float64); ok && (math.IsNaN(v) || math.IsInf(v, 0)) {
		delete(rest, "ambiguity")
		pending = append(pending, standIn{"not-a-number", "ambiguity is not a number"})
	}
	switch input["report"] {
	case "telemetry":
		if c, why := sequenceFault(args["sequence"]); c != "" {
			rest["sequence"] = int64(0)
			pending = append(pending, standIn{c, why})
		}
	case "bulletin":
		if id, isText := args["id"].(string); isText {
			if _, one := oneChar(id); !one {
				rest["id"] = "0"
				pending = append(pending, standIn{"invalid-addressee", "a bulletin id is one character"})
			}
		}
	}
	if len(pending) == 0 {
		return buildFinite(input)
	}
	copied := make(map[string]any, len(input))
	for k, x := range input {
		copied[k] = x
	}
	copied["args"] = rest
	first := pending[0]
	for _, p := range pending[1:] {
		if neutral.RefusalRank(p.code) < neutral.RefusalRank(first.code) {
			first = p
		}
	}
	r := buildFinite(copied)
	if r.Refusal != "" && neutral.RefusalRank(r.Refusal) < neutral.RefusalRank(first.code) {
		return r
	}
	return Outcome{Refusal: first.code, Reason: first.reason, Tool: true}
}

// sequenceFault says what refuses a telemetry sequence the builder cannot take: not-a-number for
// NaN or an infinity, value-out-of-range for a whole number outside 0-65535. (A fraction is not
// judged here.)
func sequenceFault(v any) (string, string) {
	f, ok := asNumber(v)
	if !ok {
		return "", ""
	}
	switch {
	case math.IsNaN(f) || math.IsInf(f, 0):
		return "not-a-number", "sequence is not a number"
	case f == math.Trunc(f) && (f < 0 || f > 65535):
		return "value-out-of-range", "sequence is outside 0-65535"
	}
	return "", ""
}

func asNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
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
			return Outcome{Refusal: aprs.RefusalMissingField.ID(), Reason: e.reason, Tool: true}
		case *aprs.InvalidAddressError:
			return Outcome{Refusal: aprs.RefusalInvalidAddress.ID(), Reason: e.Error()}
		}
		panic(err)
	}
	return Outcome{Packet: p}
}
