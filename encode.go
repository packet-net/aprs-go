package aprs

import (
	"fmt"
	"math"
)

// Encoding data back into an information field. The encoder writes only what APRS12c allows, and
// checks free text by decoding what it wrote: a comment that would read back as something else
// (an altitude, a data extension, a !DAO!) is refused rather than silently changed.

// Encode gives the information field for data. It fails, with an *EncodeError naming the field,
// when the data cannot be written as APRS12c allows. Mic-E data is refused, since it carries half
// its position in the destination address: use NewPacket, which works that address out. data may
// also be a pointer to one of the data types.
func Encode(data Data) ([]byte, error) {
	out, err := encode(data)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func encode(data Data) ([]byte, *EncodeError) {
	data, err := prepare(data)
	if err != nil {
		return nil, err
	}
	if _, ok := data.(MicEReport); !ok {
		if err := validate(data); err != nil {
			return nil, err
		}
	}
	switch d := data.(type) {
	case PositionReport:
		return encodePositionReport(&d)
	case ObjectReport:
		return encodeObject(&d)
	case ItemReport:
		return encodeItem(&d)
	case MicEReport:
		return nil, newEncodeError(RefusalNotEncodable, "Mic-E data carries half its position in the destination address, which this information field leaves out: use NewPacket")
	case Message, Ack, Reject, Bulletin, NWSBulletin, TelemetryNames, TelemetryUnits, TelemetryCoefficients, TelemetryBits, DirectedQuery:
		return encodeMessage(data)
	}
	return encodeOther(data)
}

func encodePositionReport(r *PositionReport) ([]byte, *EncodeError) {
	p := &pending{}
	var out []byte
	switch {
	case r.Timestamp == nil && !r.Messaging:
		out = append(out, '!')
	case r.Timestamp == nil:
		out = append(out, '=')
	case !r.Messaging:
		out = append(out, '/')
	default:
		out = append(out, '@')
	}
	if r.Timestamp != nil {
		var err *EncodeError
		out, err = dhmOrHMS(out, *r.Timestamp)
		p.run(err)
	}
	return encodePositioned(out, &r.Fields, p)
}

func encodeObject(o *ObjectReport) ([]byte, *EncodeError) {
	name := []byte(o.Name)
	if len(name) == 0 || len(name) > 9 || !isPrintableASCII(name) {
		return nil, newEncodeError(RefusalInvalidName, "an object name is 1-9 printable ASCII characters (APRS12c ch. 11)")
	}
	p := &pending{}
	if name[len(name)-1] == ' ' {
		p.note(newEncodeError(RefusalPaddingSpace, "an object name cannot end in a space, which would be read back as padding"))
	}
	out := []byte{';'}
	out = append(out, name...)
	for i := len(name); i < 9; i++ {
		out = append(out, ' ')
	}
	if o.Killed {
		out = append(out, '_')
	} else {
		out = append(out, '*')
	}
	if o.Timestamp == nil {
		p.note(newEncodeError(RefusalMissingField, "an object report must have a timestamp (APRS12c ch. 11)"))
	} else {
		var err *EncodeError
		out, err = dhmOrHMS(out, *o.Timestamp)
		p.run(err)
	}
	return encodePositioned(out, &o.Fields, p)
}

func encodeItem(i *ItemReport) ([]byte, *EncodeError) {
	name := []byte(i.Name)
	if len(name) < 3 || len(name) > 9 || !isPrintableASCII(name) || contains(name, '!') || contains(name, '_') {
		return nil, newEncodeError(RefusalInvalidName, "an item name is 3-9 printable ASCII characters other than ! and _ (APRS12c ch. 11)")
	}
	out := []byte{')'}
	out = append(out, name...)
	if i.Killed {
		out = append(out, '_')
	} else {
		out = append(out, '!')
	}
	return encodePositioned(out, &i.Fields, &pending{})
}

func dhmOrHMS(out []byte, t Timestamp) ([]byte, *EncodeError) {
	if t.Kind == MonthDayHourMinute {
		return out, newEncodeError(RefusalInvalidTimestamp, "this timestamp is DDHHMMz, DDHHMM/ or HHMMSSh; MDHM is only for positionless weather")
	}
	if err := checkTimestamp(t); err != nil {
		return out, err
	}
	return append(out, t.bytes()...), nil
}

func checkTimestamp(t Timestamp) *EncodeError {
	if t.IsValid() || t.IsPermanentObjectMarker() {
		return nil
	}
	return newEncodeError(RefusalInvalidTimestamp, fmt.Sprintf("timestamp %s has a field out of range", t.String()))
}

// encodePositioned writes the position, symbol, extension, weather and comment of a position,
// object or item. p holds what is already wrong with the report around it.
func encodePositioned(out []byte, f *Positioned, p *pending) ([]byte, *EncodeError) {
	weatherSymbol := isWeatherSymbol(f)
	if weatherSymbol && f.Weather == nil {
		// A report with the weather station symbol reads back as weather, whatever it holds.
		p.note(newEncodeError(RefusalSymbolMismatch, "a report with the weather station symbol, _, is a weather report: give it weather (APRS12c ch. 12)"))
	}
	if f.Storm != nil && !isStormSymbol(f) {
		p.note(newEncodeError(RefusalSymbolMismatch, "storm data needs the hurricane symbol, @ (APRS12c ch. 12)"))
	}
	courseSpeed := f.CourseDegrees != nil || f.SpeedKnots != nil
	if f.Weather != nil {
		if !weatherSymbol {
			p.note(newEncodeError(RefusalSymbolMismatch, "weather needs the weather station symbol, _ (APRS12c ch. 12)"))
		}
		if f.Comment != "" {
			p.note(newEncodeError(RefusalFieldNotAllowed, "a weather report has no comment field (APRS12c ch. 12, UAP 2.7.1)"))
		}
		gga := f.Compression != nil && f.Compression.Source == SourceGGA
		if f.Telemetry != nil || f.Frequency != nil || f.Signpost != nil || (f.AltitudeFeet != nil && !(f.Compressed && gga)) {
			p.note(newEncodeError(RefusalFieldNotAllowed, "a weather report has no comment, so it cannot carry telemetry, frequency, signpost or /A= altitude (APRS12c ch. 12)"))
		}
		// Its data extension, or its cs bytes, are the wind.
		conflict := courseSpeed
		if !f.Compressed {
			conflict = courseSpeed || f.PHG != nil || f.DFS != nil || f.Area != nil || f.DFBearing != nil || f.Storm != nil || f.RangeMiles != nil
		}
		if conflict {
			p.note(newEncodeError(RefusalExtensionConflict, "a weather report's data extension is its wind, so it cannot carry a course, speed, PHG, RNG, DFS, area, DF bearing or storm data (APRS12c ch. 12)"))
		}
	}

	// The area symbol makes a course and speed read back as an area.
	if !f.Compressed && f.CourseDegrees != nil && f.Area == nil && f.Symbol.Table == '\\' && f.Symbol.Code == 'l' {
		p.note(newEncodeError(RefusalSymbolMismatch, "a course and speed on the area symbol, \\l, read back as an area"))
	}

	var daoMode *bool
	if f.DAO != nil && f.DAO.Precision != DAONone && !f.Compressed {
		daoMode = Ptr(f.DAO.Precision == DAOBase91)
	}
	start := len(out)
	var daoDigitsWritten *[2]byte
	altitudeInCS := false
	extension := false

	if f.Compressed {
		if f.PHG != nil || f.DFS != nil || f.Area != nil || f.DFBearing != nil || f.Storm != nil {
			p.note(newEncodeError(RefusalFieldNotAllowed, "a compressed position cannot carry a 7-byte data extension (PHG, DFS, area, DF bearing, storm) (APRS12c ch. 9)"))
		}
		out = encodeCompressed(out, f.Position, f.Symbol, p)
		// A compressed weather report has no comment, so an altitude the cs bytes cannot carry
		// exactly is rounded to their step and has no /A= (vectors README, Encoding).
		out, altitudeInCS = compressedCS(out, f, p)
	} else {
		// The 7-byte data extension holds one thing: course/speed (with a DF bearing or storm
		// data, which ride on it), PHG, RNG, DFS or an area object.
		count := 0
		for _, x := range []bool{
			f.CourseDegrees != nil || f.SpeedKnots != nil || f.DFBearing != nil || f.Storm != nil,
			f.PHG != nil, f.RangeMiles != nil, f.DFS != nil, f.Area != nil,
		} {
			if x {
				count++
			}
		}
		if count > 1 || (f.DFBearing != nil && f.Storm != nil) {
			p.note(newEncodeError(RefusalExtensionConflict, "only one data extension (course/speed, PHG, RNG, DFS or area object) fits in a report (APRS12c ch. 7)"))
		}
		out, daoDigitsWritten = encodeUncompressed(out, f.Position, f.Symbol, daoMode, p)
		out, extension = uncompressedExtension(out, f, p)
	}

	if f.Weather != nil {
		out = writeWeatherFields(out, f.Weather, p)
	}

	// After the data extension, in this order: the voice frequency and its fields, in the first
	// bytes of the comment where radios read it (APRS12c ch. 18); the signpost or corridor braces,
	// straight after the symbol or area extension as APRS12c's examples put them; the /A=
	// altitude; then the free text, base-91 telemetry and the !DAO! (APRS12c ch. 13).
	if f.Weather == nil {
		if f.Frequency != nil {
			// APRS12c ch. 18 writes a frequency after a data extension as $CSE/SPD/FFF.FFFMHz; a
			// PHGR (PHG72604/) already ends in its mandatory /.
			if extension && out[len(out)-1] != '/' {
				out = append(out, '/')
			}
			out = encodeFrequency(out, f.Frequency, p)
		}
		if f.Signpost != nil {
			sign := *f.Signpost
			if !(f.Symbol.Table == '\\' && f.Symbol.Code == 'm') {
				p.note(newEncodeError(RefusalSymbolMismatch, "a signpost needs the signpost symbol, \\m"))
			}
			if containsAny(sign, "{}") || !isPrintableASCII([]byte(sign)) {
				p.note(newEncodeError(RefusalInvalidCharacter, "signpost text is printable ASCII without braces"))
			}
			if len(sign) == 0 || len(sign) > 3 {
				p.note(newEncodeError(RefusalMalformed, "signpost text is 1-3 characters"))
			}
			out = append(out, '{')
			out = append(out, sign...)
			out = append(out, '}')
		}
		if f.Area != nil && f.Area.CorridorWidthMiles != nil {
			w := *f.Area.CorridorWidthMiles
			if w > 999 {
				p.note(newEncodeError(RefusalValueOutOfRange, "an area corridor width is at most 999 miles"))
			}
			out = append(out, fmt.Sprintf("{%d}", w)...)
		}
		if f.AltitudeFeet != nil && !altitudeInCS {
			feet := *f.AltitudeFeet
			n := roundSteps(feet)
			switch {
			case !inRange(n, -99999, 999999):
				p.note(newEncodeError(refusalForValue(feet), "/A= altitude must fit in 6 digits (or - and 5 digits)"))
			case n < 0:
				out = append(out, "/A=-"...)
				out = pushDigits(out, uint64(toU32(-n)), 5)
			default:
				out = append(out, "/A="...)
				out = pushDigits(out, uint64(toU32(n)), 6)
			}
		}
	}

	text := []byte(f.Comment)
	// A weather report has no comment: that is the fault, and the characters are not looked at.
	if f.Weather == nil && hasLineBreak(text) {
		p.note(newEncodeError(RefusalInvalidCharacter, "comment text cannot contain a line break"))
	}
	textAt := len(out)
	var trailer []byte
	if f.Telemetry != nil {
		trailer = telemetryInto(trailer, f.Telemetry, p)
	}
	if f.DAO != nil {
		d := *f.DAO
		// Whatever its precision: even !W  ! reads back as dao-with-ambiguity.
		if f.Position.Ambiguity > 0 {
			p.note(newEncodeError(RefusalFieldNotAllowed, "a !DAO! on an ambiguous position contradicts it, whatever its precision"))
		}
		p.run(checkDAODatum(d))
		trailer = append(trailer, '!')
		switch {
		case d.Precision == DAOThousandths && daoDigitsWritten != nil:
			trailer = append(trailer, byte(d.Datum), '0'+daoDigitsWritten[0], '0'+daoDigitsWritten[1])
		case d.Precision == DAOBase91 && daoDigitsWritten != nil:
			trailer = append(trailer, byte(runeToASCIILower(d.Datum)), daoDigitsWritten[0]+33, daoDigitsWritten[1]+33)
		case d.Precision == DAOBase91:
			// A compressed position does not use the digits, which agree with the position it
			// reports, the compressed one (vectors README, Encoding).
			lat, lon := daoDigits(compressedReported(f.Position), true)
			trailer = append(trailer, byte(runeToASCIILower(d.Datum)), lat+33, lon+33)
		case d.Precision == DAOThousandths:
			lat, lon := daoDigits(compressedReported(f.Position), false)
			trailer = append(trailer, byte(d.Datum), '0'+lat, '0'+lon)
		default:
			trailer = append(trailer, byte(d.Datum), ' ', ' ')
		}
		trailer = append(trailer, '!')
	}
	// What is wrong on its own, or between fields, is named before the read-back is tried.
	if err := p.finish(); err != nil {
		return nil, err
	}

	// Write the text as it is (after a frequency, a space first); if it does not read back
	// unchanged, try it without the space, then after a '/' delimiter.
	separators := []string{"", "/"}
	if f.Frequency != nil && len(text) > 0 {
		separators = []string{" ", "", " /"}
	}
	differs := ""
	for _, separator := range separators {
		out = out[:textAt]
		if f.Weather != nil && separator != "" {
			break
		}
		out = append(out, separator...)
		out = append(out, text...)
		out = append(out, trailer...)
		what, ok := positionedReadsBack(out[start:], f)
		if ok {
			return out, nil
		}
		differs = what
	}
	if f.Weather != nil {
		return nil, newEncodeError(RefusalTextReadsAsField, "the weather does not read back as given: a software type and unit, or an extra field, that would read as weather fields")
	}
	if len(text) == 0 {
		return nil, newEncodeError(RefusalDoesNotReadBack, fmt.Sprintf("the report does not read back as given: its %s differs", differs))
	}
	return nil, newEncodeError(RefusalTextReadsAsField, fmt.Sprintf("comment text contains something that decodes as a structured element (altitude, !DAO!, |telemetry|, frequency, PHG/RNG/DFS or braces), so the %s reads back differently; set the property instead", differs))
}

func containsAny(s, chars string) bool {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return true
			}
		}
	}
	return false
}

