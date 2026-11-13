package aprs

import (
	"bytes"
	"fmt"
)

// Mic-E (APRS12c ch. 10): latitude, message code and digipeat path in the destination address;
// longitude, speed, course and symbol in the information field; then a status text that can carry
// a device type code, altitude, grid locator and device suffix.

// micEBit is a destination character's flag bit.
type micEBit uint8

const (
	bitZero micEBit = iota
	bitStandard
	bitCustom
)

type micEChar struct {
	digit    int // -1 for a blank (ambiguity)
	bit      micEBit
	hasDigit bool
}

func classifyMicE(c byte) (micEChar, bool) {
	switch {
	case isDigit(c):
		return micEChar{int(c - '0'), bitZero, true}, true
	case c >= 'A' && c <= 'J':
		return micEChar{int(c - 'A'), bitCustom, true}, true
	case c == 'K':
		return micEChar{-1, bitCustom, false}, true
	case c == 'L':
		return micEChar{-1, bitZero, false}, true
	case c >= 'P' && c <= 'Y':
		return micEChar{int(c - 'P'), bitStandard, true}, true
	case c == 'Z':
		return micEChar{-1, bitStandard, false}, true
	}
	return micEChar{}, false
}

var micEMessages = [8]MicEMessage{
	MicEEmergency, MicEPriority, MicESpecial, MicECommitted, MicEReturning, MicEInService, MicEEnRoute, MicEOffDuty,
}

