package aprs

import (
	"math"
	"reflect"
	"sort"
	"strings"
)

// The read-back: the one final check every byte-writing path ends with (vectors README, Rules,
// Read-back).
//
// An encoder builds the forms it may write for the data (the preferred one first: a comment
// straight on, then after a '/', then after a space; a Mic-E altitude in a /A= where the bytes
// have no clean form), and readBack decodes each one leniently under a well-formed header and
// compares the result with the data given. The first form that matches is the bytes written; if
// none does, the data is refused. The size check comes after, on the bytes that passed.
//
// Nothing decides whether it runs, and no encoder returns bytes except through readBack: the
// encoders return forms, a type only an encoder can make and only readBack turns into bytes.

// forms are the byte forms an encoder may write for some data, most preferred first.
type forms struct {
	n  int
	at func(i int) []byte
	// dest is the destination a Mic-E report computed (the information field carries half its
	// position there), and micE says the read-back is done under it.
	dest Address
	micE bool
}

// onlyForm is data that has one form.
func onlyForm(b []byte) forms {
	return forms{n: 1, at: func(int) []byte { return faulted(b) }}
}

// formsOf is data with the forms at gives.
func formsOf(n int, at func(i int) []byte) forms {
	return forms{n: n, at: func(i int) []byte { return faulted(at(i)) }}
}

// writerFault is nil but in tests (export_test.go), which set it to make every writer's bytes
// wrong in one place, as a field writer that wrote a wrong byte would: the read-back has to refuse
// them. It is what lets a test show that each report type's public path ends in the read-back.
var writerFault func(b []byte) []byte

func faulted(b []byte) []byte {
	if writerFault == nil {
		return b
	}
	return writerFault(append([]byte(nil), b...))
}

// A refusal's text names what was found.
type mismatch struct {
	// what is the first thing wrong, for people.
	what string
	// paths are the neutral keys whose values differ, empty when a diagnostic is what is wrong.
	paths []string
}

// readBack is the final check: it returns the first of the forms whose bytes read back as data,
// and the destination a Mic-E report computed; or the refusal for data none of them does.
func readBack(data Data, f forms) ([]byte, Address, *EncodeError) {
	var first *mismatch
	for i := 0; i < f.n; i++ {
		b := f.at(i)
		m := readsBack(data, b, f)
		if m == nil {
			return b, f.dest, nil
		}
		if first == nil {
			first = m
		}
	}
	if first == nil {
		return nil, Address{}, newEncodeError(RefusalDoesNotReadBack, "there is no form to write for this data")
	}
	return nil, Address{}, refusalFor(data, first)
}

// allowedInfo says whether the read-back accepts an info the decode gives: only those the rules
// attach to the form the data itself asks for.
func allowedInfo(c Code) bool {
	switch c {
	case CodeObsoleteFormat, CodeMicEMissingDeviceType, CodeInvalidQuery, CodeInvalidTelemetryMetadata:
		return true
	}
	return false
}

// readsBack decodes the bytes written, leniently, under N0CALL>APZ001 (for Mic-E, N0CALL> and the
// destination computed) and compares the result with the data given: no warning or error, no
// info but the listed ones, and every field of the neutral form the same, at every depth and in
// both directions, but for the three allowances (rounding to the form's step, a value the form
// must carry, empty and absent as one). nil when they match.
func readsBack(data Data, b []byte, f forms) *mismatch {
	destination := Address{DefaultDestination}
	if f.micE {
		destination = f.dest
	}
	ctx := newContext(Lenient)
	back := decodeInformation(ctx, destination, b)
	for _, d := range ctx.diagnostics {
		if d.Severity != Info || !allowedInfo(d.Code) {
			return &mismatch{what: "it decodes with " + d.Severity.String() + " " + d.Code.ID() + ": " + d.Message}
		}
	}
	want := neutralData(expectedBack(data, back))
	got := neutralData(back)
	innerDiagnostics(want)
	innerDiagnostics(got)
	var paths []string
	nfDifferences(want, got, "", &paths)
	if len(paths) == 0 {
		return nil
	}
	return &mismatch{what: "it reads back with a different " + strings.TrimPrefix(paths[0], "."), paths: paths}
}

// innerDiagnostics leaves a third-party packet's inner diagnostics, which are part of its data, as
// the warnings and errors alone, in order: their infos, given or decoded, are not compared. The
// decode must hold the warnings and errors the data gives, and none it does not (the data of a
// packet decoded leniently gives its own).
func innerDiagnostics(m map[string]any) {
	for m != nil {
		packet, _ := m["packet"].(map[string]any)
		if packet == nil {
			return
		}
		if list, ok := packet["diagnostics"].([]any); ok {
			var kept []string
			for _, d := range list {
				if s, _ := d.(string); !strings.HasPrefix(s, "info:") {
					kept = append(kept, s)
				}
			}
			sort.Strings(kept)
			if len(kept) == 0 {
				delete(packet, "diagnostics")
			} else {
				packet["diagnostics"] = nfStrs(kept)
			}
		}
		m, _ = packet["data"].(map[string]any)
	}
}

