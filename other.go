package aprs

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The other data types: positionless and raw weather (APRS12c ch. 12), raw NMEA (ch. 7),
// Maidenhead beacons, general queries and station capabilities (ch. 15), third-party traffic
// (ch. 17), user-defined data (ch. 19), test data (ch. 20) and Agrelo DF reports.

func decodeRawWeather(ctx *context, info []byte, format RawWeatherFormat, skip int) Data {
	ctx.info(CodeObsoleteFormat, "raw weather station output, which the spec keeps only for old stations (APRS12c ch. 12)", 0)
	raw := info[skip:]
	if !isPrintableASCII(raw) {
		ctx.error(CodeInvalidWeather, "raw weather station data is printable ASCII", skip)
		return nil
	}
	return RawWeather{Format: format, Data: string(raw)}
}

// decodePositionlessWeather reads _MMDDHHMM then weather fields, wind first as c and s.
func decodePositionlessWeather(ctx *context, info []byte) Data {
	ctx.info(CodeObsoleteFormat, "a positionless weather report; APRS12c ch. 12 prefers weather with a position", 0)
	var timestamp Timestamp
	b, ok := get(info, 1, 9)
	if ok {
		timestamp, ok = ParseTimestampMDHM(b)
	}
	if !ok {
		ctx.error(CodeInvalidTimestamp, "a positionless weather report starts with an 8-digit MDHM timestamp (APRS12c ch. 12)", 1)
		return nil
	}
	if !timestamp.IsValid() && !ctx.tolerate(CodeInvalidTimestamp, "the timestamp is out of range (APRS12c ch. 6)", 1) {
		return nil
	}
	weather := Weather{}
	run, ok := weatherFields(ctx, info[9:], 9, &weather, windPositionless)
	if !ok {
		return nil
	}
	at := 9 + run.length
	rest := info[at:]
	comment := ""
	if !softwareAndUnit(rest, &weather) && len(rest) > 0 {
		if !ctx.tolerate(CodeWeatherComment, "text after the weather data; a weather report has no comment (UAP 2.7.1)", at) {
			return nil
		}
		c, ok := decodeText(ctx, rest, at)
		if !ok {
			return nil
		}
		comment = c
	}
	return WeatherReport{Timestamp: &timestamp, Weather: weather, Comment: comment}
}

// decodeNMEA reads raw NMEA: after $, an NMEA 0183 sentence (vectors interpretations.md, "What $
// text is an NMEA sentence"): printable ASCII, an address field of five upper-case letters or
// digits (or P and three or more, a proprietary sentence) and at least one field, with no $ in it
// and no * except the one that starts a checksum. The checksum, * and two hex digits, ends the
// sentence; any text after it is the comment. The structure is checked before the checksum. GGA,
// GLL, RMC, VTG and WPL are read, field by field; a field that is missing or does not parse is
// left out.
func decodeNMEA(ctx *context, info []byte) Data {
	ctx.info(CodeObsoleteFormat, "raw NMEA, which APRS12c ch. 7 does not recommend", 0)
	body := info[1:]
	sentence, ok := nmeaSentenceOf(body)
	if !ok {
		ctx.error(CodeInvalidNMEA, "not an NMEA 0183 sentence: printable ASCII, an address field such as GPRMC, then fields, with * only before a checksum", 1)
		return nil
	}
	if sentence.hasChecksum && sentence.checksum != nmeaChecksum(sentence.content) {
		ctx.error(CodeNMEAChecksumMismatch, "the NMEA checksum does not match, so the sentence is corrupt", 1)
		return nil
	}
	commentAt := 1 + sentence.length
	comment, ok := decodeText(ctx, body[sentence.length:], commentAt)
	if !ok {
		return nil
	}
	n := nmeaFields(sentence.content)
	n.Sentence = string(body[:sentence.length])
	n.HasChecksum = sentence.hasChecksum
	n.Comment = comment
	return n
}

// nmeaSentence is the structure of an NMEA sentence at the start of the text after $.
type nmeaSentence struct {
	// content is the address field and data fields: everything before any *.
	content string
	// checksum is the checksum sent, if any.
	checksum    byte
	hasChecksum bool
	// length is the bytes the sentence takes, checksum included; any after it are the comment.
	length int
}

