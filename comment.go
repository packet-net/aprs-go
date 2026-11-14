package aprs

import (
	"math"
	"sort"
)

// Everything after a position: data extensions (APRS12c ch. 7), weather (ch. 12), storm data,
// and the comment with what can be lifted out of it: altitude, !DAO!, base-91 telemetry, a voice
// frequency and signpost text.

const (
	knotsToMph   = 1.150779448023543
	metresToFeet = 1.0 / 0.3048
)

func isWeatherSymbol(f *Positioned) bool { return f.Symbol.Code == '_' }
func isDFSymbol(f *Positioned) bool      { return f.Symbol.Table == '/' && f.Symbol.Code == '\\' }
func isAreaSymbol(f *Positioned) bool    { return f.Symbol.Table == '\\' && f.Symbol.Code == 'l' }

// isStormSymbol: the hurricane symbol, in either table or with an overlay.
func isStormSymbol(f *Positioned) bool    { return f.Symbol.Code == '@' }
func isSignpostSymbol(f *Positioned) bool { return f.Symbol.Table == '\\' && f.Symbol.Code == 'm' }

// span is a range of bytes, start included and end not.
type span struct{ start, end int }

func (s span) length() int { return s.end - s.start }

// drain takes c[from:to] out of c, in place.
func drain(c []byte, from, to int) []byte {
	return append(c[:from], c[to:]...)
}

// decodeComment reads everything after the position into fields: for an uncompressed position b
// starts with the data extension, for a compressed one after the type byte. False when a defect
// was not tolerated.
func decodeComment(ctx *context, fields *Positioned, bytes *cs, b []byte, offset int) bool {
	weatherSymbol := isWeatherSymbol(fields)
	weather := Weather{}
	csIsWind := false

	if bytes != nil {
		t := bytes.t
		fields.Compression = &t
		c, s := int(bytes.c)-33, int(bytes.s)-33
		switch {
		case bytes.t.Source == SourceGGA:
			fields.AltitudeFeet = Ptr(math.Pow(1.002, float64(c*91+s)))
		case bytes.c == '{':
			fields.RangeMiles = Ptr(2 * math.Pow(1.08, float64(s)))
		case weatherSymbol:
			weather.WindDirectionDegrees = Ptr(float64(c * 4))
			weather.WindSpeedMph = Ptr((math.Pow(1.08, float64(s)) - 1) * knotsToMph)
			csIsWind = true
		default:
			// The compressed course has no "unknown": 0 is north, reported as 360 (interpretations.md).
			if c == 0 {
				fields.CourseDegrees = Ptr(360.0)
			} else {
				fields.CourseDegrees = Ptr(float64(c * 4))
			}
			fields.SpeedKnots = Ptr(math.Pow(1.08, float64(s)) - 1)
		}
	}

	if weatherSymbol {
		// A weather station's report is weather, whatever else it holds: the wind, then the fields.
		at := 0
		var wind windMode
		if len(b) >= 7 && isCourseSpeed(b[:7]) {
			if fields.Compressed && !ctx.tolerate(CodeWindExtensionAfterCompressed, "an uncompressed wind extension after a compressed weather position (UAP 5.33)", offset) {
				return false
			}
			// The extension replaces any wind in the cs bytes, unknown included (interpretations.md).
			dir, dirOK, speed, speedOK := courseSpeedValues(b[:7])
			weather.WindDirectionDegrees = nil
			if dirOK {
				if dir > 360 {
					if !ctx.tolerate(CodeOutOfRangeValue, "a wind direction over 360 degrees was dropped", offset) {
						return false
					}
				} else {
					weather.WindDirectionDegrees = Ptr(float64(dir))
				}
			}
			weather.WindSpeedMph = nil
			if speedOK {
				weather.WindSpeedMph = Ptr(float64(speed))
			}
			at = 7
			wind = windKnown
		} else if csIsWind {
			wind = windKnown
		} else {
			// The wind is decided here, where the extension belongs, before any field is read
			// (vectors README, "The order of checks"): sent as c and s fields in its place, or
			// missing. After a compressed position the cs bytes say it is unknown, not missing.
			probe, ok := weatherFields(newContext(Lenient), b[:weatherEnd(b)], offset, &Weather{}, windAsFields)
			// The wind comes as fields once a c field is read (an s is only the speed after one).
			direction, speed := false, false
			if ok {
				direction, speed = probe.direction, probe.speed
			}
			if direction && !ctx.tolerate(CodeWindFieldsInsteadOfExtension, "wind sent as c/s fields instead of the DDD/SSS extension", offset) {
				return false
			}
			windComplete := fields.Compressed || (direction && speed)
			why := "the weather report has no wind direction/speed extension (APRS12c ch. 12)"
			if direction {
				why = "the wind needs both a direction and a speed (APRS12c ch. 12)"
			}
			if !windComplete && !ctx.tolerate(CodeIncompleteWeather, why, offset) {
				return false
			}
			wind = windAsFields
		}
		run, ok := weatherFields(ctx, b[at:weatherEnd(b)], offset+at, &weather, wind)
		if !ok {
			return false
		}
		at += run.length
		// Wind read from the bytes after a compressed position is written back into its cs bytes,
		// under the default type byte.
		if fields.Compressed && !csIsWind && fields.Compression == nil && (weather.WindDirectionDegrees != nil || weather.WindSpeedMph != nil) {
			fields.Compression = &CompressionType{Fix: FixCurrent, Source: SourceOther, Origin: OriginSoftware}
		}
		fields.Weather = &weather
		return weatherTail(ctx, fields, b[at:], offset+at)
	}

	at := 0
	if !fields.Compressed {
		n, ok := extensionAtStart(ctx, fields, b, offset)
		if !ok {
			return false
		}
		at = n
	}
	return commentTail(ctx, fields, b[at:], offset+at, -1)
}