// nfDifferences lists the neutral keys, as paths, at which the expected value and the actual one
// differ: exact key sets, strings, booleans and integers exactly, other numbers within 1e-9
// (relative above 1).
func nfDifferences(expected, actual any, at string, out *[]string) {
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			*out = append(*out, at)
			return
		}
		for k, ev := range e {
			if av, ok := a[k]; ok {
				nfDifferences(ev, av, at+"."+k, out)
			} else {
				*out = append(*out, at+"."+k)
			}
		}
		for k := range a {
			if _, ok := e[k]; !ok {
				*out = append(*out, at+"."+k)
			}
		}
	case []any:
		a, ok := actual.([]any)
		if !ok || len(a) != len(e) {
			*out = append(*out, at)
			return
		}
		for i := range e {
			nfDifferences(e[i], a[i], at, out)
		}
	case int64:
		if a, ok := actual.(int64); !ok || a != e {
			*out = append(*out, at)
		}
	case float64:
		if a, ok := actual.(float64); !ok || !sameNumber(e, a) {
			*out = append(*out, at)
		}
	default:
		if expected != actual {
			*out = append(*out, at)
		}
	}
}

// refusalFor gives the refusal for data whose bytes do not read back. A rule that names the fault
// comes first (an addressee that makes a bulletin of a message); free text that reads back
// changed is text-reads-as-field, and a weather report's software type, unit and extra fields
// are free text; otherwise the fallback, does-not-read-back.
func refusalFor(data Data, m *mismatch) *EncodeError {
	switch d := data.(type) {
	case Message:
		if isBulletinAddressee(d.Addressee) {
			return newEncodeError(RefusalInvalidAddressee, "an addressee of NWS-, NWS_ or BLN and a bulletin form makes a bulletin, not a message")
		}
	case Ack:
		if isBulletinAddressee(d.Addressee) {
			return newEncodeError(RefusalInvalidAddressee, "an addressee of NWS-, NWS_ or BLN and a bulletin form makes a bulletin, not this report")
		}
	case Reject:
		if isBulletinAddressee(d.Addressee) {
			return newEncodeError(RefusalInvalidAddressee, "an addressee of NWS-, NWS_ or BLN and a bulletin form makes a bulletin, not this report")
		}
	}
	if holdsFreeText(neutralData(data)) && !onlyPosition(m.paths) {
		return newEncodeError(RefusalTextReadsAsField, "text in the data reads back as something else: a structured element (altitude, !DAO!, telemetry, frequency, PHG/RNG/DFS, braces, a timestamp, locator or weather field) or an ack, query or bulletin; "+m.what)
	}
	return newEncodeError(RefusalDoesNotReadBack, "the bytes written do not read back as the data given: "+m.what)
}

// freeTextKeys are the neutral keys whose values are free text, which can read back as something
// else.
var freeTextKeys = map[string]bool{
	"comment": true, "text": true, "project": true, "names": true, "units": true, "signpost": true,
	"software": true, "unit": true, "extra": true, "capabilities": true, "data": true,
}

// holdsFreeText says whether some free-text field of the data is not empty, at any depth.
func holdsFreeText(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if freeTextKeys[k] {
				if s, ok := e.(string); ok && s == "" {
					continue
				}
				if _, isMap := e.(map[string]any); !isMap {
					return true
				}
			}
			if holdsFreeText(e) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if holdsFreeText(e) {
				return true
			}
		}
	}
	return false
}

// onlyPosition says whether the keys that differ are all the position's own, which no text can
// change.
func onlyPosition(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		last := p[strings.LastIndexByte(p, '.')+1:]
		if last != "latitude" && last != "longitude" && last != "ambiguity" {
			return false
		}
	}
	return true
}

// ---- what the bytes carry: the data given, rounded as the form written holds it