// nmeaSentenceOf reads the sentence structure; false when the text is not an NMEA sentence.
func nmeaSentenceOf(body []byte) (nmeaSentence, bool) {
	s := nmeaSentence{}
	content := body
	s.length = len(body)
	if i := indexByte(body, '*'); i >= 0 {
		// A * that does not start a checksum is a reserved character in a field.
		hex, ok := get(body, i+1, i+3)
		if !ok || !isHex(hex[0]) || !isHex(hex[1]) {
			return nmeaSentence{}, false
		}
		v, _ := strconv.ParseUint(string(hex), 16, 8)
		content, s.checksum, s.hasChecksum, s.length = body[:i], byte(v), true, i+3
	}
	if !isPrintableASCII(content) || contains(content, '$') {
		return nmeaSentence{}, false
	}
	s.content = string(content)
	address, _, found := strings.Cut(s.content, ",")
	if !found {
		return nmeaSentence{}, false
	}
	for i := 0; i < len(address); i++ {
		if !isUpper(address[i]) && !isDigit(address[i]) {
			return nmeaSentence{}, false
		}
	}
	if !(len(address) == 5 || (strings.HasPrefix(address, "P") && len(address) >= 4)) {
		return nmeaSentence{}, false
	}
	return s, true
}

// nmeaChecksum is the XOR of every character between $ and *.
func nmeaChecksum(content string) byte {
	var sum byte
	for i := 0; i < len(content); i++ {
		sum ^= content[i]
	}
	return sum
}

// nmeaFields reads the fields of a GGA, GLL, RMC, VTG or WPL sentence, by position. Only a
// five-character address that does not start P has a sentence formatter, in its last three
// characters.
func nmeaFields(content string) NMEA {
	f := strings.Split(content, ",")
	field := func(i int) (string, bool) {
		if i < len(f) {
			return f[i], true
		}
		return "", false
	}
	n := NMEA{}
	position := func(at int) {
		latitude, ok1 := nmeaCoordinate(field, at, "N", "S", 90)
		longitude, ok2 := nmeaCoordinate(field, at+2, "E", "W", 180)
		if ok1 && ok2 {
			n.Latitude, n.Longitude = Ptr(latitude), Ptr(longitude)
		}
	}
	number := func(i int) *float64 {
		if t, ok := field(i); ok {
			if v, ok := nmeaNumber(t); ok {
				return &v
			}
		}
		return nil
	}
	time := func(i int) *string {
		if t, ok := field(i); ok {
			if v, ok := nmeaTime(t); ok {
				return &v
			}
		}
		return nil
	}
	status := func(i int) *bool {
		if t, ok := field(i); ok {
			switch t {
			case "A":
				return Ptr(true)
			case "V":
				return Ptr(false)
			}
		}
		return nil
	}
	address := f[0]
	formatter := ""
	if len(address) == 5 && !strings.HasPrefix(address, "P") {
		formatter = address[2:]
	}
	switch formatter {
	case "GGA":
		n.Time = time(1)
		position(2)
		if q, ok := field(6); ok && len(q) == 1 && isDigit(q[0]) {
			n.FixValid = Ptr(q[0] != '0')
		}
		n.AltitudeM = number(9)
	case "RMC":
		n.Time = time(1)
		n.FixValid = status(2)
		position(3)
		n.SpeedKnots = number(7)
		n.CourseDegrees = number(8)
	case "GLL":
		position(1)
		n.Time = time(5)
		n.FixValid = status(6)
	case "VTG":
		n.CourseDegrees = number(1)
		n.SpeedKnots = number(5)
	case "WPL":
		position(1)
		if w, ok := field(5); ok && w != "" {
			n.Waypoint = Ptr(w)
		}
	}
	return n
}

// nmeaNumber reads a speed, course or altitude: an optional -, then digits with an optional .
// and fraction (the digits before or after the . may be left out, not both).
func nmeaNumber(t string) (float64, bool) {
	d := strings.TrimPrefix(t, "-")
	whole, fraction, _ := strings.Cut(d, ".")
	if whole == "" && fraction == "" {
		return 0, false
	}
	if !digitsOnly(whole) || !digitsOnly(fraction) {
		return 0, false
	}
	v, ok := parseFloat(t)
	if !ok || !isFinite(v) {
		return 0, false
	}
	return v, true
}

func digitsOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// nmeaTime reads hhmmss (hours 00-23, minutes and seconds 00-59) with an optional . and fraction,
// as HH:MM:SS.f: the fraction as sent, less trailing zeros.
func nmeaTime(t string) (string, bool) {
	if len(t) < 6 || !digitsOnly(t[:6]) {
		return "", false
	}
	fraction := t[6:]
	if !(fraction == "" || (fraction[0] == '.' && digitsOnly(fraction[1:]))) {
		return "", false
	}
	pair := func(i int) int { return int(t[i]-'0')*10 + int(t[i+1]-'0') }
	if pair(0) > 23 || pair(2) > 59 || pair(4) > 59 {
		return "", false
	}
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "." {
		fraction = ""
	}
	return t[0:2] + ":" + t[2:4] + ":" + t[4:6] + fraction, true
}

// nmeaCoordinate reads an NMEA coordinate, ddmm.mm or dddmm.mm: digits with an optional . and
// fraction, at least three before the .; the last two of those are minutes (below 60) and the
// rest degrees, however many digits they have. Within limit degrees, with the hemisphere letter
// exact.
func nmeaCoordinate(field func(int) (string, bool), at int, positive, negative string, limit float64) (float64, bool) {
	value, ok := field(at)
	if !ok {
		return 0, false
	}
	hemisphere, ok := field(at + 1)
	if !ok {
		return 0, false
	}
	var sign float64
	switch hemisphere {
	case positive:
		sign = 1
	case negative:
		sign = -1
	default:
		return 0, false
	}
	whole, fraction, _ := strings.Cut(value, ".")
	if len(whole) < 3 || !digitsOnly(whole) || !digitsOnly(fraction) {
		return 0, false
	}
	degreesText, minutesText := whole[:len(whole)-2], whole[len(whole)-2:]
	degrees, ok := parseFloat(degreesText)
	if !ok {
		return 0, false
	}
	if fraction != "" {
		minutesText += "." + fraction
	}
	minutes, ok := parseFloat(minutesText)
	if !ok {
		return 0, false
	}
	if minutes >= 60 {
		return 0, false
	}
	v := degrees + minutes/60
	if !(v <= limit) {
		return 0, false
	}
	return sign * v, true
}

// decodeMaidenheadBeacon reads [IO91SX] and a comment.
func decodeMaidenheadBeacon(ctx *context, info []byte) Data {
	ctx.info(CodeObsoleteFormat, "a Maidenhead locator beacon, which APRS12c ch. 7 marks obsolete", 0)
	end := indexByte(info, ']')
	if end < 0 {
		ctx.error(CodeInvalidLocator, "a locator beacon is [locator]", 1)
		return nil
	}
	locator := info[1:end]
	if !isLocator(locator) {
		ctx.error(CodeInvalidLocator, "the locator is not 4 or 6 characters, e.g. IO91 or IO91SX", 1)
		return nil
	}
	comment, ok := decodeText(ctx, info[end+1:], end+1)
	if !ok {
		return nil
	}
	return MaidenheadBeacon{Locator: asciiUpper(lossy(locator)), Comment: comment}
}

// decodeQuery reads ?TYPE?, optionally with a footprint: " lat,lon,rrrr" (a space for a positive
// value).
func decodeQuery(ctx *context, info []byte) Data {
	end := -1
	if i := indexByte(info[1:], '?'); i >= 0 {
		end = i + 1
	}
	if end < 0 {
		ctx.error(CodeInvalidGeneralQuery, "a general query is ?TYPE? (APRS12c ch. 15)", 0)
		return nil
	}
	queryType := info[1:end]
	ok := len(queryType) > 0
	for _, c := range queryType {
		ok = ok && isUpper(c)
	}
	if !ok {
		ctx.error(CodeInvalidGeneralQuery, "the query type is upper-case letters (APRS12c ch. 15)", 1)
		return nil
	}
	rest := info[end+1:]
	var fp *Footprint
	if len(rest) > 0 {
		f, ok := footprint(rest)
		if !ok {
			ctx.error(CodeInvalidGeneralQuery, "a query footprint is a latitude (to 90) and longitude (to 180) in degrees and a 4-digit radius (APRS12c ch. 15)", end+1)
			return nil
		}
		fp = &f
	}
	return Query{QueryType: string(queryType), Footprint: fp}
}