// weatherEnd gives where the weather text of a positioned report ends in b: the start of the
// first base-91 telemetry block or !DAO! that commentTail would lift (lifted first, and so not
// part of the weather's text, wherever it sits), else the end of b. A run-on value that reaches
// it counts as ending at the weather's end, and the text after it is comment text on its own.
func weatherEnd(b []byte) int {
	end := len(b)
	for _, sp := range telemetryAndDAOSpans(b) {
		if sp.start < end {
			end = sp.start
		}
	}
	return end
}

// weatherTail reads what follows a position's weather data: base-91 telemetry and a !DAO! are
// lifted out first, and what is left is the software type and unit, or else text that a weather
// report should not have.
func weatherTail(ctx *context, fields *Positioned, b []byte, offset int) bool {
	c := append([]byte(nil), b...)
	c, ok := liftTelemetryAndDAO(ctx, fields, c, offset, -1)
	if !ok {
		return false
	}
	if fields.Weather == nil {
		fields.Weather = &Weather{}
	}
	if len(c) == 0 || softwareAndUnit(c, fields.Weather) {
		return true
	}
	if !ctx.tolerate(CodeWeatherComment, "text after the weather data; a weather report has no comment (UAP 2.7.1)", offset) {
		return false
	}
	return finishComment(ctx, fields, c, offset)
}

// liftTelemetryAndDAO lifts base-91 telemetry and a !DAO! out of c. The telemetry block is
// located first, so a DAO-shaped run inside it is not taken for a DAO. A !DAO! is five bytes as
// sent, so one that spans joined (where bytes were taken out before, bringing the bytes on either
// side together; -1 for none) is not one.
func liftTelemetryAndDAO(ctx *context, fields *Positioned, c []byte, offset, joined int) ([]byte, bool) {
	tSpan, t, tOK := findTelemetry(c)
	var tp *span
	if tOK {
		tp = &tSpan
	}
	dSpan, found, dOK := findDAO(c, tp, joined)
	var removals []span
	if tOK {
		fields.Telemetry = &t
		removals = append(removals, tSpan)
	}
	if dOK {
		if !applyDAO(ctx, fields, found, offset+dSpan.start) {
			return c, false
		}
		removals = append(removals, dSpan)
	}
	sort.SliceStable(removals, func(i, j int) bool { return removals[i].start > removals[j].start })
	for _, r := range removals {
		c = drain(c, r.start, r.end)
	}
	return c, true
}

