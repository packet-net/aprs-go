package aprs

import (
	"math"
	"strconv"
)

// halfTolerance is how close to a step's half, measured in steps, a value must be to count as the
// half. A neutral value is decimal text whose nearest binary number often lies a few ulps either
// side of the half the text means; this lets the half the text means win.
const halfTolerance = 1e-6

// roundSteps rounds a value measured in its field's steps to the nearest whole step, halves away
// from zero, where a value within 1e-6 of a half is the half (vectors README, Encoding).
func roundSteps(steps float64) float64 {
	if !isFinite(steps) {
		return steps
	}
	magnitude := math.Abs(steps)
	floor := math.Floor(magnitude)
	fraction := magnitude - floor
	rounded := floor
	if fraction > 0.5 || math.Abs(fraction-0.5) <= halfTolerance {
		rounded = floor + 1
	}
	// A value that rounds to 0 is 0, not -0.
	if steps < 0 && rounded != 0 {
		return -rounded
	}
	return rounded
}

// sameNumber says whether two numbers are the same value, as the vectors compare them: within
// 1e-9, relative for values of 1 or more.
func sameNumber(x, y float64) bool {
	return x == y || math.Abs(x-y) <= 1e-9*fmax(fmax(math.Abs(x), math.Abs(y)), 1)
}

// numberText gives a number as the shortest decimal text that reads back as it, with no
// exponent, no +, a 0 before a leading point and no trailing .0: 5.2, -32, 0.53, and 1e61 as a 1
// and 61 zeros.
func numberText(v float64) string {
	if v == 0 {
		return "0"
	}
	if math.IsInf(v, 1) {
		return "inf"
	}
	if math.IsInf(v, -1) {
		return "-inf"
	}
	if math.IsNaN(v) {
		return "NaN"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func isFinite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// fmax is the larger of two numbers, ignoring a NaN, as Rust's f64::max does.
func fmax(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	if a > b {
		return a
	}
	return b
}

// inRange says whether lo <= x <= hi (false for NaN).
func inRange(x, lo, hi float64) bool {
	return x >= lo && x <= hi
}

// The saturating conversions of Rust's `as`: NaN is 0, and a value past either end is that end.

func toU64(x float64) uint64 {
	if !(x > 0) {
		return 0
	}
	if x >= 18446744073709551615.0 {
		return math.MaxUint64
	}
	return uint64(x)
}

func toU32(x float64) uint32 {
	if !(x > 0) {
		return 0
	}
	if x >= 4294967295.0 {
		return math.MaxUint32
	}
	return uint32(x)
}

func toU8(x float64) uint8 {
	if !(x > 0) {
		return 0
	}
	if x >= 255.0 {
		return math.MaxUint8
	}
	return uint8(x)
}

func toI64(x float64) int64 {
	if math.IsNaN(x) {
		return 0
	}
	if x >= 9223372036854775807.0 {
		return math.MaxInt64
	}
	if x <= -9223372036854775808.0 {
		return math.MinInt64
	}
	return int64(x)
}

func ldexp10(code uint8) float64 {
	return math.Ldexp(10, int(code))
}

// parseFloat reads a decimal number as Rust's str::parse::<f64> does for the forms the callers
// pass it (already checked: an optional sign, digits, a point, an exponent). An overflow is an
// infinity, not an error.
func parseFloat(t string) (float64, bool) {
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return v, true
		}
		return 0, false
	}
	return v, true
}

func absFloat(x float64) float64 { return math.Abs(x) }

// signbit says whether x is negative, -0 included.
func signbit(x float64) bool { return math.Signbit(x) }

func floorF(x float64) float64 { return math.Floor(x) }

func pad3(v uint32) string { return strconv.FormatUint(uint64(v)+1000, 10)[1:] }