// expectedBack gives what data reads back as, by the Encoding rule: the data given, with each
// number the form holds in steps rounded once to the step of the form actually written (read
// from the decode of the bytes written, back), and the values the form must carry where the data
// leaves them out. Everything else is the data as given.
func expectedBack(given, back Data) Data {
	given, back = derefData(given), derefData(back)
	switch g := given.(type) {
	case PositionReport:
		b, _ := back.(PositionReport)
		g.Fields = expectedPositioned(g.Fields, &b.Fields, false, false)
		return g
	case ObjectReport:
		b, _ := back.(ObjectReport)
		g.Fields = expectedPositioned(g.Fields, &b.Fields, false, false)
		return g
	case ItemReport:
		b, _ := back.(ItemReport)
		g.Fields = expectedPositioned(g.Fields, &b.Fields, false, false)
		return g
	case MicEReport:
		b, _ := back.(MicEReport)
		g.Fields = expectedPositioned(g.Fields, &b.Fields, true, b.AltitudeInComment)
		return g
	case WeatherReport:
		g.Weather = *expectedWeather(&g.Weather, false)
		return g
	case Query:
		if g.Footprint != nil {
			f := *g.Footprint
			f.RadiusMiles = roundSteps(f.RadiusMiles)
			g.Footprint = &f
		}
		return g
	case NMEA:
		// An NMEA report is its sentence: what the sentence says and the data leaves out is read.
		b, _ := back.(NMEA)
		if !g.HasChecksum {
			g.HasChecksum = b.HasChecksum
		}
		if g.Latitude == nil {
			g.Latitude = b.Latitude
		}
		if g.Longitude == nil {
			g.Longitude = b.Longitude
		}
		if g.FixValid == nil {
			g.FixValid = b.FixValid
		}
		if g.CourseDegrees == nil {
			g.CourseDegrees = b.CourseDegrees
		}
		if g.SpeedKnots == nil {
			g.SpeedKnots = b.SpeedKnots
		}
		if g.AltitudeM == nil {
			g.AltitudeM = b.AltitudeM
		}
		if g.Time == nil {
			g.Time = b.Time
		}
		if g.Waypoint == nil {
			g.Waypoint = b.Waypoint
		}
		return g
	case ThirdParty:
		if g.Packet == nil {
			return g
		}
		inner := *g.Packet
		var innerBack Data
		if b, ok := back.(ThirdParty); ok && b.Packet != nil {
			innerBack = b.Packet.Data
		}
		inner.Data = expectedBack(inner.Data, innerBack)
		g.Packet = &inner
		return g
	}
	return given
}

// derefData is the data a pointer to one of the data types points to (nil for a nil one).
func derefData(d Data) Data {
	v := reflect.ValueOf(d)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return nil
	}
	x, _ := v.Interface().(Data)
	return x
}

// expectedPositioned is a position's fields rounded as the form that was written holds them. b is
// what the bytes decode to, which says the form (compressed, !DAO! precision, Mic-E altitude in
// the bytes or a /A=).
func expectedPositioned(g Positioned, b *Positioned, micE, altitudeInComment bool) Positioned {
	e := g
	precision := DAONone
	if b.DAO != nil {
		precision = b.DAO.Precision
	}
	e.Position = carriedPosition(g.Position, b.Compressed, precision)
	// A compressed report whose cs bytes carry a value says its type: the default, when the data
	// gives none.
	if b.Compressed && g.Compression == nil && b.Compression != nil {
		e.Compression = &CompressionType{Fix: FixCurrent, Source: SourceOther, Origin: OriginSoftware}
	}
	// Mic-E always carries a speed.
	if micE && g.SpeedKnots == nil && b.SpeedKnots != nil && *b.SpeedKnots == 0 {
		e.SpeedKnots = Ptr(0.0)
	}
	if g.CourseDegrees != nil {
		c := *g.CourseDegrees
		if b.Compressed {
			// The cs bytes hold 4-degree steps, and north is 360.
			if steps := roundSteps(c / 4); math.Mod(steps, 90) == 0 {
				c = 360
			} else {
				c = steps * 4
			}
		} else {
			c = roundSteps(c)
		}
		e.CourseDegrees = Ptr(c)
	}
	if g.SpeedKnots != nil {
		s := *g.SpeedKnots
		if b.Compressed {
			s = math.Pow(1.08, roundSteps(math.Log(s+1)/math.Log(1.08))) - 1
		} else {
			s = roundSteps(s)
		}
		e.SpeedKnots = Ptr(s)
	}
	if g.AltitudeFeet != nil {
		feet := *g.AltitudeFeet
		switch {
		case micE && !altitudeInComment:
			// Whole metres, compared in feet.
			feet = roundSteps(feet/metresToFeet) * metresToFeet
		case b.Compressed && b.Compression != nil && b.Compression.Source == SourceGGA:
			steps := 0.0
			if feet > 1 {
				steps = roundSteps(math.Log(feet) / math.Log(1.002))
			}
			carried := math.Pow(1.002, steps)
			if g.Weather != nil || math.Abs(carried-feet) <= 1e-9*math.Abs(feet) {
				// A compressed weather report has no comment to hold a /A=, and one the cs bytes hold
				// exactly has none either.
				feet = carried
			} else {
				feet = roundSteps(feet)
			}
		default:
			feet = roundSteps(feet)
		}
		e.AltitudeFeet = Ptr(feet)
	}
	if g.RangeMiles != nil {
		r := *g.RangeMiles
		if b.Compressed {
			r = 2 * math.Pow(1.08, roundSteps(math.Log(r/2)/math.Log(1.08)))
		} else {
			r = roundSteps(r)
		}
		e.RangeMiles = Ptr(r)
	}
	if g.Frequency != nil {
		f := *g.Frequency
		perMHz := 1000.0
		if f.TenKHzResolution {
			perMHz = 100
		}
		units := roundSteps(f.MHz * perMHz)
		f.MHz = math.Floor(units/perMHz) + (units-math.Floor(units/perMHz)*perMHz)/perMHz
		e.Frequency = &f
	}
	if g.Weather != nil {
		e.Weather = expectedWeather(g.Weather, b.Compressed)
	}
	return e
}