// footprint reads the latitude and longitude as sent, and the radius.
func footprint(rest []byte) (Footprint, bool) {
	if !utf8.Valid(rest) {
		return Footprint{}, false
	}
	parts := strings.Split(string(rest), ",")
	if len(parts) != 3 {
		return Footprint{}, false
	}
	latitude, ok := footprintDegrees(parts[0], 90)
	if !ok {
		return Footprint{}, false
	}
	longitude, ok := footprintDegrees(parts[1], 180)
	if !ok {
		return Footprint{}, false
	}
	radius := parts[2]
	if len(radius) != 4 || !digitsOnly(radius) {
		return Footprint{}, false
	}
	r, _ := strconv.ParseUint(radius, 10, 32)
	return Footprint{Latitude: latitude, Longitude: longitude, RadiusMiles: float64(r), LatitudeText: parts[0], LongitudeText: parts[1]}, true
}

// footprintDegrees reads a footprint's degrees: a decimal number, with a minus sign or, only for
// a positive value, a leading space ("Note the leading space in the latitude, as its value is
// positive", APRS12c ch. 15), within limit either way.
func footprintDegrees(t string, limit float64) (float64, bool) {
	var d string
	if strings.HasPrefix(t, " ") {
		t = t[1:]
		d = t
	} else {
		d = strings.TrimPrefix(t, "-")
	}
	whole, fraction, _ := strings.Cut(d, ".")
	if (whole == "" && fraction == "") || !digitsOnly(whole) || !digitsOnly(fraction) {
		return 0, false
	}
	v, ok := parseFloat(t)
	if !ok || !(absFloat(v) <= limit) {
		return 0, false
	}
	return v, true
}

// footprintText writes a footprint's latitude or longitude: the text it was sent as, when that is
// a number as APRS12c ch. 15 writes one and still says value, and otherwise the number as the
// shortest decimal that reads back the same, after a space unless it is negative (0 included).
func footprintText(value float64, text string, limit float64, what string) (string, *EncodeError) {
	if sent, ok := footprintDegrees(text, limit); ok && sent == value {
		return text, nil
	}
	if value != value || absFloat(value) > limit {
		return "", newEncodeError(refusalForValue(value), fmt.Sprintf("a query footprint's %s is decimal degrees within %v (a leading space only before a positive value)", what, limit))
	}
	digits := numberText(value)
	if value < 0 {
		return digits, nil
	}
	return " " + digits, nil
}

// decodeCapabilities reads TOKEN,TOKEN=VALUE,...
func decodeCapabilities(ctx *context, info []byte) Data {
	body, ok := decodeTextUnjudged(ctx, info[1:], 1)
	if !ok {
		return nil
	}
	// TOKEN or TOKEN=VALUE items, separated by commas and split at the first '='. The spaces
	// (U+0020 only) around an item, a token or a value are padding, not part of them.
	var items []Capability
	for _, item := range strings.Split(body, ",") {
		item = strings.Trim(item, " ")
		if item == "" {
			continue
		}
		if t, v, found := strings.Cut(item, "="); found {
			items = append(items, Capability{Token: strings.Trim(t, " "), Value: Ptr(strings.Trim(v, " "))})
		} else {
			items = append(items, Capability{Token: item})
		}
	}
	if len(items) == 0 {
		ctx.error(CodeInvalidCapabilities, "a capabilities report lists at least one capability (APRS12c ch. 15)", 1)
		return nil
	}
	// An empty token, or one with a space or a control character in it, or a value with a control
	// character, is free text: a beacon sent with the wrong data type identifier.
	freeText := false
	for _, c := range items {
		if c.Token == "" || strings.ContainsFunc(c.Token, func(r rune) bool { return r <= ' ' || r == 0x7F }) ||
			(c.Value != nil && strings.ContainsFunc(*c.Value, func(r rune) bool { return r < ' ' || r == 0x7F })) {
			freeText = true
		}
	}
	if freeText && !ctx.tolerate(CodeFreeTextCapabilities, "a capabilities packet holds free text rather than TOKEN / TOKEN=VALUE items (APRS12c ch. 15)", 1) {
		return nil
	}
	// A value is text: a control character in one is also a defect of its own, judged last.
	for _, c := range items {
		if c.Value != nil && hasControl(*c.Value) {
			if !ctx.tolerate(CodeControlCharacterInText, "a capability value with a control character in it (a byte below 0x20 or 0x7F); APRS12c gives text as printable characters", 1) {
				return nil
			}
			break
		}
	}
	return Capabilities{Capabilities: items}
}