// compressedCS writes the compressed cs and type bytes; true when they carry the altitude
// exactly.
//
// The cs bytes carry one thing: a GGA altitude, a range, or a course and speed (the wind, for a
// weather station). Whatever else would have to go in them is refused, not dropped, and so is a
// compression type with nothing to carry, since blank cs bytes have no type byte (vectors
// interpretations.md, "Re-encoding into compressed bytes rounds").
func compressedCS(out []byte, f *Positioned, p *pending) ([]byte, bool) {
	var wind *Weather
	if f.Weather != nil && (f.Weather.WindDirectionDegrees != nil || f.Weather.WindSpeedMph != nil) {
		wind = f.Weather
	}
	courseSpeed := f.CourseDegrees != nil || f.SpeedKnots != nil
	t := CompressionType{Fix: FixCurrent, Source: SourceOther, Origin: OriginSoftware}
	if f.Compression != nil {
		t = *f.Compression
	}
	logBase := func(x, base float64) float64 { return math.Log(x) / math.Log(base) }
	altitude := false
	var c, s byte
	switch {
	case t.Source == SourceGGA:
		// GGA cs bytes are read as an altitude, whatever else was meant.
		if f.AltitudeFeet == nil {
			p.note(newEncodeError(RefusalMissingField, "a GGA compression type carries an altitude in the cs bytes, and there is none"))
		}
		if f.RangeMiles != nil {
			// A range that is itself wrong is named before it is found to clash.
			rangeStep(*f.RangeMiles, p)
		}
		if courseSpeed || f.RangeMiles != nil || wind != nil {
			p.note(newEncodeError(RefusalExtensionConflict, "the cs bytes carry the GGA altitude, so a compressed position cannot also carry a course, speed, range or wind"))
		}
		if f.AltitudeFeet != nil {
			feet := *f.AltitudeFeet
			if !isFinite(feet) {
				p.note(newEncodeError(RefusalNotANumber, "the altitude is not a number"))
			} else {
				// 1.002^cs feet cannot be 1 foot or less: the nearest is cs 0, and /A= carries the rest.
				var cs int64
				if feet > 1 {
					cs = toI64(roundSteps(logBase(feet, 1.002)))
				}
				if cs > 90*91+90 {
					p.note(newEncodeError(RefusalValueOutOfRange, "the altitude is too high for a compressed position"))
				} else {
					// The cs bytes carry altitude only to 0.2%; one they cannot carry exactly is
					// also written as /A=, which the decoder prefers.
					altitude = math.Abs(math.Pow(1.002, float64(cs))-feet) <= 1e-9*math.Abs(feet)
					c, s = byte(cs/91), byte(cs%91)
				}
			}
		}
	case wind != nil:
		// A weather station's cs course and speed are its wind.
		if f.RangeMiles != nil {
			rangeStep(*f.RangeMiles, p)
			p.note(newEncodeError(RefusalExtensionConflict, "the cs bytes carry the wind or a range, not both"))
		}
		// The cs bytes carry a direction and a speed together; one without the other would read
		// back as 0.
		if wind.WindDirectionDegrees == nil || wind.WindSpeedMph == nil {
			p.note(newEncodeError(RefusalMissingField, "the cs bytes carry the wind as a direction and a speed together, so both are needed"))
		}
		if wind.WindDirectionDegrees != nil {
			if v, err := directionCode(*wind.WindDirectionDegrees, "wind direction"); err != nil {
				p.note(err)
			} else {
				c = v
			}
		}
		if wind.WindSpeedMph != nil {
			if v, err := speedCode(*wind.WindSpeedMph / knotsToMph); err != nil {
				p.note(err)
			} else {
				s = v
			}
		}
	case f.RangeMiles != nil:
		if courseSpeed {
			p.note(newEncodeError(RefusalExtensionConflict, "the cs bytes carry a course and speed or a range, not both (APRS12c ch. 9)"))
		}
		// The steps are 2 x 1.08^s miles for every whole s, and a range rounds to the nearest; one
		// whose s is below the byte's 0 is refused (1.95 miles is s 0, 1.9 is s -1).
		if f.CourseDegrees != nil {
			// A course of 0 has no form, whatever else is wrong.
			if whole, err := wholeDegrees(*f.CourseDegrees, "course"); err != nil {
				p.note(err)
			} else if whole == 0 {
				p.note(newEncodeError(RefusalValueHasNoForm, "a compressed course of 0 would read back as 360, and the cs bytes have no unknown course"))
			}
		}
		if step, ok := rangeStep(*f.RangeMiles, p); ok {
			c, s = '{'-33, step
		}
	case courseSpeed:
		// The cs bytes hold a course and a speed together, so writing one alone would add a value
		// the data does not have; and they have no unknown course, since a c of 0 reads back as
		// 360 (vectors README, Encoding).
		if f.CourseDegrees == nil || f.SpeedKnots == nil {
			p.note(newEncodeError(RefusalMissingField, "the cs bytes carry a course and a speed together, so a compressed report needs both"))
		}
		if f.CourseDegrees != nil {
			// Unknown (0) and in range (1-360) are decided in whole degrees; a known course is
			// then rounded once to the 4-degree step, where the step at north is 360.
			course := *f.CourseDegrees
			if whole, err := wholeDegrees(course, "course"); err != nil {
				p.note(err)
			} else if whole == 0 {
				p.note(newEncodeError(RefusalValueHasNoForm, "a compressed course of 0 would read back as 360, and the cs bytes have no unknown course"))
			} else if v, err := directionCode(course, "course"); err != nil {
				p.note(err)
			} else {
				c = v
			}
		}
		if f.SpeedKnots != nil {
			if v, err := speedCode(*f.SpeedKnots); err != nil {
				p.note(err)
			} else {
				s = v
			}
		}
	default:
		// No course/speed, range or altitude: the spec's own form for that, which has no type byte.
		if f.Compression != nil {
			p.note(newEncodeError(RefusalMissingField, "a compression type needs something in the cs bytes: a course and speed, range, altitude or wind"))
		}
		return append(out, " sT"...), false
	}
	return append(out, c+33, s+33, typeToBits(t)+33), altitude
}

