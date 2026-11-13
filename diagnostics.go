package aprs

import (
	"fmt"
)

// Severity is how serious a diagnostic is.
type Severity uint8

const (
	// Info: worth knowing, but nothing is wrong with the packet.
	Info Severity = iota
	// Warning: the packet breaks the spec in a way the decoder tolerated (see ParseOptions).
	Warning
	// Error: the packet breaks the spec and what it says could not be decoded.
	Error
)

// String gives the lower-case name used in the conformance vectors: info, warning or error.
func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return fmt.Sprintf("severity-%d", uint8(s))
}

// AllCodes gives every diagnostic code, in catalogue order.
func AllCodes() []Code {
	out := make([]Code, len(codeIDs))
	for i := range out {
		out[i] = Code(i)
	}
	return out
}

// ID gives the code's id in the conformance vectors, such as "lowercase-hemisphere".
func (c Code) ID() string {
	if int(c) < len(codeIDs) {
		return codeIDs[c]
	}
	return fmt.Sprintf("code-%d", uint8(c))
}

// String gives the code's id.
func (c Code) String() string {
	return c.ID()
}

// Tolerable says whether a lenient decoder may accept this defect with a warning (a strict one
// rejects it).
func (c Code) Tolerable() bool {
	return int(c) < len(codeTolerable) && codeTolerable[c]
}

// CodeFromID gives the code with this vectors id, if there is one.
func CodeFromID(id string) (Code, bool) {
	for i, s := range codeIDs {
		if s == id {
			return Code(i), true
		}
	}
	return 0, false
}

// Diagnostic is something a decoder noticed about a packet.
type Diagnostic struct {
	// Severity says how serious it is.
	Severity Severity
	// Code says what it is.
	Code Code
	// Message is a sentence saying what was found, for people. Its wording is not part of the
	// stable API.
	Message string
	// Offset is the byte offset in the information field (or the TNC2 line, for a header
	// problem) it refers to, or -1 when there is none.
	Offset int
}

// String gives the diagnostic for people: severity, code, message and any offset.
func (d Diagnostic) String() string {
	s := fmt.Sprintf("%s %s: %s", d.Severity, d.Code, d.Message)
	if d.Offset >= 0 {
		s += fmt.Sprintf(" (at %d)", d.Offset)
	}
	return s
}

// ParseOptions says which spec deviations a decoder tolerates. Each tolerable Code is one
// switch: when it is tolerated the defect is accepted and reported as a Warning, and when it is
// not, the defect is an error and the data is not decoded (or, for the few codes that are about
// an extra reading rather than a defect, the extra reading is not made).
//
// The zero value is Lenient, which tolerates everything tolerable and is the usual choice;
// Strict tolerates nothing.
type ParseOptions struct {
	// rejected holds the tolerable codes not tolerated, one bit for each of the 256 codes a Code
	// can be, so that the zero value tolerates them all.
	rejected [4]uint64
}

// Lenient tolerates every tolerable defect, each with a warning. It is the zero ParseOptions.
var Lenient = ParseOptions{}

// Strict tolerates nothing: exactly what APRS12c allows.
var Strict = func() ParseOptions {
	o := Lenient
	for _, c := range AllCodes() {
		o = o.Without(c)
	}
	return o
}()

// Tolerates says whether code is tolerated. Always false for a code that is not tolerable.
func (o ParseOptions) Tolerates(code Code) bool {
	return code.Tolerable() && o.rejected[code/64]&(1<<(code%64)) == 0
}

// With gives these options also tolerating code (ignored if the code is not tolerable).
func (o ParseOptions) With(code Code) ParseOptions {
	o.rejected[code/64] &^= 1 << (code % 64)
	return o
}

// Without gives these options no longer tolerating code (ignored if the code is not tolerable).
func (o ParseOptions) Without(code Code) ParseOptions {
	if code.Tolerable() {
		o.rejected[code/64] |= 1 << (code % 64)
	}
	return o
}