// maxThirdPartyDepth is how many third-party headers a decoder reads, one inside another.
const maxThirdPartyDepth = 4

// decodeThirdParty reads } and a whole TNC2 packet, decoded with the same options. Its source may
// be any 1-9 printable ASCII characters other than > and : (APRS12c ch. 17); a defect its header
// may tolerate is a warning on the inner packet, and rejects it when not tolerated.
func decodeThirdParty(ctx *context, info []byte) Data {
	if ctx.depth >= maxThirdPartyDepth {
		ctx.error(CodeInvalidThirdParty, "third-party packets nest at most four deep", 1)
		return nil
	}
	inner, err := decodeTNC2Line(info[1:], ctx.options, ctx.depth+1)
	if err != nil {
		ctx.error(CodeInvalidThirdParty, "the third-party header is not SOURCE>DEST,PATH: (APRS12c ch. 17)", 1)
		return nil
	}
	return ThirdParty{Packet: inner}
}

func decodeUserDefined(ctx *context, info []byte) Data {
	if len(info) < 3 {
		ctx.error(CodeInvalidUserDefined, "user-defined data needs { then a user ID and a packet type (APRS12c ch. 19)", 0)
		return nil
	}
	return UserDefined{UserID: rune(info[1]), PacketType: rune(info[2]), Data: append([]byte(nil), info[3:]...)}
}

func decodeTestData(ctx *context, info []byte) Data {
	text, ok := decodeTextUnjudged(ctx, info[1:], 1)
	if !ok {
		return nil
	}
	return TestData{Data: text}
}

// decodeAgrelo reads %bbb/q, exactly: a bearing of 000 to 360 and a quality digit (APRS12c
// Appendix 1).
func decodeAgrelo(ctx *context, info []byte) Data {
	if len(info) != 6 || !allDigits(info[1:4]) || info[4] != '/' || !isDigit(info[5]) || digits(info[1:4]) > 360 {
		ctx.error(CodeInvalidAgreloDF, "an Agrelo DF report is exactly %bbb/q, with a bearing of 000 to 360", 0)
		return nil
	}
	return AgreloDF{BearingDegrees: uint16(digits(info[1:4])), Quality: info[5] - '0'}
}