// rangeStep gives a compressed range's s byte: the steps are 2 x 1.08^s miles for every whole s,
// and a range rounds to the nearest; one whose s is below the byte's 0 is refused (1.95 miles is
// s 0, 1.9 is s -1).
func rangeStep(rng float64, p *pending) (byte, bool) {
	switch {
	case !isFinite(rng):
		p.note(newEncodeError(RefusalNotANumber, "a compressed range is a number"))
	case rng <= 0:
		p.note(newEncodeError(RefusalValueOutOfRange, "a compressed range must be at least 2 miles"))
	default:
		step := roundSteps(math.Log(rng/2) / math.Log(1.08))
		switch {
		case step < 0:
			p.note(newEncodeError(RefusalValueOutOfRange, "a compressed range must round to at least 2 miles"))
		case step > 90:
			p.note(newEncodeError(RefusalValueOutOfRange, "the range is too far for a compressed position"))
		default:
			return toU8(step), true
		}
	}
	return 0, false
}

// wholeDegrees rounds a course or wind direction once to whole degrees, which must be 0-360. It
// gives 0 with the error.
func wholeDegrees(degrees float64, what string) (uint32, *EncodeError) {
	rounded := roundSteps(degrees)
	if !inRange(rounded, 0, 360) {
		return 0, newEncodeError(refusalForValue(degrees), what+" must round to 0-360 degrees")
	}
	return toU32(rounded), nil
}