func decodeMicE(ctx *context, destination Address, info []byte) Data {
	oldData := info[0] == '\'' || info[0] == 0x1D
	dest := destination.Callsign()
	chars := make([]micEChar, 0, len(dest))
	valid := len(dest) == 6
	for i := 0; i < len(dest); i++ {
		ch, ok := classifyMicE(dest[i])
		if !ok {
			valid = false
			break
		}
		chars = append(chars, ch)
	}
	if valid {
		for _, ch := range chars[3:] {
			if ch.bit == bitCustom {
				valid = false
			}
		}
	}
	if !valid {
		ctx.error(CodeInvalidMicEDestination, "the destination is not a Mic-E latitude and message code (APRS12c ch. 10)", none)
		return nil
	}

	// Ambiguity: latitude digits blanked from the right.
	ambiguity := 0
	for _, i := range []int{5, 4, 3, 2} {
		if chars[i].hasDigit {
			break
		}
		ambiguity++
	}
	for _, ch := range chars[:6-ambiguity] {
		if !ch.hasDigit {
			ctx.error(CodeInvalidMicEDestination, "the Mic-E latitude has a blank that is not trailing", none)
			return nil
		}
	}
	if !chars[0].hasDigit || !chars[1].hasDigit {
		ctx.error(CodeInvalidMicEDestination, "the Mic-E latitude has a blank that is not trailing", none)
		return nil
	}
	digit := func(i int) float64 {
		if !chars[i].hasDigit {
			return 0
		}
		return float64(chars[i].digit)
	}
	latDeg := digit(0)*10 + digit(1)
	latMin := digit(2)*10 + digit(3) + digit(4)/10 + digit(5)/100
	halfBox := [...]float64{0, 0.05, 0.5, 5, 30}[ambiguity]
	if latMin >= 60 || latDeg+(latMin+halfBox)/60 > 90 {
		ctx.error(CodeInvalidMicEDestination, "the Mic-E latitude is out of range", none)
		return nil
	}
	north := chars[3].bit == bitStandard
	lonOffset := chars[4].bit == bitStandard
	west := chars[5].bit == bitStandard

	custom, standard := false, false
	n := 0
	for _, ch := range chars[:3] {
		custom = custom || ch.bit == bitCustom
		standard = standard || ch.bit == bitStandard
		n *= 2
		if ch.bit != bitZero {
			n++
		}
	}
	var message MicEMessage
	switch {
	case custom && standard:
		message = MicEUnknown
	case micEMessages[n] == MicEEmergency:
		message = MicEEmergency
	case custom:
		message = MicECustom(uint8(7 - n))
	default:
		message = micEMessages[n]
	}

	if len(info) < 9 {
		ctx.error(CodeInvalidMicEInformation, "a Mic-E information field is at least 9 bytes (APRS12c ch. 10)", 0)
		return nil
	}
	d, m, h := info[1], info[2], info[3]
	inR := func(b byte, lo, hi byte) bool { return b >= lo && b <= hi }
	if !inR(d, 38, 127) || !inR(m, 38, 97) || !inR(h, 28, 127) || !inR(info[4], 28, 127) || !inR(info[5], 28, 127) || !inR(info[6], 28, 127) {
		ctx.error(CodeInvalidMicEInformation, "a Mic-E longitude, speed or course byte is out of range (APRS12c ch. 10)", 1)
		return nil
	}
	lonDeg := uint32(d - 28)
	if lonOffset {
		lonDeg += 100
	}
	if lonDeg >= 180 && lonDeg <= 189 {
		lonDeg -= 80
	} else if lonDeg >= 190 && lonDeg <= 199 {
		lonDeg -= 190
	}
	lonMin := uint32(m - 28)
	if lonMin >= 60 {
		lonMin -= 60
	}
	lonHundredths := uint32(h - 28)
	if lonDeg > 180 || lonHundredths > 99 {
		ctx.error(CodeInvalidMicEInformation, "the Mic-E longitude is out of range", 1)
		return nil
	}
	// The latitude's ambiguity applies to the longitude (interpretations.md): blanked digits, then
	// the centre.
	lonUnits := lonMin*100 + lonHundredths
	switch ambiguity {
	case 1:
		lonUnits = lonUnits / 10 * 10
	case 2:
		lonUnits = lonUnits / 100 * 100
	case 3:
		lonUnits = lonUnits / 1000 * 1000
	case 4:
		lonUnits = 0
	}
	latitude := latDeg + (latMin+halfBox)/60
	longitude := float64(lonDeg) + (float64(lonUnits)/100+halfBox)/60

	sp, dc, se := uint32(info[4]-28), uint32(info[5]-28), uint32(info[6]-28)
	speed := sp*10 + dc/10
	if speed >= 800 {
		speed -= 800
	}
	course := (dc%10)*100 + se
	if course >= 400 {
		course -= 400
	}
	var courseDegrees *float64
	switch {
	case course == 0:
	case course <= 360:
		courseDegrees = Ptr(float64(course))
	default:
		if !ctx.tolerate(CodeOutOfRangeValue, "a Mic-E course over 360 degrees was dropped", 5) {
			return nil
		}
	}

	symbol, ok := decodeSymbol(ctx, info[8], info[7], 8, 7, false)
	if !ok {
		return nil
	}
	if !north {
		latitude = -latitude
	}
	if west {
		longitude = -longitude
	}
	report := MicEReport{
		MicEMessage:     message,
		OldData:         oldData,
		DestinationSSID: destination.SSID(),
		Fields: Positioned{
			Position:      Position{Latitude: latitude, Longitude: longitude, Ambiguity: uint8(ambiguity)},
			Symbol:        symbol,
			CourseDegrees: courseDegrees,
			SpeedKnots:    Ptr(float64(speed)),
		},
	}
	if !micEStatus(ctx, &report, info[9:], 9) {
		return nil
	}
	return report
}

func isTypeCode(b byte) bool {
	return b == '`' || b == '\'' || b == '>' || b == ']' || b == ' '
}