func encodeOther(data Data) ([]byte, *EncodeError) {
	printable := func(t, what string) *EncodeError {
		if hasControl(t) {
			return newEncodeError(RefusalInvalidCharacter, what+" cannot contain a control character")
		}
		return nil
	}
	var out []byte
	switch d := data.(type) {
	case Status:
		return encodeStatus(&d)
	case Telemetry:
		return encodeTelemetry(&d)
	case WeatherReport:
		p := &pending{}
		var t *Timestamp
		switch {
		case d.Timestamp == nil:
			p.note(newEncodeError(RefusalMissingField, "a positionless weather report needs a timestamp (APRS12c ch. 12)"))
		case d.Timestamp.Kind == MonthDayHourMinute:
			p.run(checkTimestamp(*d.Timestamp))
			t = d.Timestamp
		default:
			p.note(newEncodeError(RefusalInvalidTimestamp, "a positionless weather report has an MDHM timestamp (APRS12c ch. 12)"))
		}
		if d.Comment != "" {
			p.note(newEncodeError(RefusalFieldNotAllowed, "a weather report has no comment field (APRS12c ch. 12)"))
		}
		var direction *float64
		if d.Weather.WindDirectionDegrees != nil {
			if v, err := wholeDegrees(*d.Weather.WindDirectionDegrees, "wind direction"); p.run(err) {
				direction = Ptr(float64(v))
			}
		}
		out = append(out, '_')
		if t != nil {
			out = append(out, t.bytes()...)
		}
		var err *EncodeError
		out, err = weatherField(out, 'c', direction, 3, false)
		p.run(err)
		out, err = weatherField(out, 's', d.Weather.WindSpeedMph, 3, false)
		p.run(err)
		out = writeWeatherFields(out, &d.Weather, p)
		if err := p.finish(); err != nil {
			return nil, err
		}
		// The software type and unit, and the extra fields, must not read back as fields.
		back, ok := decodePositionlessWeather(newContext(Lenient), out).(WeatherReport)
		if !ok || !sameWeatherText(&back.Weather, &d.Weather) {
			return nil, newEncodeError(RefusalTextReadsAsField, "the weather does not read back as given: a software type and unit, or an extra field, that would read as weather fields")
		}
	case RawWeather:
		if !isPrintableASCII([]byte(d.Data)) {
			return nil, newEncodeError(RefusalInvalidCharacter, "raw weather station data is printable ASCII")
		}
		switch d.Format {
		case PeetBrosHash:
			out = append(out, '#')
		case PeetBrosStar:
			out = append(out, '*')
		case UltimeterPacket:
			out = append(out, "$ULTW"...)
		default:
			out = append(out, "!!"...)
		}
		out = append(out, d.Data...)
	case NMEA:
		return encodeNMEA(&d)
	case MaidenheadBeacon:
		p := &pending{}
		if !isLocator([]byte(d.Locator)) || d.Locator != asciiUpper(d.Locator) {
			p.note(newEncodeError(RefusalMalformed, "a grid locator is 4 or 6 upper-case characters, e.g. IO91 or IO91SX"))
		}
		p.run(printable(d.Comment, "a beacon comment"))
		if err := p.finish(); err != nil {
			return nil, err
		}
		out = append(out, '[')
		out = append(out, asciiUpper(d.Locator)...)
		out = append(out, ']')
		out = append(out, d.Comment...)
	case Query:
		p := &pending{}
		typeOK := d.QueryType != ""
		for i := 0; i < len(d.QueryType); i++ {
			typeOK = typeOK && isUpper(d.QueryType[i])
		}
		if !typeOK {
			p.note(newEncodeError(RefusalInvalidCode, "a query type is upper-case letters (APRS12c ch. 15)"))
		}
		out = append(out, '?')
		out = append(out, d.QueryType...)
		out = append(out, '?')
		if f := d.Footprint; f != nil {
			// The numbers are written as they were sent when they still say the value, as
			// telemetry values are (vectors README, "Numbers as sent").
			radius := roundSteps(f.RadiusMiles)
			if !inRange(radius, 0, 9999) {
				p.note(newEncodeError(refusalForValue(f.RadiusMiles), "a query footprint's radius is 0-9999 miles once rounded"))
			}
			latitude, err := footprintText(f.Latitude, f.LatitudeText, 90, "latitude")
			p.run(err)
			longitude, err := footprintText(f.Longitude, f.LongitudeText, 180, "longitude")
			p.run(err)
			out = append(out, fmt.Sprintf("%s,%s,%04d", latitude, longitude, toU32(radius))...)
		}
		if err := p.finish(); err != nil {
			return nil, err
		}
	case Capabilities:
		if len(d.Capabilities) == 0 {
			return nil, newEncodeError(RefusalMissingField, "a capabilities report lists at least one token")
		}
		// What would not read back the same: a token is fixed-form, a value is free text whose
		// spaces around it would be taken for padding.
		p := &pending{}
		for _, c := range d.Capabilities {
			if c.Token == "" || strings.ContainsFunc(c.Token, func(r rune) bool { return r <= ' ' || r == 0x7F || r == ',' || r == '=' }) {
				p.note(newEncodeError(RefusalMalformed, "a capability token is not empty and has no spaces, ',', '=' or control characters"))
			}
			if c.Value != nil {
				v := *c.Value
				if strings.ContainsFunc(v, func(r rune) bool { return r < ' ' || r == 0x7F || r == ',' }) {
					p.note(newEncodeError(RefusalInvalidCharacter, "a capability value has no ',' or control characters"))
				}
				if strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") {
					p.note(newEncodeError(RefusalPaddingSpace, "a capability value does not start or end with a space"))
				}
			}
		}
		if err := p.finish(); err != nil {
			return nil, err
		}
		out = append(out, '<')
		for i, c := range d.Capabilities {
			if i > 0 {
				out = append(out, ',')
			}
			out = append(out, c.Token...)
			if c.Value != nil {
				out = append(out, '=')
				out = append(out, *c.Value...)
			}
		}
	case ThirdParty:
		// Walk the chain iteratively, counting levels before anything else is looked at: a
		// caller-built chain thousands deep is refused without recursion.
		depth := 0
		for cur := Data(d); ; {
			var tp ThirdParty
			switch x := cur.(type) {
			case ThirdParty:
				tp = x
			case *ThirdParty:
				if x == nil {
					depth = -1
				} else {
					tp = *x
				}
			default:
				depth = -1
			}
			if depth < 0 {
				break
			}
			depth++
			if depth > maxThirdPartyDepth {
				return nil, newEncodeError(RefusalInvalidThirdPartyHeader, "third-party packets nest at most four deep")
			}
			if tp.Packet == nil {
				break
			}
			cur = tp.Packet.Data
		}
		if d.Packet == nil {
			return nil, newEncodeError(RefusalMissingField, "a third-party packet carries a packet")
		}
		if err := thirdPartyHeader(d.Packet); err != nil {
			return nil, err
		}
		out = append(out, '}')
		out = append(out, d.Packet.TNC2()...)
		// The information field is written as received; only a line break at its very end would
		// not read back.
		if last := out[len(out)-1]; last == '\r' || last == '\n' {
			return nil, newEncodeError(RefusalInvalidCharacter, "a third-party packet's information field cannot end with a line break")
		}
	case UserDefined:
		// APRS12c ch. 19 puts no restriction on user-defined data, these two characters included:
		// any byte is written back as it came.
		if d.UserID < 0 || d.UserID > 0xFF || d.PacketType < 0 || d.PacketType > 0xFF {
			return nil, newEncodeError(RefusalValueOutOfRange, "the user ID and packet type are one byte each (U+0000 to U+00FF)")
		}
		out = append(out, '{', byte(d.UserID), byte(d.PacketType))
		out = append(out, d.Data...)
		// A line break at the very end would be taken for the end of the line and dropped.
		if last := out[len(out)-1]; last == '\r' || last == '\n' {
			return nil, newEncodeError(RefusalInvalidCharacter, "user-defined data cannot end with a line break, which would not read back")
		}
	case TestData:
		// Test data is data, not text: only a line break would not read back.
		if hasLineBreak([]byte(d.Data)) {
			return nil, newEncodeError(RefusalInvalidCharacter, "test data cannot contain a line break")
		}
		out = append(out, ',')
		out = append(out, d.Data...)
	case AgreloDF:
		if d.BearingDegrees > 360 || d.Quality > 9 {
			return nil, newEncodeError(RefusalValueOutOfRange, "an Agrelo bearing is 0-360 and the quality 0-9")
		}
		out = append(out, fmt.Sprintf("%%%03d/%d", d.BearingDegrees, d.Quality)...)
	case Unrecognized:
		return nil, newEncodeError(RefusalNotEncodable, "undecoded data has nothing to encode")
	default:
		return nil, newEncodeError(RefusalNotEncodable, "not one of the other data types")
	}
	return out, nil
}