// directionCode gives a course or wind direction in the compressed c byte's 4-degree steps: the
// nearest step, rounded once from the value given (103 degrees is written as 104), with 360 and 0
// both north. The step must be 0-360 degrees.
func directionCode(degrees float64, what string) (byte, *EncodeError) {
	steps := roundSteps(degrees / 4)
	if !inRange(steps, 0, 90) {
		return 0, newEncodeError(refusalForValue(degrees), what+" must round to 0-360 degrees")
	}
	return byte(toU32(steps) % 90), nil
}

func speedCode(knots float64) (byte, *EncodeError) {
	// The steps are 1.08^s - 1 knots for every whole s, and a value rounds to the nearest; one
	// whose s is below the byte's 0 is refused (-0.03 knots is s 0, -0.3 is s -5).
	if !isFinite(knots) {
		return 0, newEncodeError(RefusalNotANumber, "speed is a number")
	}
	if knots <= -1 {
		return 0, newEncodeError(RefusalValueOutOfRange, "speed cannot be negative")
	}
	s := roundSteps(math.Log(knots+1) / math.Log(1.08))
	if s < 0 {
		return 0, newEncodeError(RefusalValueOutOfRange, "speed cannot be negative")
	}
	if s > 90 {
		return 0, newEncodeError(RefusalValueOutOfRange, "the speed is too high for a compressed position")
	}
	return toU8(s), nil
}

