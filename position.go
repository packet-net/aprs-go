package aprs

import (
	"fmt"
	"math"
)

// cs is the compressed course/speed, range or altitude bytes, kept raw: what they mean depends on
// the symbol (a weather station sends wind in them) and on the type byte.
type cs struct {
	c, s byte
	t    CompressionType
}

type decodedPosition struct {
	position Position
	symbol   Symbol
	// length is the number of bytes used.
	length     int
	compressed bool
	cs         *cs
}

// canStartPosition says whether a position can start with this byte: a latitude digit
// (uncompressed) or a symbol table identifier (compressed; overlay digits are written as a-j).
func canStartPosition(b byte) bool {
	return isDigit(b) || b == '/' || b == '\\' || isUpper(b) || (b >= 'a' && b <= 'j')
}

// decodePosition reads a position and symbol at the start of b. Errors go to ctx; false when
// there is no usable position.
func decodePosition(ctx *context, b []byte, offset int) (decodedPosition, bool) {
	switch {
	case len(b) == 0:
		ctx.error(CodeTruncated, "the position is missing", offset)
		return decodedPosition{}, false
	case isDigit(b[0]):
		return uncompressedPosition(ctx, b, offset)
	case canStartPosition(b[0]):
		return compressedPosition(ctx, b, offset)
	default:
		ctx.error(CodeInvalidPosition, "a position starts with a latitude digit or a symbol table identifier", offset)
		return decodedPosition{}, false
	}
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func uncompressedPosition(ctx *context, b []byte, offset int) (decodedPosition, bool) {
	if len(b) < 19 {
		ctx.error(CodeTruncated, "an uncompressed position needs 19 bytes: latitude, table, longitude, symbol", offset)
		return decodedPosition{}, false
	}
	// Checked in the order the bytes come: latitude, table, longitude, symbol.
	lat := b[0:8]
	lon := b[9:18]

	// Ambiguity: trailing digits of the latitude replaced by spaces (APRS12c ch. 6).
	latDigits := []int{6, 5, 3, 2}
	ambiguity := 0
	for ambiguity < len(latDigits) && lat[latDigits[ambiguity]] == ' ' {
		ambiguity++
	}
	latIgnored := latDigits[:ambiguity]
	latOK := lat[4] == '.' && isDigit(lat[0]) && isDigit(lat[1])
	for _, i := range []int{2, 3, 5, 6} {
		if !isDigit(lat[i]) && !containsInt(latIgnored, i) {
			latOK = false
		}
	}
	if !latOK {
		ctx.error(CodeInvalidLatitude, "the latitude is not DDMM.hh with N or S (APRS12c ch. 6)", offset)
		return decodedPosition{}, false
	}
	// An ambiguous position is the centre of the area it covers (interpretations.md).
	halfBox := [...]float64{0, 0.05, 0.5, 5, 30}[ambiguity]
	digit := func(s []byte, i int, ignored []int) float64 {
		if containsInt(ignored, i) || s[i] == ' ' {
			return 0
		}
		return float64(s[i] - '0')
	}
	latDeg := digit(lat, 0, nil)*10 + digit(lat, 1, nil)
	latMin := digit(lat, 2, latIgnored)*10 + digit(lat, 3, latIgnored) + digit(lat, 5, latIgnored)/10 + digit(lat, 6, latIgnored)/100 + halfBox
	if latMin >= 60 {
		ctx.error(CodeInvalidLatitude, "the latitude minutes are 60 or more", offset)
		return decodedPosition{}, false
	}
	// The digits and the range come before the hemisphere letter (vectors README, Positions).
	latitude := latDeg + latMin/60
	if latitude > 90 {
		ctx.error(CodeInvalidLatitude, "the latitude is beyond 90 degrees", offset)
		return decodedPosition{}, false
	}
	var north bool
	switch lat[7] {
	case 'N':
		north = true
	case 'S':
		north = false
	case 'n', 's':
		if !ctx.tolerate(CodeLowercaseHemisphere, "a lower-case latitude hemisphere (UAP 5.9)", offset+7) {
			return decodedPosition{}, false
		}
		north = lat[7] == 'n'
	default:
		ctx.error(CodeInvalidLatitude, "the latitude hemisphere is not N or S", offset+7)
		return decodedPosition{}, false
	}

	// The symbol table sits between latitude and longitude.
	if t := b[8]; !(t == '/' || t == '\\' || isDigit(t) || isUpper(t)) {
		ctx.error(CodeInvalidSymbolTable, "the symbol table identifier is not /, \\, 0-9 or A-Z", offset+8)
		return decodedPosition{}, false
	}

	// The latitude's ambiguity applies to the longitude: those digits are ignored, whatever they are.
	lonDigits := []int{7, 6, 4, 3}
	lonIgnored := lonDigits[:ambiguity]
	lonOK := lon[5] == '.' && isDigit(lon[0]) && isDigit(lon[1]) && isDigit(lon[2])
	for _, i := range []int{3, 4, 6, 7} {
		if !isDigit(lon[i]) && !(lon[i] == ' ' && containsInt(lonIgnored, i)) {
			lonOK = false
		}
	}
	if !lonOK {
		ctx.error(CodeInvalidLongitude, "the longitude is not DDDMM.hh with E or W (APRS12c ch. 6)", offset+9)
		return decodedPosition{}, false
	}
	lonDeg := digit(lon, 0, nil)*100 + digit(lon, 1, nil)*10 + digit(lon, 2, nil)
	lonMin := digit(lon, 3, lonIgnored)*10 + digit(lon, 4, lonIgnored) + digit(lon, 6, lonIgnored)/10 + digit(lon, 7, lonIgnored)/100
	if lonMin >= 60 {
		ctx.error(CodeInvalidLongitude, "the longitude minutes are 60 or more", offset+9)
		return decodedPosition{}, false
	}
	longitude := lonDeg + (lonMin+halfBox)/60
	if longitude > 180 {
		ctx.error(CodeInvalidLongitude, "the longitude is beyond 180 degrees", offset+9)
		return decodedPosition{}, false
	}
	var east bool
	switch lon[8] {
	case 'E':
		east = true
	case 'W':
		east = false
	case 'e', 'w':
		if !ctx.tolerate(CodeLowercaseHemisphere, "a lower-case longitude hemisphere (UAP 5.9)", offset+17) {
			return decodedPosition{}, false
		}
		east = lon[8] == 'e'
	default:
		ctx.error(CodeInvalidLongitude, "the longitude hemisphere is not E or W", offset+17)
		return decodedPosition{}, false
	}

	symbol, ok := decodeSymbol(ctx, b[8], b[18], offset+8, offset+18, false)
	if !ok {
		return decodedPosition{}, false
	}
	if !north {
		latitude = -latitude
	}
	if !east {
		longitude = -longitude
	}
	return decodedPosition{
		position: Position{Latitude: latitude, Longitude: longitude, Ambiguity: uint8(ambiguity)},
		symbol:   symbol,
		length:   19,
	}, true
}

func compressedPosition(ctx *context, b []byte, offset int) (decodedPosition, bool) {
	if len(b) < 13 {
		ctx.error(CodeTruncated, "a compressed position needs 13 bytes (APRS12c ch. 9)", offset)
		return decodedPosition{}, false
	}
	y, yok := base91Decode(b[1:5])
	x, xok := base91Decode(b[5:9])
	if !yok || !xok {
		ctx.error(CodeInvalidCompressedPosition, "the compressed latitude or longitude is not base-91", offset+1)
		return decodedPosition{}, false
	}
	latitude := 90 - float64(y)/380926
	longitude := -180 + float64(x)/190463
	if !inRange(latitude, -90, 90) || !inRange(longitude, -180, 180) {
		ctx.error(CodeInvalidCompressedPosition, "the compressed position is out of range", offset+1)
		return decodedPosition{}, false
	}
	symbol, ok := decodeSymbol(ctx, b[0], b[9], offset, offset+9, true)
	if !ok {
		return decodedPosition{}, false
	}

	c, s, t := b[10], b[11], b[12]
	var bytes *cs
	if c != ' ' {
		if !isBase91(c) || !isBase91(s) || !isBase91(t) {
			ctx.error(CodeInvalidCompressedPosition, "the compressed course/speed, range or altitude bytes are not base-91", offset+10)
			return decodedPosition{}, false
		}
		bits := t - 33
		if bits&0xC0 != 0 && !ctx.tolerate(CodeCompressionTypeReservedBits, "the compression type byte sets its unused high bits (APRS12c ch. 9)", offset+12) {
			return decodedPosition{}, false
		}
		bytes = &cs{c: c, s: s, t: typeFromBits(bits)}
	}
	return decodedPosition{position: Position{Latitude: latitude, Longitude: longitude}, symbol: symbol, length: 13, compressed: true, cs: bytes}, true
}

func typeFromBits(bits byte) CompressionType {
	t := CompressionType{Fix: FixOld, Source: CompressionSource((bits >> 3) & 3), Origin: CompressionOrigin(bits & 7)}
	if bits&0x20 != 0 {
		t.Fix = FixCurrent
	}
	return t
}

func typeToBits(t CompressionType) byte {
	var fix byte
	if t.Fix == FixCurrent {
		fix = 0x20
	}
	return fix | (byte(t.Source)&3)<<3 | byte(t.Origin)&7
}

// decodeSymbol reads a symbol from its table and code bytes.
func decodeSymbol(ctx *context, table, code byte, tableOffset, codeOffset int, compressed bool) (Symbol, bool) {
	switch {
	case table == '/' || table == '\\' || isUpper(table):
	case isDigit(table) && !compressed:
	case table >= 'a' && table <= 'j' && compressed:
		table = '0' + (table - 'a')
	default:
		ctx.error(CodeInvalidSymbolTable, "the symbol table identifier is not /, \\, 0-9 or A-Z", tableOffset)
		return Symbol{}, false
	}
	if code < '!' || code > '~' {
		ctx.error(CodeInvalidSymbolCode, "the symbol code is not printable ASCII", codeOffset)
		return Symbol{}, false
	}
	return Symbol{Table: rune(table), Code: rune(code)}, true
}

func checkSymbol(s Symbol) *EncodeError {
	t := s.Table
	if !(t == '/' || t == '\\' || runeIsASCIIDigit(t) || runeIsASCIIUpper(t)) {
		return newEncodeError(RefusalInvalidSymbol, fmt.Sprintf("symbol table '%c' is not /, \\, 0-9 or A-Z", t))
	}
	if s.Code < '!' || s.Code > '~' {
		return newEncodeError(RefusalInvalidSymbol, "the symbol code is not printable ASCII")
	}
	return nil
}

// encodeUncompressed writes an uncompressed position with its symbol. For a !DAO!, it gives the
// extra latitude and longitude digits (thousandths, or base-91 when asked). What is wrong is noted
// in p, and what is written then is thrown away.
func encodeUncompressed(out []byte, position Position, symbol Symbol, dao *bool, p *pending) ([]byte, *[2]byte) {
	p.run(checkSymbol(symbol))
	position = checkedPosition(position, p)
	if position.Ambiguity > 0 && dao != nil {
		p.note(newEncodeError(RefusalFieldNotAllowed, "a !DAO! adds precision to an ambiguous position, which contradicts it"))
	}
	latText, latExtra := coordinate(position.Latitude, 2, position.Ambiguity, dao)
	lonText, lonExtra := coordinate(position.Longitude, 3, position.Ambiguity, dao)
	out = append(out, latText...)
	if math.Signbit(position.Latitude) {
		out = append(out, 'S')
	} else {
		out = append(out, 'N')
	}
	out = append(out, byte(symbol.Table))
	out = append(out, lonText...)
	if math.Signbit(position.Longitude) {
		out = append(out, 'W')
	} else {
		out = append(out, 'E')
	}
	out = append(out, byte(symbol.Code))
	if dao == nil {
		return out, nil
	}
	return out, &[2]byte{latExtra, lonExtra}
}

// checkedPosition gives the position if it is one that can be written, else a harmless one, with
// what is wrong noted.
func checkedPosition(position Position, p *pending) Position {
	fine := true
	for _, c := range []struct {
		value, limit float64
		what         string
	}{
		{position.Latitude, 90, "latitude must be -90 to 90 degrees"},
		{position.Longitude, 180, "longitude must be -180 to 180 degrees"},
	} {
		if !inRange(c.value, -c.limit, c.limit) {
			p.note(newEncodeError(refusalForValue(c.value), c.what))
			fine = false
		}
	}
	if position.Ambiguity > 4 {
		p.note(newEncodeError(RefusalValueOutOfRange, "position ambiguity is 0-4 digits"))
		fine = false
	}
	if fine {
		return position
	}
	return Position{}
}

// daoDigits gives the !DAO! digits for a position: latitude, then longitude, each the digit
// (base91: the base-91 value, 0-90) a DAO of that precision gives the position written
// uncompressed. A decoder does not apply them to a compressed position, so there they are written
// to agree with it: a reader that did apply them lands on the position reported.
func daoDigits(position Position, base91 bool) (byte, byte) {
	_, lat := coordinate(position.Latitude, 2, 0, &base91)
	_, lon := coordinate(position.Longitude, 3, 0, &base91)
	return lat, lon
}

// coordinate gives degrees and minutes to hundredths (DDMM.hh or DDDMM.hh), with ambiguity
// blanks, plus the !DAO! digit for the next place when asked for.
func coordinate(value float64, degreeDigits int, ambiguity uint8, dao *bool) ([]byte, byte) {
	value = math.Abs(value)
	// The value in units of the smallest step written.
	perMinute, extraBase := 100.0, uint64(1)
	if dao != nil {
		if *dao {
			perMinute, extraBase = 9100, 91
		} else {
			perMinute, extraBase = 1000, 10
		}
	}
	// Rounding then blanking keeps the box the position is in, and a decoded position (the centre
	// of its box) writes back the same digits.
	units := toU64(roundSteps(value * 60 * perMinute))
	extra := byte(units % extraBase)
	units /= extraBase
	// units is now hundredths of a minute.
	degrees := units / 6000
	hundredths := units % 6000
	text := []byte(fmt.Sprintf("%0*d%02d.%02d", degreeDigits, degrees, hundredths/100, hundredths%100))
	// Blank the trailing digits: hundredths, tenths, units of minutes, tens of minutes.
	n := len(text)
	blanks := []int{n - 1, n - 2, n - 4, n - 5}
	for _, i := range blanks[:ambiguity] {
		text[i] = ' '
	}
	return text, extra
}

// encodeCompressed writes a compressed position: table, latitude, longitude, symbol code. The cs
// and type bytes follow separately.
func encodeCompressed(out []byte, position Position, symbol Symbol, p *pending) []byte {
	p.run(checkSymbol(symbol))
	if position.Ambiguity != 0 {
		p.note(newEncodeError(RefusalFieldNotAllowed, "a compressed position cannot be ambiguous"))
	}
	if !inRange(position.Latitude, -90, 90) || !inRange(position.Longitude, -180, 180) {
		value := position.Latitude
		if inRange(position.Latitude, -90, 90) {
			value = position.Longitude
		}
		p.note(newEncodeError(refusalForValue(value), "the position is out of range"))
		position = Position{}
	}
	y, x := compressedSteps(position)
	if runeIsASCIIDigit(symbol.Table) {
		out = append(out, 'a'+byte(symbol.Table-'0'))
	} else {
		out = append(out, byte(symbol.Table))
	}
	out = base91Encode(out, y, 4)
	out = base91Encode(out, x, 4)
	return append(out, byte(symbol.Code))
}

// compressedSteps gives the compressed latitude and longitude steps for a position.
func compressedSteps(position Position) (uint32, uint32) {
	// Round, rather than truncate as APRS12c's worked example does (interpretations.md).
	y := toU32(roundSteps(380926 * (90 - position.Latitude)))
	x := toU32(roundSteps(190463 * (180 + position.Longitude)))
	const max = 91*91*91*91 - 1
	return min(y, max), min(x, max)
}

// compressedReported gives the position a compressed report of position reads back as, which its
// !DAO! digits follow.
func compressedReported(position Position) Position {
	y, x := compressedSteps(position)
	return Position{Latitude: 90 - float64(y)/380926, Longitude: -180 + float64(x)/190463}
}