// encodeNMEA writes $, the sentence and its comment, when they read back as given: the sentence
// well formed, its checksum (if any) right and ending it, a comment only after a checksum, and
// every field given agreeing with what the sentence says.
func encodeNMEA(n *NMEA) ([]byte, *EncodeError) {
	p := &pending{}
	sentence := []byte(n.Sentence)
	// A number that is not one is named before anything else about the sentence.
	for _, v := range []*float64{n.Latitude, n.Longitude, n.CourseDegrees, n.SpeedKnots, n.AltitudeM} {
		if v != nil && !isFinite(*v) {
			p.note(newEncodeError(RefusalNotANumber, "an NMEA field is not a number"))
		}
	}
	if hasControl(n.Comment) {
		p.note(newEncodeError(RefusalInvalidCharacter, "an NMEA comment cannot contain a control character"))
	}
	// $ULTW is raw weather, not NMEA, so such a sentence would not read back.
	structure, ok := nmeaSentenceOf(sentence)
	if !ok || structure.length != len(sentence) || strings.HasPrefix(n.Sentence, "ULTW") {
		p.note(newEncodeError(RefusalInvalidNMEA, "not an NMEA 0183 sentence: printable ASCII, an address field such as GPRMC, then fields, and at most a *hh checksum at the end"))
	} else {
		switch {
		case structure.hasChecksum && structure.checksum != nmeaChecksum(structure.content):
			p.note(newEncodeError(RefusalInvalidNMEA, "the NMEA checksum does not match the sentence"))
		case structure.hasChecksum && !n.HasChecksum:
			p.note(newEncodeError(RefusalInvalidNMEA, "the sentence has a *hh checksum but has_checksum is not set"))
		case !structure.hasChecksum && n.HasChecksum:
			p.note(newEncodeError(RefusalInvalidNMEA, "has_checksum is set but the sentence has no *hh checksum"))
		case !structure.hasChecksum && n.Comment != "":
			p.note(newEncodeError(RefusalInvalidNMEA, "an NMEA comment goes after a *hh checksum, which is what ends the sentence"))
		}
		read := nmeaFields(structure.content)
		// Degrees reached by other arithmetic differ in the last bits, so numbers agree within a
		// part in 10^9 (absolute below 1), as the vectors compare them.
		near := func(given, read *float64) bool {
			if given == nil {
				return true
			}
			if read == nil {
				return false
			}
			g, r := *given, *read
			return absFloat(g-r) <= 1e-9*fmax(fmax(absFloat(g), absFloat(r)), 1)
		}
		agreesBool := func(given, read *bool) bool { return given == nil || samePtr(given, read) }
		agreesString := func(given, read *string) bool { return given == nil || samePtr(given, read) }
		if !(near(n.Latitude, read.Latitude) && near(n.Longitude, read.Longitude) && agreesBool(n.FixValid, read.FixValid) &&
			near(n.CourseDegrees, read.CourseDegrees) && near(n.SpeedKnots, read.SpeedKnots) && near(n.AltitudeM, read.AltitudeM) &&
			agreesString(n.Time, read.Time) && agreesString(n.Waypoint, read.Waypoint)) {
			p.note(newEncodeError(RefusalInvalidNMEA, "a field given does not match what the NMEA sentence says"))
		}
	}
	if err := p.finish(); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(sentence)+len(n.Comment))
	out = append(out, '$')
	out = append(out, sentence...)
	return append(out, n.Comment...), nil
}