func three(out []byte, v *uint32, max uint32, what string) ([]byte, *EncodeError) {
	switch {
	case v == nil:
		return append(out, "..."...), nil
	case *v <= max:
		return pushDigits(out, uint64(*v), 3), nil
	}
	return out, newEncodeError(RefusalValueOutOfRange, what+" does not fit in its three digits")
}

// uncompressedExtension writes the 7-byte data extension of an uncompressed position; true if
// there is one.
func uncompressedExtension(out []byte, f *Positioned, p *pending) ([]byte, bool) {
	var err *EncodeError
	if w := f.Weather; w != nil {
		var direction *uint32
		if w.WindDirectionDegrees != nil {
			if d, e := wholeDegrees(*w.WindDirectionDegrees, "wind direction"); p.run(e) {
				direction = &d
			}
		}
		out, err = three(out, direction, 360, "wind direction")
		p.run(err)
		out = append(out, '/')
		speed, e := whole(w.WindSpeedMph, "wind speed")
		p.run(e)
		out, err = three(out, speed, 999, "wind speed")
		p.run(err)
		return out, true
	}
	if f.CourseDegrees != nil || f.SpeedKnots != nil || f.DFBearing != nil || f.Storm != nil {
		var course *uint32
		if f.CourseDegrees != nil {
			if c, e := wholeDegrees(*f.CourseDegrees, "course"); p.run(e) {
				course = &c
			}
		}
		out, err = three(out, course, 360, "course")
		p.run(err)
		out = append(out, '/')
		speed, e := whole(f.SpeedKnots, "speed")
		p.run(e)
		out, err = three(out, speed, 999, "speed")
		p.run(err)
		if b := f.DFBearing; b != nil {
			if !(f.Symbol.Table == '/' && f.Symbol.Code == '\\') {
				p.note(newEncodeError(RefusalSymbolMismatch, "a DF bearing needs the DF symbol, /\\"))
			}
			if b.BearingDegrees > 360 || b.Number > 9 || b.Range > 9 || b.Quality > 9 {
				p.note(newEncodeError(RefusalValueOutOfRange, "DF bearing is 0-360 and N, R, Q are single digits"))
			} else {
				out = append(out, '/')
				out = pushDigits(out, uint64(b.BearingDegrees), 3)
				out = append(out, '/', '0'+b.Number, '0'+b.Range, '0'+b.Quality)
			}
		}
		if s := f.Storm; s != nil {
			out = append(out, '/')
			switch s.Kind {
			case TropicalStorm:
				out = append(out, "TS"...)
			case Hurricane:
				out = append(out, "HC"...)
			default:
				out = append(out, "TD"...)
			}
			for _, x := range []struct {
				marker byte
				value  *uint16
				width  int
			}{
				{'/', s.SustainedWindKnots, 3}, {'^', s.GustKnots, 3}, {'/', s.CentralPressureMbar, 4},
				{'>', s.HurricaneRadiusNM, 3}, {'&', s.TropicalStormRadiusNM, 3},
			} {
				out = append(out, x.marker)
				out, err = stormValue(out, x.value, x.width)
				p.run(err)
			}
			if s.WholeGaleRadiusNM != nil {
				out = append(out, '%')
				out, err = stormValue(out, s.WholeGaleRadiusNM, 3)
				p.run(err)
			}
		}
		return out, true
	}
	if ph := f.PHG; ph != nil {
		out, err = phgCodes(out, *ph)
		p.run(err)
		if ph.BeaconsPerHour != nil {
			if c, ok := beaconRateChar(*ph.BeaconsPerHour); ok {
				out = append(out, c, '/')
			} else {
				p.note(newEncodeError(RefusalValueOutOfRange, "PHGR beacons per hour is 1-35"))
			}
		}
		return out, true
	}
	if f.RangeMiles != nil {
		r := *f.RangeMiles
		n := roundSteps(r)
		if !inRange(n, 0, 9999) {
			p.note(newEncodeError(refusalForValue(r), "RNG range is 0-9999 miles"))
		} else {
			out = append(out, "RNG"...)
			out = pushDigits(out, uint64(toU32(n)), 4)
		}
		return out, true
	}
	if f.DFS != nil {
		out, err = dfsCodes(out, *f.DFS)
		p.run(err)
		return out, true
	}
	if a := f.Area; a != nil {
		if !(f.Symbol.Table == '\\' && f.Symbol.Code == 'l') {
			p.note(newEncodeError(RefusalSymbolMismatch, "an area object needs the area symbol, \\l"))
		}
		if a.LatOffset > 99 || a.LonOffset > 99 {
			p.note(newEncodeError(RefusalValueOutOfRange, "area offsets are two digits"))
			return out, true
		}
		out = append(out, '0'+byte(a.Shape))
		out = pushDigits(out, uint64(a.LatOffset), 2)
		color := uint64(a.Color)
		if color > 15 {
			color = 0
		}
		if color < 10 {
			out = append(out, '/', '0'+byte(color))
		} else {
			out = pushDigits(out, color, 2)
		}
		out = pushDigits(out, uint64(a.LonOffset), 2)
		// A line's corridor width, {www}, is written with the comment's braces.
		return out, true
	}
	return out, false
}

