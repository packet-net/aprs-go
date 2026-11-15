// Package neutral is the conformance vectors' neutral data form: writing decoded data (for
// comparison) and reading it back (to build data for the encode cases). The rules are in the
// vectors' README. It is not public API: the vectors runner and the diff-dump tool share it. The
// writer itself is package aprs's (its encoder reads back with it) and is reached through
// internal/hook.
package neutral

import (
	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/hook"
)

// A neutral value is one of: nil, bool, string, int64 (a whole number as JSON writes it),
// float64, []any or map[string]any.

// Diagnostic gives a diagnostic as severity:code.
func Diagnostic(d aprs.Diagnostic) string {
	return d.Severity.String() + ":" + d.Code.ID()
}

// Diagnostics gives each diagnostic as severity:code.
func Diagnostics(ds []aprs.Diagnostic) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = Diagnostic(d)
	}
	return out
}

// Header gives a packet's header: source, destination, path and q-construct.
func Header(p *aprs.Packet) map[string]any {
	o := map[string]any{"source": p.Source.String(), "destination": p.Destination.String()}
	if len(p.Path) > 0 {
		o["path"] = path(p.Path)
	}
	if q, ok := p.QConstruct(); ok {
		qo := map[string]any{"construct": q.Construct}
		if q.Station != nil {
			qo["station"] = q.Station.String()
		}
		o["q_construct"] = qo
	}
	return o
}

func path(entries []aprs.PathEntry) []any {
	out := make([]any, len(entries))
	for i, e := range entries {
		s := e.Address.String()
		if e.Used {
			s += "*"
		}
		out[i] = s
	}
	return out
}

// Data gives data in the neutral form.
func Data(d aprs.Data) map[string]any {
	return hook.Neutral(d)
}

var (
	fixNames    = hook.Names.Fix
	sourceNames = hook.Names.Source
	originNames = hook.Names.Origin
	shapeNames  = hook.Names.Shape
	colorNames  = hook.Names.Color
	stormNames  = hook.Names.Storm
	daoNames    = hook.Names.DAO
	toneNames   = hook.Names.Tone
)