// thirdPartyHeader checks that a third-party packet's header can be written so that it reads
// back the same: the source 1-9 printable ASCII characters other than > and : (APRS12c ch. 17),
// the destination and path APRS-IS addresses, the used entries a run from the start (the TNC2
// form marks only the last), and no header defect that the inner decode tolerated: that is part of
// its data, and no clean form reproduces it.
func thirdPartyHeader(p *Packet) *EncodeError {
	if !isThirdPartySource(p.Source.text) {
		return newEncodeError(RefusalInvalidThirdPartyHeader, "a third-party source is 1-9 printable ASCII characters other than > and : (APRS12c ch. 17)")
	}
	pathOK := true
	for _, e := range p.Path {
		pathOK = pathOK && isValidAddress(e.Address.text)
	}
	if !isValidAddress(p.Destination.text) || !pathOK {
		return newEncodeError(RefusalInvalidThirdPartyHeader, "a third-party destination and path entries are addresses: 1-9 letters, digits or -")
	}
	last := -1
	for i, e := range p.Path {
		if e.Used {
			last = i
		}
	}
	for i := 0; i < last; i++ {
		if !p.Path[i].Used {
			return newEncodeError(RefusalInvalidThirdPartyHeader, "a used path entry after one that is not used cannot be written in TNC2 form")
		}
	}
	for _, d := range p.Diagnostics {
		if isHeaderDefect(d.Code) {
			return newEncodeError(RefusalInvalidThirdPartyHeader, "the third-party packet's header has a defect that was tolerated, which is part of its data and cannot be written back")
		}
	}
	return nil
}

func isHeaderDefect(c Code) bool {
	switch c {
	case CodeEmptyDestination, CodeEmptyPathEntry, CodeMultipleUsedMarkers, CodeNulPaddedAddress, CodeInvalidAX25AddressCharacters:
		return true
	}
	return false
}