// maxHeightCode is the highest PHG or DFS height code: ~, the last printable character after 0
// (APRS12c ch. 7: "any ASCII character 0-9 and above").
const maxHeightCode = '~' - '0'

// phgCodes writes PHGphgd (without the PHGR rate): the power, gain and directivity codes are
// digits, and the height code runs on past 9 through the ASCII table (: is 10), for balloons and
// aircraft.
func phgCodes(out []byte, p PHG) ([]byte, *EncodeError) {
	if p.Power > 9 || p.Gain > 9 || p.Directivity > 9 || p.Height > maxHeightCode {
		return out, newEncodeError(RefusalValueOutOfRange, "PHG power, gain and directivity codes are single digits, and the height code 0-78")
	}
	return append(out, 'P', 'H', 'G', '0'+p.Power, '0'+p.Height, '0'+p.Gain, '0'+p.Directivity), nil
}

// dfsCodes writes DFSshgd: the height code as for PHG, the others digits.
func dfsCodes(out []byte, d DFS) ([]byte, *EncodeError) {
	if d.Strength > 9 || d.Gain > 9 || d.Directivity > 9 || d.Height > maxHeightCode {
		return out, newEncodeError(RefusalValueOutOfRange, "DFS strength, gain and directivity codes are single digits, and the height code 0-78")
	}
	return append(out, 'D', 'F', 'S', '0'+d.Strength, '0'+d.Height, '0'+d.Gain, '0'+d.Directivity), nil
}