// telemetryAndDAOSpans says where the base-91 telemetry and !DAO! commentTail would lift out of c
// are, last first.
func telemetryAndDAOSpans(c []byte) []span {
	var spans []span
	tSpan, _, tOK := findTelemetry(c)
	var tp *span
	if tOK {
		tp = &tSpan
		spans = append(spans, tSpan)
	}
	if dSpan, _, ok := findDAO(c, tp, -1); ok {
		spans = append(spans, dSpan)
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	return spans
}

// withoutTelemetryAndDAO gives c with its base-91 telemetry and !DAO! taken out, as commentTail
// would lift them.
func withoutTelemetryAndDAO(c []byte) []byte {
	rest := append([]byte(nil), c...)
	for _, r := range telemetryAndDAOSpans(c) {
		rest = drain(rest, r.start, r.end)
	}
	return rest
}

// commentTail reads the comment after any data extension: it lifts out telemetry and a !DAO!
// (from the end), then an altitude, signpost or corridor braces, a data extension found later in
// the text, and a voice frequency at the start, in the order that keeps one from being taken for
// another; the rest, less one leading delimiter, is the comment. joined is where bytes were
// already taken out of the text (a Mic-E altitude), which a !DAO! does not span, or -1.
func commentTail(ctx *context, fields *Positioned, b []byte, offset, joined int) bool {
	c := append([]byte(nil), b...)
	c, ok := liftTelemetryAndDAO(ctx, fields, c, offset, joined)
	if !ok {
		return false
	}

	// A /A= altitude is read wherever it is, and wins over a compressed or Mic-E one.
	if i, feet, ok := findAltitude(c); ok {
		fields.AltitudeFeet = Ptr(float64(feet))
		c = drain(c, i, i+9)
	}

	c = braces(fields, c)

	if fields.PHG == nil && fields.RangeMiles == nil && fields.DFS == nil {
		c = lateExtension(ctx, fields, c, offset)
	}

	// A voice frequency at the start, after at most one delimiter, and one space after it.
	start := 0
	if len(c) > 0 && (c[0] == ' ' || c[0] == '/') {
		start = 1
	}
	if f, n, ok := frequencyAt(c[start:]); ok {
		fields.Frequency = &f
		end := start + n
		if end < len(c) && c[end] == ' ' {
			end++
		}
		c = drain(c, 0, end)
	}

	return finishComment(ctx, fields, c, offset)
}

// finishComment: one leading delimiter, a space or /, is not part of the text (APRS12c ch. 18);
// the rest is the comment.
func finishComment(ctx *context, fields *Positioned, c []byte, offset int) bool {
	if len(c) > 0 && (c[0] == ' ' || c[0] == '/') {
		c = c[1:]
	}
	comment, ok := decodeText(ctx, c, offset)
	if !ok {
		return false
	}
	fields.Comment = comment
	return true
}

// braces reads signpost text {ttt} on the \m symbol, or a corridor width {www} on a line area
// object: the first well-formed braces in the comment, wherever they are (APRS12c ch. 11:
// "enclosing the 1-3 characters in braces in the comment field"). Well formed is {, 1-3 characters
// that are not braces, and }, the characters printable ASCII for a signpost or digits for a
// corridor; braces that are not are comment text and do not stop the search, so {5Wm{55} and {{5}
// hold 55 and 5.
func braces(fields *Positioned, c []byte) []byte {
	signpost := isSignpostSymbol(fields)
	corridor := fields.Area != nil && (fields.Area.Shape == AreaLineDownRight || fields.Area.Shape == AreaLineDownLeft)
	if !signpost && !corridor {
		return c
	}
	wellFormed := func(inner []byte) bool {
		if len(inner) < 1 || len(inner) > 3 || contains(inner, '{') || contains(inner, '}') {
			return false
		}
		if signpost {
			return isPrintableASCII(inner)
		}
		return allDigits(inner)
	}
	open, closing := -1, -1
	for i := 0; i < len(c); i++ {
		if c[i] != '{' {
			continue
		}
		j := indexByte(c[i+1:], '}')
		if j < 0 {
			continue
		}
		if wellFormed(c[i+1 : i+1+j]) {
			open, closing = i, i+1+j
			break
		}
	}
	if open < 0 {
		return c
	}
	inner := c[open+1 : closing]
	if signpost {
		fields.Signpost = Ptr(string(inner))
	} else if fields.Area != nil {
		area := *fields.Area
		area.CorridorWidthMiles = Ptr(uint16(digits(inner)))
		fields.Area = &area
	}
	return drain(c, open, closing+1)
}

// lateExtension reads a PHG, RNG or DFS extension after other comment text instead of straight
// after the symbol: the first well-formed PHG, else the first well-formed RNG, else the first
// well-formed DFS. A malformed one such as PHG12 is comment text and does not stop the search
// (vectors interpretations.md, "A late data extension is the first well-formed one"). Strictly
// that is free text (UAP 5.15); recognising it is an extra reading the options allow.
func lateExtension(ctx *context, fields *Positioned, c []byte, offset int) []byte {
	for _, tag := range []string{"PHG", "RNG", "DFS"} {
		var found Positioned
		at, n := -1, 0
		for i := 0; i+3 <= len(c) && i < len(c)-2; i++ {
			if string(c[i:i+3]) != tag {
				continue
			}
			found = Positioned{}
			if l, ok := stationExtension(&found, c[i:]); ok {
				at, n = i, l
				break
			}
		}
		if at < 0 {
			continue
		}
		if ctx.allows(CodeDataExtensionInComment, "a data extension found later in the comment; the spec puts it straight after the symbol (UAP 5.15)", offset+at) {
			fields.PHG, fields.RangeMiles, fields.DFS = found.PHG, found.RangeMiles, found.DFS
			c = drain(c, at, at+n)
		}
		return c
	}
	return c
}

func isCourseSpeed(ext []byte) bool {
	part := func(b []byte) bool {
		digits, dots, spaces := true, true, true
		for _, x := range b {
			digits = digits && isDigit(x)
			dots = dots && x == '.'
			spaces = spaces && x == ' '
		}
		return digits || dots || spaces
	}
	return len(ext) >= 7 && ext[3] == '/' && part(ext[0:3]) && part(ext[4:7])
}

func courseSpeedValues(ext []byte) (uint32, bool, uint32, bool) {
	c, s := ext[0:3], ext[4:7]
	return digits(c), allDigits(c), digits(s), allDigits(s)
}

// extensionAtStart reads a data extension at the start of an uncompressed position's comment. It
// gives the number of bytes used, or false when a defect was not tolerated.
func extensionAtStart(ctx *context, fields *Positioned, b []byte, offset int) (int, bool) {
	if len(b) < 7 {
		return 0, true
	}
	ext := b[:7]
	// An area object's Tyy/Cxx looks like course/speed; the area symbol says which it is.
	if isAreaSymbol(fields) {
		if a, ok := area(ext); ok {
			fields.Area = &a
			return 7, true
		}
	}
	if isCourseSpeed(ext) {
		course, courseOK, speed, speedOK := courseSpeedValues(ext)
		if courseOK {
			if course > 360 {
				if !ctx.tolerate(CodeOutOfRangeValue, "a course over 360 degrees was dropped", offset) {
					return 0, false
				}
			} else {
				fields.CourseDegrees = Ptr(float64(course))
			}
		}
		fields.SpeedKnots = nil
		if speedOK {
			fields.SpeedKnots = Ptr(float64(speed))
		}
		if isDFSymbol(fields) {
			if rest, ok := get(b, 7, 15); ok {
				if br, ok := dfBearing(rest); ok {
					// The bearing is degrees: one over 360 drops the whole /BRG/NRQ, as an
					// out-of-range course is dropped (vectors README, Positions).
					if br.BearingDegrees > 360 {
						if !ctx.tolerate(CodeOutOfRangeValue, "a DF bearing over 360 degrees was dropped", offset+7) {
							return 0, false
						}
					} else {
						fields.DFBearing = &br
					}
					return 15, true
				}
			}
		} else if isStormSymbol(fields) {
			if st, n, ok := storm(b[7:]); ok {
				fields.Storm = &st
				return 7 + n, true
			}
		}
		return 7, true
	}
	n, _ := stationExtension(fields, b)
	return n, true
}

// micEExtension reads a data extension at the start of a Mic-E status text (PHG, RNG, DFS, or an
// area on the area symbol; not course and speed, which Mic-E carries itself); its length.
func micEExtension(fields *Positioned, b []byte) (int, bool) {
	if len(b) < 7 {
		return 0, false
	}
	ext := b[:7]
	if isCourseSpeed(ext) {
		return 0, false
	}
	if isAreaSymbol(fields) {
		if a, ok := area(ext); ok {
			fields.Area = &a
			return 7, true
		}
	}
	return stationExtension(fields, b)
}

// stationExtension reads PHG, RNG or DFS at the start of b into fields; its length.
func stationExtension(fields *Positioned, b []byte) (int, bool) {
	if p, n, ok := phg(b); ok {
		fields.PHG = &p
		return n, true
	}
	if len(b) < 7 {
		return 0, false
	}
	ext := b[:7]
	if hasPrefix(ext, "RNG") && allDigits(ext[3:7]) {
		fields.RangeMiles = Ptr(float64(digits(ext[3:7])))
		return 7, true
	}
	if d, ok := dfs(ext); ok {
		fields.DFS = &d
		return 7, true
	}
	return 0, false
}

// isPHGCodes: PHG and DFS codes are digits, except the height, which goes on past 9 through the
// ASCII table (: is 10, 2^10 x 10 feet), up to ~.
func isPHGCodes(ext []byte) bool {
	return isDigit(ext[3]) && ext[4] >= '0' && ext[4] <= '~' && isDigit(ext[5]) && isDigit(ext[6])
}

// phg reads PHGphgd, or PHGR PHGphgdR/.
func phg(b []byte) (PHG, int, bool) {
	if len(b) < 7 {
		return PHG{}, 0, false
	}
	ext := b[:7]
	if !hasPrefix(ext, "PHG") || !isPHGCodes(ext) {
		return PHG{}, 0, false
	}
	p := PHG{Power: ext[3] - '0', Height: ext[4] - '0', Gain: ext[5] - '0', Directivity: ext[6] - '0'}
	n := 7
	if len(b) >= 9 && b[8] == '/' {
		if rate, ok := beaconRate(b[7]); ok {
			p.BeaconsPerHour = &rate
			n = 9
		}
	}
	return p, n, true
}

// dfs reads DFSshgd.
func dfs(ext []byte) (DFS, bool) {
	if !hasPrefix(ext, "DFS") || !isPHGCodes(ext) {
		return DFS{}, false
	}
	return DFS{Strength: ext[3] - '0', Height: ext[4] - '0', Gain: ext[5] - '0', Directivity: ext[6] - '0'}, true
}

// beaconRate reads a PHGR beacon rate: 1-9, then A = 10 up to Z = 35 per hour.
func beaconRate(b byte) (uint8, bool) {
	switch {
	case b >= '1' && b <= '9':
		return b - '0', true
	case isUpper(b):
		return b - 'A' + 10, true
	}
	return 0, false
}

func beaconRateChar(rate uint8) (byte, bool) {
	switch {
	case rate >= 1 && rate <= 9:
		return '0' + rate, true
	case rate >= 10 && rate <= 35:
		return 'A' + rate - 10, true
	}
	return 0, false
}

// dfBearing reads /BRG/NRQ after the course and speed of a DF report.
func dfBearing(b []byte) (DFBearing, bool) {
	if len(b) < 8 || b[0] != '/' || b[4] != '/' || !allDigits(b[1:4]) || !allDigits(b[5:8]) {
		return DFBearing{}, false
	}
	return DFBearing{BearingDegrees: uint16(digits(b[1:4])), Number: b[5] - '0', Range: b[6] - '0', Quality: b[7] - '0'}, true
}

// storm reads /ST/www^GGG/pppp>RRR&rrr and optionally %ggg (APRS12c ch. 12).
func storm(b []byte) (Storm, int, bool) {
	if len(b) < 24 || b[0] != '/' {
		return Storm{}, 0, false
	}
	var kind StormKind
	switch string(b[1:3]) {
	case "TS":
		kind = TropicalStorm
	case "HC":
		kind = Hurricane
	case "TD":
		kind = TropicalDepression
	default:
		return Storm{}, 0, false
	}
	// field gives the value and whether there is such a field: its marker and then digits (a field
	// of dots or spaces is not storm data, unlike a weather value).
	field := func(at int, marker byte, width int) (*uint16, bool) {
		f, ok := get(b, at, at+1+width)
		if !ok || f[0] != marker || !allDigits(f[1:]) {
			return nil, false
		}
		return Ptr(uint16(digits(f[1:]))), true
	}
	s := Storm{Kind: kind}
	var ok bool
	if s.SustainedWindKnots, ok = field(3, '/', 3); !ok {
		return Storm{}, 0, false
	}
	if s.GustKnots, ok = field(7, '^', 3); !ok {
		return Storm{}, 0, false
	}
	if s.CentralPressureMbar, ok = field(11, '/', 4); !ok {
		return Storm{}, 0, false
	}
	if s.HurricaneRadiusNM, ok = field(16, '>', 3); !ok {
		return Storm{}, 0, false
	}
	if s.TropicalStormRadiusNM, ok = field(20, '&', 3); !ok {
		return Storm{}, 0, false
	}
	n := 24
	if g, ok := field(24, '%', 3); ok {
		s.WholeGaleRadiusNM = g
		n = 28
	}
	return s, n, true
}

// area reads TyyCCxx (CC is /0-/9 or 10-15). A line's corridor width {www} is read with the
// comment's braces.
func area(b []byte) (AreaObject, bool) {
	if !isDigit(b[0]) || !allDigits(b[1:3]) || !allDigits(b[5:7]) {
		return AreaObject{}, false
	}
	var color byte
	switch {
	case b[3] == '/' && isDigit(b[4]):
		color = b[4] - '0'
	case b[3] == '1' && b[4] >= '0' && b[4] <= '5':
		color = 10 + b[4] - '0'
	default:
		return AreaObject{}, false
	}
	return AreaObject{
		Shape:     AreaShape(b[0] - '0'),
		LatOffset: uint8(digits(b[1:3])),
		Color:     AreaColor(color),
		LonOffset: uint8(digits(b[5:7])),
	}, true
}

// findAltitude finds /A=nnnnnn anywhere in the comment (APRS12c ch. 6), or the de facto
// /A=-nnnnn.
func findAltitude(c []byte) (int, int32, bool) {
	for i := 0; i+9 <= len(c); i++ {
		w := c[i : i+9]
		if string(w[:3]) != "/A=" {
			continue
		}
		if allDigits(w[3:9]) {
			return i, int32(digits(w[3:9])), true
		}
		if w[3] == '-' && allDigits(w[4:9]) {
			return i, -int32(digits(w[4:9])), true
		}
	}
	return 0, 0, false
}

// findTelemetry finds base-91 comment telemetry |ss11...|: 2-7 pairs between bars.
func findTelemetry(c []byte) (span, CommentTelemetry, bool) {
	// Only the last two bars are considered: telemetry goes at the end of the comment.
	end := lastIndexByte(c, '|')
	if end < 0 {
		return span{}, CommentTelemetry{}, false
	}
	start := lastIndexByte(c[:end], '|')
	if start < 0 {
		return span{}, CommentTelemetry{}, false
	}
	inner := c[start+1 : end]
	if len(inner) < 4 || len(inner) > 14 || len(inner)%2 != 0 {
		return span{}, CommentTelemetry{}, false
	}
	for _, b := range inner {
		if !isBase91(b) {
			return span{}, CommentTelemetry{}, false
		}
	}
	values := make([]uint16, 0, len(inner)/2)
	for i := 0; i < len(inner); i += 2 {
		v, _ := base91Decode(inner[i : i+2])
		values = append(values, uint16(v))
	}
	t := CommentTelemetry{Sequence: values[0]}
	rest := values[1:]
	if len(rest) == 6 {
		t.Analog = append([]uint16(nil), rest[:5]...)
		t.Digital = Ptr(uint8(rest[5]))
	} else {
		t.Analog = append([]uint16(nil), rest...)
	}
	return span{start, end + 1}, t, true
}

// foundDAO is a !DAO! and the extra minutes of latitude and longitude it adds.
type foundDAO struct {
	dao      DAO
	lat, lon float64
}

// findDAO finds the last !DAO! outside the telemetry block, and not across joined: !Wdd! (an
// upper-case datum, a digit each: thousandths of a minute), !wBB! (a lower-case datum, a base-91
// character each: v/91 hundredths of a minute) or !D  ! (datum only; a letter or a digit).
func findDAO(c []byte, telemetry *span, joined int) (span, foundDAO, bool) {
	for i := len(c) - 5; i >= 0; i-- {
		if telemetry != nil && i+4 >= telemetry.start && i < telemetry.end {
			continue
		}
		if joined >= 0 && i < joined && joined < i+5 {
			continue
		}
		w := c[i : i+5]
		if w[0] != '!' || w[4] != '!' {
			continue
		}
		d, a, o := w[1], w[2], w[3]
		var found foundDAO
		switch {
		case a == ' ' && o == ' ' && isAlnum(d):
			found = foundDAO{dao: DAO{Datum: rune(toUpper(d)), Precision: DAONone}}
		case isUpper(d) && isDigit(a) && isDigit(o):
			extra := func(b byte) float64 { return float64(b-'0') * 0.001 }
			found = foundDAO{dao: DAO{Datum: rune(d), Precision: DAOThousandths}, lat: extra(a), lon: extra(o)}
		case isLower(d) && isBase91(a) && isBase91(o):
			extra := func(b byte) float64 { return float64(b-33) / 91 * 0.01 }
			found = foundDAO{dao: DAO{Datum: rune(toUpper(d)), Precision: DAOBase91}, lat: extra(a), lon: extra(o)}
		default:
			continue
		}
		return span{i, i + 5}, found, true
	}
	return span{}, foundDAO{}, false
}

func applyDAO(ctx *context, fields *Positioned, found foundDAO, offset int) bool {
	d := found.dao
	fields.DAO = &d
	if fields.Compressed {
		return true
	}
	if fields.Position.Ambiguity > 0 {
		// A !DAO! on an ambiguous position contradicts it whatever its precision, !W  ! too
		// (vectors README, Encoding); the position stays as it was.
		return ctx.tolerate(CodeDAOWithAmbiguity, "a !DAO! adds precision to an ambiguous position", offset)
	}
	if found.dao.Precision == DAONone {
		return true
	}
	p := &fields.Position
	if math.Signbit(p.Latitude) {
		p.Latitude += -found.lat / 60
	} else {
		p.Latitude += found.lat / 60
	}
	if math.Signbit(p.Longitude) {
		p.Longitude += -found.lon / 60
	} else {
		p.Longitude += found.lon / 60
	}
	return true
}

func equalFoldMHz(b []byte) bool {
	return len(b) == 3 && toUpper(b[0]) == 'M' && toUpper(b[1]) == 'H' && toUpper(b[2]) == 'Z'
}

// frequencyAt reads an APRS 1.2 voice frequency at the start of c: FFF.FFFMHz or FFF.FF MHz,
// then optional tone, offset and range fields, each after a space. It gives its length.
func frequencyAt(c []byte) (VoiceFrequency, int, bool) {
	if len(c) < 10 {
		return VoiceFrequency{}, 0, false
	}
	head := c[:10]
	// Above 999 MHz the first digit is a letter standing for the leading digits (APRS12c ch. 18).
	var whole uint32
	switch {
	case allDigits(head[0:3]):
		whole = digits(head[0:3])
	case allDigits(head[1:3]):
		prefix, ok := microwavePrefix(head[0])
		if !ok {
			return VoiceFrequency{}, 0, false
		}
		whole = prefix*100 + digits(head[1:3])
	default:
		return VoiceFrequency{}, 0, false
	}
	f := VoiceFrequency{}
	switch {
	case head[3] == '.' && allDigits(head[4:7]) && equalFoldMHz(head[7:10]):
		f.MHz = float64(whole) + float64(digits(head[4:7]))/1000
	case head[3] == '.' && allDigits(head[4:6]) && head[6] == ' ' && equalFoldMHz(head[7:10]):
		f.MHz = float64(whole) + float64(digits(head[4:6]))/100
		f.TenKHzResolution = true
	default:
		return VoiceFrequency{}, 0, false
	}
	at := 10
	// Each field is a space and four characters, then a space or the end of the text.
	field := func(at int) ([]byte, bool) {
		b, ok := get(c, at, at+5)
		if !ok || b[0] != ' ' || (at+5 < len(c) && c[at+5] != ' ') {
			return nil, false
		}
		return b[1:5], true
	}

	if b, ok := field(at); ok {
		var kind Tone
		var value *uint16
		found := true
		switch s := string(b); {
		case s == "Toff" || s == "toff":
			kind = ToneOff
		case s == "1750" || s == "l750":
			kind = ToneBurst
		case (b[0] == 'T' || b[0] == 't' || b[0] == 'C' || b[0] == 'c' || b[0] == 'D' || b[0] == 'd') && allDigits(b[1:]):
			switch toUpper(b[0]) {
			case 'T':
				kind = ToneTone
			case 'C':
				kind = ToneCTCSS
			default:
				kind = ToneDCS
			}
			value = Ptr(uint16(digits(b[1:])))
		default:
			found = false
		}
		if found {
			f.Tone = &kind
			f.ToneValue = value
			f.Narrow = isLower(b[0])
			at += 5
		}
	}
	if b, ok := field(at); ok {
		if (b[0] == '+' || b[0] == '-') && allDigits(b[1:4]) {
			v := int32(digits(b[1:4])) * 10
			if b[0] == '-' {
				v = -v
			}
			f.OffsetKHz = &v
			at += 5
		}
	}
	if b, ok := field(at); ok {
		if b[0] == 'R' && allDigits(b[1:3]) && (b[3] == 'm' || b[3] == 'k') {
			f.Range = Ptr(uint16(digits(b[1:3])))
			f.RangeKm = b[3] == 'k'
			at += 5
		}
	}
	return f, at, true
}

// microwavePrefixes are the letters that stand for the leading digits of frequencies above 999
// MHz (APRS12c ch. 18).
var microwavePrefixes = [...]struct {
	letter byte
	value  uint32
}{
	{'A', 12}, {'B', 23}, {'C', 24}, {'D', 34}, {'E', 56}, {'F', 57}, {'G', 58}, {'H', 101},
	{'I', 102}, {'J', 103}, {'K', 104}, {'L', 105}, {'M', 240}, {'N', 241}, {'O', 242},
}

func microwavePrefix(letter byte) (uint32, bool) {
	for _, p := range microwavePrefixes {
		if p.letter == letter {
			return p.value, true
		}
	}
	return 0, false
}