// micEStatus reads the status text: type code, altitude, grid locator, a data extension, the
// comment with what can be lifted out of it, and the device suffix.
func micEStatus(ctx *context, report *MicEReport, b []byte, offset int) bool {
	s := append([]byte(nil), b...)
	if contains(s, 0xFF) {
		if !ctx.tolerate(CodeKenwoodFFPadding, "Kenwood 0xFF padding in the status text was removed (UAP 5.10)", offset) {
			return false
		}
		kept := s[:0]
		for _, c := range s {
			if c != 0xFF {
				kept = append(kept, c)
			}
		}
		s = kept
	}

	// Rev 0 binary telemetry, looked for once the 0xFF padding is gone (vectors interpretations.md).
	if len(s) >= 6 && s[0] == 0x1D {
		ctx.info(CodeObsoleteFormat, "obsolete Mic-E binary telemetry (APRS12c ch. 10)", offset)
		report.LegacyTelemetry = append([]byte(nil), s[1:6]...)
		s = drain(s, 0, 6)
	}

	var typeCode *byte
	if len(s) > 0 && isTypeCode(s[0]) {
		t := s[0]
		typeCode = &t
		s = s[1:]
		report.TypeCode = Ptr(rune(t))
	} else {
		if len(s) > 0 {
			ctx.info(CodeMicEMissingDeviceType, "a Mic-E report without a device type prefix (UAP 5.4)", offset)
		}
		report.TypeCode = nil
	}

	suffix := micESuffixLen(typeCode, s)
	report.DeviceSuffix = lossy(s[len(s)-suffix:])
	s = s[:len(s)-suffix]

	// Altitude: xxx} first. One later in the text (a tolerated reading) is looked for once a
	// locator and a data extension at the start have taken their bytes.
	altitudeFirst, firstOK := micEAltitude(s)
	if firstOK {
		report.Fields.AltitudeFeet = Ptr(altitudeFirst)
		s = drain(s, 0, 4)
	}

	// A grid locator and the /G symbol, then a space before any text.
	if n, ok := locatorLen(s); ok {
		report.Locator = Ptr(asciiUpper(lossy(s[:n])))
		s = drain(s, 0, n+2)
		// Base-91 telemetry and a !DAO! are lifted out before the locator is read, so a locator
		// followed straight by them alone has nothing after it (vectors interpretations.md,
		// "Where a Mic-E locator is read").
		if len(s) > 0 && len(withoutTelemetryAndDAO(s)) > 0 {
			if s[0] == ' ' {
				s = drain(s, 0, 1)
			} else if !ctx.tolerate(CodeMissingSpaceAfterLocator, "text straight after the grid locator; a space comes first (APRS12c ch. 10)", offset) {
				return false
			}
		}
	}

	// A data extension at the start of the text, lifted before an altitude is looked for later on,
	// so its bytes are never read as one (0PH} in PHG3330PH}); one later on is found by the
	// comment's rules.
	if n, ok := micEExtension(&report.Fields, s); ok {
		s = drain(s, 0, n)
	}

	// An altitude later in the text, when none came first: a tolerated reading. Taking it out
	// brings the bytes either side of it together, and a !DAO! across that join is not one.
	joined := -1
	if !firstOK {
		for i := 0; i+3 < len(s); i++ {
			if _, ok := micEAltitude(s[i:]); !ok {
				continue
			}
			if ctx.allows(CodeMicEAltitudeNotFirst, "a Mic-E altitude after other status text instead of first (APRS12c ch. 10)", offset+i) {
				feet, _ := micEAltitude(s[i:])
				report.Fields.AltitudeFeet = Ptr(feet)
				s = drain(s, i, i+4)
				joined = i
			} else if joinsIntoAltitude(s) {
				// Strictly the altitude is text and telemetry and the !DAO! are lifted as usual,
				// unless what is left holds an altitude across a place one was taken from, which a
				// lenient reader would then take: that reading is rejected (vectors README, Mic-E).
				ctx.error(CodeMicEAltitudeNotFirst, "lifting telemetry and the !DAO! would join the status text into a Mic-E altitude (APRS12c ch. 10)", offset+i)
				return false
			}
			break
		}
	}
	altitudeBefore := report.Fields.AltitudeFeet
	// A /A= in the text wins over the Mic-E bytes, even when it says the same: whether the text
	// gave one is seen by letting it write into an empty field.
	report.Fields.AltitudeFeet = nil
	ok := commentTail(ctx, &report.Fields, s, offset, joined)
	report.AltitudeInComment = report.Fields.AltitudeFeet != nil
	if !report.AltitudeInComment {
		report.Fields.AltitudeFeet = altitudeBefore
	}
	return ok
}

// joinsIntoAltitude says whether the text left once base-91 telemetry and the !DAO! are lifted
// out of s holds a Mic-E altitude, xxx}, across a place one was taken from.
func joinsIntoAltitude(s []byte) bool {
	rest := append([]byte(nil), s...)
	// Spans come last first, so each join's place in what is left is its span's start less the
	// lengths of the spans before it, taken out later.
	spans := telemetryAndDAOSpans(s)
	for _, r := range spans {
		rest = drain(rest, r.start, r.end)
	}
	for _, r := range spans {
		before := 0
		for _, o := range spans {
			if o.start < r.start {
				before += o.length()
			}
		}
		join := r.start - before
		for j := max(join-3, 0); j < join; j++ {
			if _, ok := micEAltitude(rest[j:]); ok {
				return true
			}
		}
	}
	return false
}

