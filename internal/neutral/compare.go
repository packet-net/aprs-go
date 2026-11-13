package neutral

import (
	"encoding/json"
	"fmt"
	"math"
)

// Differences lists the differences between an expected and a produced value, by the vectors'
// rules: exact key sets, strings, booleans and integers exactly, other numbers within 1e-9.
func Differences(expected, actual any, at string, out *[]string) {
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: expected %s, got %s", at, Show(expected), Show(actual)))
			return
		}
		for k, ev := range e {
			av, ok := a[k]
			if !ok {
				*out = append(*out, fmt.Sprintf("%s.%s: expected %s, missing", at, k, Show(ev)))
				continue
			}
			Differences(ev, av, at+"."+k, out)
		}
		for k, av := range a {
			if _, ok := e[k]; !ok {
				*out = append(*out, fmt.Sprintf("%s.%s: unexpected %s", at, k, Show(av)))
			}
		}
	case []any:
		a, ok := actual.([]any)
		if !ok || len(a) != len(e) {
			*out = append(*out, fmt.Sprintf("%s: expected %s, got %s", at, Show(expected), Show(actual)))
			return
		}
		for i := range e {
			Differences(e[i], a[i], fmt.Sprintf("%s[%d]", at, i), out)
		}
	case int64, float64:
		x, xok := asFloat(expected)
		y, yok := asFloat(actual)
		if !yok {
			*out = append(*out, fmt.Sprintf("%s: expected %s, got %s", at, Show(expected), Show(actual)))
			return
		}
		ei, eInt := expected.(int64)
		ai, aInt := actual.(int64)
		same := false
		if eInt && aInt {
			same = ei == ai
		} else if xok {
			scale := math.Max(math.Abs(x), math.Abs(y))
			if scale < 1 {
				scale = 1
			}
			same = math.Abs(x-y) <= 1e-9*scale
		}
		if !same {
			*out = append(*out, fmt.Sprintf("%s: expected %s, got %s", at, Show(expected), Show(actual)))
		}
	default:
		if expected != actual {
			*out = append(*out, fmt.Sprintf("%s: expected %s, got %s", at, Show(expected), Show(actual)))
		}
	}
}

// Show gives a neutral value as JSON, for messages.
func Show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