// checkDAODatum checks a !DAO! datum the decoder reads back: an upper-case letter (W for WGS84),
// or a digit (a local datum), which has no case to say how to read added digits and so carries
// none.
func checkDAODatum(d DAO) *EncodeError {
	if runeIsASCIIUpper(d.Datum) || (runeIsASCIIDigit(d.Datum) && d.Precision == DAONone) {
		return nil
	}
	return newEncodeError(RefusalInvalidCode, "the !DAO! datum is an upper-case letter (W for WGS84), or a digit with no added precision")
}

func whole(v *float64, what string) (*uint32, *EncodeError) {
	if v == nil {
		return nil, nil
	}
	// Rounded first, so -0.3 is 0.
	if r := roundSteps(*v); r >= 0 {
		return Ptr(toU32(r)), nil
	}
	return nil, newEncodeError(RefusalValueOutOfRange, what+" cannot be negative")
}

func stormValue(out []byte, v *uint16, width int) ([]byte, *EncodeError) {
	if v == nil {
		for i := 0; i < width; i++ {
			out = append(out, '.')
		}
		return out, nil
	}
	limit := uint32(1)
	for i := 0; i < width; i++ {
		limit *= 10
	}
	if uint32(*v) < limit {
		return pushDigits(out, uint64(*v), width), nil
	}
	return out, newEncodeError(RefusalValueOutOfRange, "a storm value does not fit its field")
}

func encodeFrequency(out []byte, f *VoiceFrequency, p *pending) []byte {
	if !(f.MHz >= 0 && f.MHz < 24300) {
		p.note(newEncodeError(refusalForValue(f.MHz), "the frequency must be below 24300 MHz"))
		return out
	}
	perMHz, decimals := uint64(1000), 3
	if f.TenKHzResolution {
		perMHz, decimals = 100, 2
	}
	units := toU64(roundSteps(f.MHz * float64(perMHz)))
	whole, fraction := units/perMHz, units%perMHz
	var head string
	if whole < 1000 {
		head = fmt.Sprintf("%03d", whole)
	} else {
		found := false
		for _, m := range microwavePrefixes {
			if uint64(m.value) == whole/100 {
				head = fmt.Sprintf("%c%02d", m.letter, whole%100)
				found = true
				break
			}
		}
		if !found {
			p.note(newEncodeError(RefusalValueHasNoForm, "APRS12c ch. 18 has no letter for this frequency above 999 MHz"))
			return out
		}
	}
	out = append(out, head...)
	out = append(out, '.')
	out = append(out, fmt.Sprintf("%0*d", decimals, fraction)...)
	if f.TenKHzResolution {
		out = append(out, " MHz"...)
	} else {
		out = append(out, "MHz"...)
	}
	if f.Tone != nil {
		out = append(out, ' ')
		var letter byte
		switch *f.Tone {
		case ToneOff:
			if f.Narrow {
				out = append(out, "toff"...)
			} else {
				out = append(out, "Toff"...)
			}
		case ToneBurst:
			if f.Narrow {
				out = append(out, "l750"...)
			} else {
				out = append(out, "1750"...)
			}
		case ToneTone:
			letter = 'T'
		case ToneCTCSS:
			letter = 'C'
		default:
			letter = 'D'
		}
		if letter != 0 {
			switch {
			case f.ToneValue != nil && *f.ToneValue <= 999:
				if f.Narrow {
					letter = toLower(letter)
				}
				out = append(out, letter)
				out = pushDigits(out, uint64(*f.ToneValue), 3)
			case f.ToneValue != nil:
				p.note(newEncodeError(RefusalValueOutOfRange, "a tone's value is up to three digits"))
			default:
				p.note(newEncodeError(RefusalMissingField, "a tone needs a value of up to three digits"))
			}
		}
	} else if f.Narrow {
		p.note(newEncodeError(RefusalMissingField, "narrow band is written with the tone field, so it needs a tone"))
	}
	if f.OffsetKHz != nil {
		offset := int64(*f.OffsetKHz)
		abs := offset
		if abs < 0 {
			abs = -abs
		}
		switch {
		case abs > 9990:
			p.note(newEncodeError(RefusalValueOutOfRange, "the offset is up to 9.99 MHz"))
		case offset%10 != 0:
			p.note(newEncodeError(RefusalValueHasNoForm, "the offset is in 10 kHz steps"))
		default:
			out = append(out, ' ')
			if offset < 0 {
				out = append(out, '-')
			} else {
				out = append(out, '+')
			}
			out = pushDigits(out, uint64(abs/10), 3)
		}
	}
	if f.Range != nil {
		if *f.Range > 99 {
			p.note(newEncodeError(RefusalValueOutOfRange, "the frequency range is two digits"))
		} else {
			out = append(out, ' ', 'R')
			out = pushDigits(out, uint64(*f.Range), 2)
			if f.RangeKm {
				out = append(out, 'k')
			} else {
				out = append(out, 'm')
			}
		}
	} else if f.RangeKm {
		p.note(newEncodeError(RefusalMissingField, "range_km needs a range"))
	}
	return out
}