// carriedPosition is the position a report reads back as: the value rounded to hundredths of a
// minute (to the !DAO! digits' step with one), to the base-91 steps of a compressed position, or
// for an ambiguous one the centre of the box that holds it.
func carriedPosition(p Position, compressed bool, precision DAOPrecision) Position {
	signed := func(v, magnitude float64) float64 {
		if math.Signbit(v) {
			return -magnitude
		}
		return magnitude
	}
	switch {
	case compressed:
		r := compressedReported(p)
		p.Latitude, p.Longitude = r.Latitude, r.Longitude
	case p.Ambiguity > 0 && p.Ambiguity <= 4:
		half := [...]float64{1, 10, 100, 1000, 6000}[p.Ambiguity] / 2
		p.Latitude = signed(p.Latitude, (float64(cutToBox(math.Abs(p.Latitude), p.Ambiguity))+half)/6000)
		p.Longitude = signed(p.Longitude, (float64(cutToBox(math.Abs(p.Longitude), p.Ambiguity))+half)/6000)
	default:
		step := 6000.0
		switch precision {
		case DAOThousandths:
			step = 60000
		case DAOBase91:
			step = 546000
		}
		p.Latitude = signed(p.Latitude, roundSteps(math.Abs(p.Latitude)*step)/step)
		p.Longitude = signed(p.Longitude, roundSteps(math.Abs(p.Longitude)*step)/step)
	}
	return p
}

// expectedWeather is weather rounded as the form that was written holds it; csWind says the wind
// is in a compressed position's cs bytes (4-degree steps, and speeds in the 1.08 steps), not in a
// data extension of whole units.
func expectedWeather(g *Weather, csWind bool) *Weather {
	e := *g
	whole := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		return Ptr(roundSteps(*v))
	}
	steps := func(v *float64, perUnit float64) *float64 {
		if v == nil {
			return nil
		}
		return Ptr(roundSteps(*v*perUnit) / perUnit)
	}
	e.WindDirectionDegrees = whole(g.WindDirectionDegrees)
	e.WindSpeedMph = whole(g.WindSpeedMph)
	if csWind {
		if g.WindDirectionDegrees != nil {
			e.WindDirectionDegrees = Ptr(math.Mod(roundSteps(*g.WindDirectionDegrees/4), 90) * 4)
		}
		if g.WindSpeedMph != nil {
			knots := *g.WindSpeedMph / knotsToMph
			e.WindSpeedMph = Ptr((math.Pow(1.08, roundSteps(math.Log(knots+1)/math.Log(1.08))) - 1) * knotsToMph)
		}
	}
	e.WindGustMph = whole(g.WindGustMph)
	e.TemperatureF = whole(g.TemperatureF)
	e.Rain1hIn = steps(g.Rain1hIn, 100)
	e.Rain24hIn = steps(g.Rain24hIn, 100)
	e.RainMidnightIn = steps(g.RainMidnightIn, 100)
	e.HumidityPercent = whole(g.HumidityPercent)
	e.PressureMbar = steps(g.PressureMbar, 10)
	e.LuminosityWM2 = whole(g.LuminosityWM2)
	if g.Snow24hIn != nil {
		e.Snow24hIn = Ptr(carriedSnowfall(*g.Snow24hIn))
	}
	return &e
}

// carriedSnowfall is the snowfall its three characters hold: hundredths below 1 inch, tenths
// below 10, else whole inches (Encoding, snowfall's three forms).
func carriedSnowfall(inches float64) float64 {
	hundredths := roundSteps(inches * 100)
	tenths := roundSteps(inches * 10)
	switch {
	case hundredths < 100:
		return hundredths / 100
	case tenths < 100:
		return tenths / 10
	}
	return roundSteps(inches)
}