// micEAltitude reads a Mic-E altitude, xxx}, at the start of s, in feet.
func micEAltitude(s []byte) (float64, bool) {
	if len(s) >= 4 && s[3] == '}' {
		v, ok := base91Decode(s[:3])
		if !ok {
			return 0, false
		}
		metres := float64(v) - 10000
		return metres * metresToFeet, true
	}
	return 0, false
}

// locatorLen reads AA00 or AA00aa, then /G.
func locatorLen(s []byte) (int, bool) {
	letters := func(b []byte, hi byte) bool {
		for _, c := range b {
			u := toUpper(c)
			if u < 'A' || u > hi {
				return false
			}
		}
		return true
	}
	if !(len(s) >= 6 && letters(s[0:2], 'R') && isDigit(s[2]) && isDigit(s[3])) {
		return 0, false
	}
	if len(s) >= 8 && letters(s[4:6], 'X') && string(s[6:8]) == "/G" {
		return 6, true
	}
	if string(s[4:6]) == "/G" {
		return 4, true
	}
	return 0, false
}

// encodeMicE gives the destination address and information field for a Mic-E report. An altitude
// goes in the Mic-E altitude bytes (whole metres) unless the report says it is in the comment, as
// /A= feet (vectors README, Builder rules).
func encodeMicE(r *MicEReport) (Address, []byte, *EncodeError) {
	f := &r.Fields
	pend := &pending{}
	pend.run(checkSymbol(f.Symbol))
	given := f.Position
	fine := true
	switch {
	case !isFinite(given.Latitude) || !isFinite(given.Longitude):
		pend.note(newEncodeError(RefusalNotANumber, "a Mic-E position is made of numbers"))
		fine = false
	case !inRange(given.Latitude, -90, 90) || !inRange(given.Longitude, -180, 180):
		pend.note(newEncodeError(RefusalValueOutOfRange, "a Mic-E position must be within -90 to 90 and -180 to 180 degrees"))
		fine = false
	case given.Longitude == 180:
		pend.note(newEncodeError(RefusalValueHasNoForm, "Mic-E cannot carry a longitude of 180 degrees"))
		fine = false
	}
	if given.Ambiguity > 4 {
		pend.note(newEncodeError(RefusalValueOutOfRange, "position ambiguity is 0-4 digits"))
		fine = false
	}
	// The rest is worked out from a position that is safe to write when this one is not.
	p := given
	if !fine {
		p = Position{}
	}
	if f.Compressed || f.Weather != nil || f.Area != nil || f.DFBearing != nil || f.Storm != nil || f.Signpost != nil {
		pend.note(newEncodeError(RefusalFieldNotAllowed, "Mic-E carries no compression, weather, area, DF bearing, storm or signpost"))
	}
	extensions := 0
	for _, x := range []bool{f.PHG != nil, f.RangeMiles != nil, f.DFS != nil} {
		if x {
			extensions++
		}
	}
	if extensions > 1 {
		pend.note(newEncodeError(RefusalExtensionConflict, "only one data extension (PHG, RNG or DFS) fits in a report"))
	}
	if r.DestinationSSID > 15 {
		pend.note(newEncodeError(RefusalValueOutOfRange, "the destination SSID is 0-15"))
	}
	bits, custom := 0, false
	if n, isCustom := r.MicEMessage.Custom(); isCustom {
		if n > 6 {
			pend.note(newEncodeError(RefusalInvalidCode, "custom Mic-E messages are C0-C6"))
		} else {
			bits, custom = 7-int(n), true
		}
	} else if r.MicEMessage == MicEUnknown {
		pend.note(newEncodeError(RefusalInvalidCode, "an unknown Mic-E message code (mixed standard and custom bits) cannot be sent"))
	} else {
		found := false
		for i, m := range micEMessages {
			if m == r.MicEMessage {
				bits, found = i, true
				break
			}
		}
		if !found {
			pend.note(newEncodeError(RefusalInvalidCode, "not a Mic-E message code"))
		}
	}
	// Whatever its precision: even !W  ! reads back as dao-with-ambiguity.
	if f.DAO != nil && given.Ambiguity > 0 {
		pend.note(newEncodeError(RefusalFieldNotAllowed, "a !DAO! on an ambiguous position contradicts it, whatever its precision"))
	}
	var dao *bool
	if f.DAO != nil && f.DAO.Precision != DAONone {
		dao = Ptr(f.DAO.Precision == DAOBase91)
	}
	perMinute, extraBase := 100.0, uint64(1)
	if dao != nil {
		if *dao {
			perMinute, extraBase = 9100, 91
		} else {
			perMinute, extraBase = 1000, 10
		}
	}

	// Latitude digits into the destination, blanked for ambiguity: rounding then blanking keeps
	// the box the position is in, and a decoded position (the centre of its box) writes back the
	// same digits.
	var latUnits uint64
	var latExtra byte
	if p.Ambiguity > 0 {
		// Cut to the box, not rounded and then cut again.
		latUnits = cutToBox(p.Latitude, p.Ambiguity)
	} else {
		latUnits = toU64(roundSteps(absFloat(p.Latitude) * 60 * perMinute))
		latExtra = byte(latUnits % extraBase)
		latUnits /= extraBase
	}
	// The box, in hundredths of a minute, that each level of ambiguity blanks.
	boxSize := [...]uint64{1, 10, 100, 1000, 6000}[p.Ambiguity]
	if p.Ambiguity > 0 && latUnits/boxSize*boxSize+boxSize/2 > 90*6000 {
		pend.note(newEncodeError(RefusalValueHasNoForm, "an ambiguous latitude whose box is centred past 90 degrees cannot be sent"))
	}
	latText := fmt.Sprintf("%02d%02d%02d", latUnits/6000, latUnits%6000/100, latUnits%100)
	var lonUnits uint64
	var lonExtra byte
	if p.Ambiguity > 0 {
		// The longitude digits the ambiguity makes a decoder ignore hold the centre it reports
		// (vectors README, Mic-E), of the box the longitude is cut to.
		lonUnits = cutToBox(p.Longitude, p.Ambiguity) + boxSize/2
	} else {
		lonUnits = toU64(roundSteps(absFloat(p.Longitude) * 60 * perMinute))
		lonExtra = byte(lonUnits % extraBase)
		lonUnits /= extraBase
	}
	lonDeg := uint32(lonUnits / 6000)
	lonMin := uint32(lonUnits % 6000 / 100)
	lonHun := uint32(lonUnits % 100)
	if lonDeg >= 180 {
		pend.note(newEncodeError(RefusalValueHasNoForm, "Mic-E cannot carry a longitude of 180 degrees"))
	}
	offset := !(lonDeg >= 10 && lonDeg <= 99)

	var dest []byte
	for i := 0; i < len(latText); i++ {
		blank := i >= 6-int(p.Ambiguity)
		d := latText[i] - '0'
		var flag, isCustom bool
		switch {
		case i <= 2:
			flag, isCustom = (bits>>(2-i))&1 == 1, custom
		case i == 3:
			flag = p.Latitude >= 0 && !signbit(p.Latitude)
		case i == 4:
			flag = offset
		default:
			flag = signbit(p.Longitude)
		}
		switch {
		case flag && isCustom && !blank:
			dest = append(dest, 'A'+d)
		case flag && isCustom && blank:
			dest = append(dest, 'K')
		case flag && !blank:
			dest = append(dest, 'P'+d)
		case flag && blank:
			dest = append(dest, 'Z')
		case !blank:
			dest = append(dest, '0'+d)
		default:
			dest = append(dest, 'L')
		}
	}
	if r.DestinationSSID != 0 {
		dest = append(dest, fmt.Sprintf("-%d", r.DestinationSSID)...)
	}

	out := make([]byte, 0, 32)
	if r.OldData {
		out = append(out, '\'')
	} else {
		out = append(out, '`')
	}
	switch {
	case lonDeg <= 9:
		out = append(out, byte(lonDeg+118))
	case lonDeg <= 99:
		out = append(out, byte(lonDeg+28))
	case lonDeg <= 109:
		out = append(out, byte(lonDeg+8))
	default:
		out = append(out, byte(lonDeg-72))
	}
	if lonMin < 10 {
		out = append(out, byte(lonMin+88))
	} else {
		out = append(out, byte(lonMin+28))
	}
	out = append(out, byte(lonHun+28))
	var speed uint32
	if f.SpeedKnots != nil {
		// Rounded first, so -0.3 is 0 and 799.4 is 799.
		s := *f.SpeedKnots
		if inRange(roundSteps(s), 0, 799) {
			speed = toU32(roundSteps(s))
		} else {
			pend.note(newEncodeError(refusalForValue(s), "Mic-E speed is 0-799 knots"))
		}
	}
	var course uint32
	if f.CourseDegrees != nil {
		whole, err := wholeDegrees(*f.CourseDegrees, "course")
		pend.run(err)
		// A course of 0 is how Mic-E sends no course, so it would read back as none.
		if whole == 0 {
			pend.note(newEncodeError(RefusalValueHasNoForm, "a Mic-E course of 0 means an unknown course (APRS12c ch. 10), so it cannot be sent: due north is 360"))
		}
		course = whole
	}
	// The printable forms APRS12c ch. 10 allows, as its own examples use them: speed tens + 80
	// from 'l' below 190 knots, then '/' for 190-199 (the other form is DEL, a control
	// character), and course hundreds + 4.
	if speed < 190 {
		out = append(out, byte(108+speed/10))
	} else {
		out = append(out, byte(28+speed/10))
	}
	out = append(out, byte(32+(speed%10)*10+course/100))
	out = append(out, byte(28+course%100))
	out = append(out, byte(f.Symbol.Code), byte(f.Symbol.Table))

	statusStart := len(out)
	if len(r.LegacyTelemetry) > 0 {
		// A 255 would be taken for Kenwood 0xFF padding and removed on the way back in.
		if contains(r.LegacyTelemetry, 255) {
			pend.note(newEncodeError(RefusalValueHasNoForm, "obsolete Mic-E binary telemetry is each 0-254"))
		}
		if len(r.LegacyTelemetry) != 5 {
			pend.note(newEncodeError(RefusalMalformed, "obsolete Mic-E binary telemetry is 5 values"))
		}
		out = append(out, 0x1D)
		out = append(out, r.LegacyTelemetry...)
	}
	if r.TypeCode != nil {
		t := *r.TypeCode
		if !(t == '`' || t == '\'' || t == '>' || t == ']' || t == ' ') {
			pend.note(newEncodeError(RefusalInvalidCode, "a Mic-E type code is `, ', >, ] or a space"))
		}
		out = append(out, byte(t))
	}
	// Mic-E altitude is whole metres; one that is not exactly (a /A= altitude in feet, such as
	// 30105, which is 9176 metres less 0.01 foot) goes in the text as /A=, so that it reads back
	// the same (vectors README, Encoding).
	var altitudeInText *float64
	if f.AltitudeFeet != nil {
		feet := *f.AltitudeFeet
		metres := roundSteps(feet / metresToFeet)
		if r.AltitudeInComment || !sameNumber(metres*metresToFeet, feet) {
			altitudeInText = &feet
		} else {
			datum := metres + 10000
			if !(datum >= 0 && datum < 91*91*91) {
				pend.note(newEncodeError(refusalForValue(feet), "the Mic-E altitude is out of range"))
			} else {
				at := len(out)
				out = base91Encode(out, toU32(datum), 3)
				out = append(out, '}')
				// With no type code before it (legacy telemetry is taken off first), a first
				// byte that is one reads back as the type code, and beyond six digits no /A= can
				// say the altitude instead.
				legacy := 0
				if len(r.LegacyTelemetry) > 0 {
					legacy = 6
				}
				if r.TypeCode == nil && at == statusStart+legacy && isTypeCode(out[at]) && !inRange(roundSteps(feet), -99999, 999999) {
					pend.note(newEncodeError(RefusalValueHasNoForm, "this Mic-E altitude has no form: its first byte would read as a type code and no /A= holds it"))
				}
			}
		}
	}
	if r.Locator != nil {
		b := []byte(*r.Locator)
		if n, ok := locatorLen(append(append([]byte(nil), b...), '/', 'G')); !ok || n != len(b) {
			pend.note(newEncodeError(RefusalMalformed, "a grid locator is 4 or 6 characters, e.g. IO91 or IO91SX"))
		}
		out = append(out, b...)
		out = append(out, '/', 'G')
		// Anything after the locator and its symbol comes after a space (APRS12c ch. 10), a data
		// extension included; the device suffix is read off the end first, and base-91 telemetry
		// and a !DAO! are lifted out before the locator is read, so they need none.
		if altitudeInText != nil || f.PHG != nil || f.RangeMiles != nil || f.DFS != nil || f.Frequency != nil || f.Comment != "" {
			out = append(out, ' ')
		}
	}
	beforeExtension := len(out)
	// One of PHG, RNG and DFS is written; any other keeps its own checks.
	exts := []struct {
		present bool
		write   func(out []byte) []byte
	}{
		{f.PHG != nil, func(out []byte) []byte {
			var err *EncodeError
			out, err = phgCodes(out, *f.PHG)
			pend.run(err)
			if f.PHG.BeaconsPerHour != nil {
				if c, ok := beaconRateChar(*f.PHG.BeaconsPerHour); ok {
					out = append(out, c, '/')
				} else {
					pend.note(newEncodeError(RefusalValueOutOfRange, "PHGR beacons per hour is 1-35"))
				}
			}
			return out
		}},
		{f.RangeMiles != nil, func(out []byte) []byte {
			rng := *f.RangeMiles
			n := roundSteps(rng)
			if !inRange(n, 0, 9999) {
				pend.note(newEncodeError(refusalForValue(rng), "RNG range is 0-9999 miles"))
			} else {
				out = append(out, fmt.Sprintf("RNG%04d", toU32(n))...)
			}
			return out
		}},
		{f.DFS != nil, func(out []byte) []byte {
			var err *EncodeError
			out, err = dfsCodes(out, *f.DFS)
			pend.run(err)
			return out
		}},
	}
	written := false
	for _, e := range exts {
		if !e.present {
			continue
		}
		if written {
			e.write(nil)
		} else {
			out = e.write(out)
			written = true
		}
	}
	afterExtension := -1
	if len(out) > beforeExtension {
		afterExtension = len(out)
	}
	if altitudeInText != nil {
		feet := *altitudeInText
		n := roundSteps(feet)
		switch {
		case !inRange(n, -99999, 999999):
			pend.note(newEncodeError(refusalForValue(feet), "/A= altitude must fit in 6 digits (or - and 5 digits)"))
		case n < 0:
			out = append(out, fmt.Sprintf("/A=-%05d", toU32(-n))...)
		default:
			out = append(out, fmt.Sprintf("/A=%06d", toU32(n))...)
		}
	}

	if f.Frequency != nil {
		// Straight after a data extension, a / (a PHGR already ends in one), as in a position's
		// comment; after a /A= altitude, straight on.
		if afterExtension == len(out) && out[len(out)-1] != '/' {
			out = append(out, '/')
		}
		out = encodeFrequency(out, f.Frequency, pend)
	}

	text := []byte(f.Comment)
	// A weather report has no comment: that is the fault, and the characters are not looked at.
	if f.Weather == nil && hasLineBreak(text) {
		pend.note(newEncodeError(RefusalInvalidCharacter, "comment text cannot contain a line break"))
	}
	var trailer []byte
	if f.Telemetry != nil {
		trailer = telemetryInto(trailer, f.Telemetry, pend)
	}
	if f.DAO != nil {
		d := *f.DAO
		pend.run(checkDAODatum(d))
		trailer = append(trailer, '!')
		switch d.Precision {
		case DAOThousandths:
			trailer = append(trailer, byte(d.Datum), '0'+latExtra, '0'+lonExtra)
		case DAOBase91:
			trailer = append(trailer, byte(runeToASCIILower(d.Datum)), latExtra+33, lonExtra+33)
		default:
			trailer = append(trailer, byte(d.Datum), ' ', ' ')
		}
		trailer = append(trailer, '!')
	}
	suffix := []byte(r.DeviceSuffix)
	// A device suffix belongs to a type code and must be one the database knows for it.
	if len(suffix) > 0 {
		if r.TypeCode == nil {
			pend.note(newEncodeError(RefusalMissingField, "a Mic-E device suffix goes with a type code"))
		} else {
			t := byte(*r.TypeCode)
			if micESuffixLen(&t, suffix) != len(suffix) {
				pend.note(newEncodeError(RefusalInvalidCode, "the device suffix is not one the database knows for this type code"))
			}
		}
	}
	if err := pend.finish(); err != nil {
		return Address{}, nil, err
	}

	textAt := len(out)
	needsSpace := len(text) > 0 && f.Frequency != nil
	// Status text that would start with a type code character or 0x1D (Rev 0 telemetry) is
	// written after a /, even when it would read back without one (vectors README, Encoding). Only
	// the start of the status text counts: after a type code or telemetry, it is written straight
	// on.
	statusStartsWithCode := textAt == statusStart && len(text) > 0 && (isTypeCode(text[0]) || text[0] == 0x1D)
	var separators []string
	switch {
	case needsSpace:
		separators = []string{" ", "", " /"}
	case statusStartsWithCode:
		separators = []string{"/"}
	default:
		// A space last: it keeps text such as {5} from joining the digits before it into a Mic-E
		// altitude (1{5}), where / would join them too (vectors README, Encoding).
		separators = []string{"", "/", " "}
	}
	for _, separator := range separators {
		out = out[:textAt]
		out = append(out, separator...)
		out = append(out, text...)
		out = append(out, trailer...)
		out = append(out, suffix...)
		// A CR or LF at the end (the last byte of Rev 0 telemetry, say) reads back as a trailing
		// line break, a warning.
		if last := out[len(out)-1]; last == '\r' || last == '\n' {
			return Address{}, nil, newEncodeError(RefusalInvalidCharacter, "the information field would end in a CR or LF, which reads back as a trailing line break")
		}
		if micEReadsBack(r, out[statusStart:]) {
			destination, err := NewAddress(string(dest))
			if err != nil {
				return Address{}, nil, newEncodeError(RefusalInvalidAddress, "the Mic-E destination is not an address")
			}
			return destination, out, nil
		}
	}
	return Address{}, nil, newEncodeError(RefusalTextReadsAsField, "the status text does not read back as given: a device suffix the database does not know, or text that decodes as a structured element; set the property instead")
}