func telemetryInto(out []byte, t *CommentTelemetry, p *pending) []byte {
	const max = 91*91 - 1
	over := t.Sequence > max
	for _, v := range t.Analog {
		over = over || v > max
	}
	if over {
		p.note(newEncodeError(RefusalValueOutOfRange, "comment telemetry values are 0-8280"))
	}
	if len(t.Analog) == 0 || len(t.Analog) > 5 || (t.Digital != nil && len(t.Analog) != 5) {
		p.note(newEncodeError(RefusalMalformed, "comment telemetry has 1-5 analog values, and the digital bits only after all five"))
	}
	out = append(out, '|')
	out = base91Encode(out, uint32(min(t.Sequence, max)), 2)
	for _, v := range t.Analog {
		out = base91Encode(out, uint32(min(v, max)), 2)
	}
	if t.Digital != nil {
		out = base91Encode(out, uint32(*t.Digital), 2)
	}
	return append(out, '|')
}

// positionedReadsBack says whether what was written for f (from the position on) decodes back
// to the same comment and structured elements; if not, which one differs.
func positionedReadsBack(written []byte, f *Positioned) (string, bool) {
	ctx := newContext(Lenient)
	decoded, ok := decodePosition(ctx, written, 0)
	if !ok {
		return "position", false
	}
	back := Positioned{Position: decoded.position, Symbol: decoded.symbol, Compressed: decoded.compressed}
	if !decodeComment(ctx, &back, decoded.cs, written[decoded.length:], decoded.length) || ctx.hasErrors() {
		return "comment", false
	}
	close := func(a, b *float64, tolerance float64) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		return math.Abs(*a-*b) <= tolerance*fmax(math.Abs(*a), 1)
	}
	// A /A= or RNG value is written rounded to its whole step.
	rounded := func(a, b *float64) bool {
		return a != nil && b != nil && *a == roundSteps(*b)
	}
	datum := func(d *DAO) *rune {
		if d == nil {
			return nil
		}
		return &d.Datum
	}
	checks := []struct {
		same bool
		what string
	}{
		{back.Comment == f.Comment, "comment"},
		{samePHG(back.PHG, f.PHG), "PHG"},
		{samePtr(back.DFS, f.DFS), "DFS"},
		{sameArea(back.Area, f.Area), "area"},
		{samePtr(back.DFBearing, f.DFBearing), "DF bearing"},
		{sameStorm(back.Storm, f.Storm), "storm data"},
		{sameCommentTelemetry(back.Telemetry, f.Telemetry), "telemetry"},
		{sameFrequencyFields(back.Frequency, f.Frequency), "frequency"},
		{samePtr(back.Signpost, f.Signpost), "signpost"},
		{samePtr(datum(back.DAO), datum(f.DAO)), "DAO"},
		{sameWeatherText(back.Weather, f.Weather), "weather"},
		{close(back.AltitudeFeet, f.AltitudeFeet, 0.01) || rounded(back.AltitudeFeet, f.AltitudeFeet), "altitude"},
		{close(back.RangeMiles, f.RangeMiles, 0.1) || rounded(back.RangeMiles, f.RangeMiles), "range"},
	}
	for _, c := range checks {
		if !c.same {
			return c.what, false
		}
	}
	return "", true
}

func samePHG(a, b *PHG) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Power == b.Power && a.Height == b.Height && a.Gain == b.Gain && a.Directivity == b.Directivity && samePtr(a.BeaconsPerHour, b.BeaconsPerHour)
}

func sameArea(a, b *AreaObject) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Shape == b.Shape && a.LatOffset == b.LatOffset && a.Color == b.Color && a.LonOffset == b.LonOffset && samePtr(a.CorridorWidthMiles, b.CorridorWidthMiles)
}

func sameStorm(a, b *Storm) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Kind == b.Kind && samePtr(a.SustainedWindKnots, b.SustainedWindKnots) && samePtr(a.GustKnots, b.GustKnots) &&
		samePtr(a.CentralPressureMbar, b.CentralPressureMbar) && samePtr(a.HurricaneRadiusNM, b.HurricaneRadiusNM) &&
		samePtr(a.TropicalStormRadiusNM, b.TropicalStormRadiusNM) && samePtr(a.WholeGaleRadiusNM, b.WholeGaleRadiusNM)
}

func sameCommentTelemetry(a, b *CommentTelemetry) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Sequence != b.Sequence || len(a.Analog) != len(b.Analog) || !samePtr(a.Digital, b.Digital) {
		return false
	}
	for i := range a.Analog {
		if a.Analog[i] != b.Analog[i] {
			return false
		}
	}
	return true
}

// sameFrequencyFields compares the parts of a voice frequency a comment can change: its tone,
// offset and range fields.
func sameFrequencyFields(a, b *VoiceFrequency) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return samePtr(a.Tone, b.Tone) && samePtr(a.ToneValue, b.ToneValue) && samePtr(a.OffsetKHz, b.OffsetKHz) &&
		samePtr(a.Range, b.Range) && a.RangeKm == b.RangeKm && a.Narrow == b.Narrow
}
