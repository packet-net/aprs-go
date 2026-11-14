package neutral

import (
	"math"
	"testing"
)

func TestParseNonFiniteTokens(t *testing.T) {
	v, err := Parse([]byte(`{"a":NaN,"b":Infinity,"c":-Infinity,"d":"NaN Infinity","e":[1,NaN]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if !math.IsNaN(m["a"].(float64)) || !math.IsInf(m["b"].(float64), 1) || !math.IsInf(m["c"].(float64), -1) {
		t.Errorf("non-finite tokens read as %v", m)
	}
	if m["d"] != "NaN Infinity" {
		t.Errorf("a string was changed: %v", m["d"])
	}
	if !math.IsNaN(m["e"].([]any)[1].(float64)) {
		t.Errorf("a list element was not read: %v", m["e"])
	}
}