func micEReadsBack(r *MicEReport, status []byte) bool {
	ctx := newContext(Lenient)
	back := MicEReport{Fields: Positioned{Symbol: r.Fields.Symbol}}
	if !micEStatus(ctx, &back, status, 0) || ctx.hasErrors() {
		return false
	}
	a, b := &back.Fields, &r.Fields
	upper := func(s *string) *string {
		if s == nil {
			return nil
		}
		return Ptr(asciiUpper(*s))
	}
	datum := func(d *DAO) *rune {
		if d == nil {
			return nil
		}
		return &d.Datum
	}
	round := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		return Ptr(roundSteps(*v))
	}
	close := func(x, y *float64) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		return sameNumber(*x, *y)
	}
	return samePtr(back.TypeCode, r.TypeCode) &&
		bytes.Equal(back.LegacyTelemetry, r.LegacyTelemetry) &&
		back.DeviceSuffix == r.DeviceSuffix &&
		samePtr(upper(back.Locator), upper(r.Locator)) &&
		a.Comment == b.Comment &&
		samePHG(a.PHG, b.PHG) &&
		samePtr(a.DFS, b.DFS) &&
		sameCommentTelemetry(a.Telemetry, b.Telemetry) &&
		samePtr(datum(a.DAO), datum(b.DAO)) &&
		sameFrequencyFields(a.Frequency, b.Frequency) &&
		// A /A= altitude is whole feet, rounded as a position's is.
		(close(a.AltitudeFeet, b.AltitudeFeet) || samePtr(round(a.AltitudeFeet), round(b.AltitudeFeet))) &&
		samePtr(round(a.RangeMiles), round(b.RangeMiles))
}
