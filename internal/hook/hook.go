// Package hook lets the vectors' tooling reach what package aprs holds for its own read-back: the
// neutral data form, which the encoder compares with, and the names it gives enumerations. It is
// not public API. Package aprs sets these when it is initialised; the packages that import aprs
// (internal/neutral, which the vectors runner and the dump tool share) read them.
package hook

// Neutral writes data (an aprs.Data, passed as any) in the vectors' neutral data form.
var Neutral func(data any) map[string]any

// Names are the neutral form's names for the enumerations, in value order.
var Names struct {
	Fix, Source, Origin, Shape, Color, Storm, DAO, Tone []string
}
