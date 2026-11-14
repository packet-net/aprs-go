package aprs

import (
	"fmt"
	"math"
)

// windMode says how the wind is read from the weather fields.
type windMode uint8

const (
	// windPositionless: a positionless report: c and s are the wind, wherever they come, until
	// each is read.
	windPositionless windMode = iota
	// windKnown: already known, from the DDD/SSS extension or the compressed cs bytes: c ends the
	// fields and s is snowfall.
	windKnown
	// windAsFields: a positioned report without the extension: a c field, wherever it comes, is
	// the wind direction, and once it is read an s is the wind speed until that is read
	// (wind-fields-instead-of-extension).
	windAsFields
)

// weatherRun is what a run of weather fields held.
type weatherRun struct {
	// length is the number of bytes used.
	length int
	// direction: a wind direction was read from a c field.
	direction bool
	// speed: a wind speed was read from an s field.
	speed bool
}

// weatherFields reads letter-and-value weather fields from the start of b into w; false when a
// defect was not tolerated. Which field a letter is depends on what has been read (vectors
// interpretations.md, "Which weather field a letter is"): c is the wind direction until the wind
// is known, but only with a value after it; when the wind comes as fields, s is the wind speed
// until the speed is known, and snowfall otherwise; L and l are one field, luminosity. The run
// stops at the first thing that is not a field, at a defined field already read, and at c once
// the wind direction is known. Extra fields are kept as a list, so a repeated extra letter does
// not stop it.
func weatherFields(ctx *context, b []byte, offset int, w *Weather, wind windMode) (weatherRun, bool) {
	at := 0
	// Fields read so far, by the field rather than the letter: snowfall is S, to keep it apart
	// from the wind speed s, and both luminosity letters are L.
	var seen []byte
	var extra []WeatherField
	known := wind == windKnown
	directionKnown, speedKnown := known, known
	run := weatherRun{}
	for at < len(b) {
		letter := b[at]
		if letter == 'c' && directionKnown {
			break
		}
		// In a positioned report the wind comes as fields once a c field is read; an s before
		// that is snowfall (vectors differential/weather-snowfall-keeps-its-width).
		snow := letter == 's' && (speedKnown || (wind == windAsFields && !run.direction))
		width := 0
		switch letter {
		case 'c', 's', 'g', 't', 'r', 'p', 'P', 'L', 'l', '#':
			width = 3
		case 'h':
			width = 2
		case 'b':
			width = 5
		}
		key := letter
		if letter == 's' && snow {
			key = 'S'
		} else if letter == 'l' {
			key = 'L'
		}
		if width > 0 && contains(seen, key) {
			break
		}
		// A fixed-width value that runs on by two or more digits, to the end of the information
		// field or into a defined field that is still to be read, is out of range and dropped; the
		// fields carry on after the run (vectors interpretations.md, "A weather field that runs
		// on by many digits"). Snowfall keeps its width, so it is not looked at here.
		if width > 0 && !snow {
			if n := runOnLength(b, at, width, letter); n > 0 {
				end := at + 1 + n
				if end == len(b) || startsReadableField(b, end, wind, seen, run, directionKnown, speedKnown) {
					if !ctx.tolerate(CodeOutOfRangeValue, "a weather value that runs on by two or more digits was dropped", offset+at) {
						return weatherRun{}, false
					}
					seen = append(seen, key)
					switch {
					case letter == 'c':
						directionKnown, run.direction = true, true
					case letter == 's':
						speedKnown, run.speed = true, true
					}
					at = end
					continue
				}
			}
		}
		var v *float64
		n := 0
		found := false
		if width > 0 {
			var ok bool
			v, n, found, ok = weatherValue(ctx, b, at, offset, width, letter, snow)
			if !ok {
				return weatherRun{}, false
			}
		}
		if found {
			seen = append(seen, key)
			if !assignWeather(ctx, w, letter, snow, v, offset+at) {
				return weatherRun{}, false
			}
			switch {
			case letter == 'c':
				directionKnown, run.direction = true, true
			case letter == 's' && !snow:
				speedKnown, run.speed = true, true
			}
			at += 1 + n
			continue
		}
		// A letter the spec does not define with a number after it: kept as it came. At least
		// two characters (as every defined field has more), ending in a digit.
		if isAlpha(letter) && !isWeatherLetter(letter) {
			l := 0
			for _, c := range b[at+1:] {
				if !isDigit(c) && c != '.' && c != '-' {
					break
				}
				l++
			}
			if l >= 2 && isDigit(b[at+l]) {
				extra = append(extra, WeatherField{Letter: rune(letter), Value: string(b[at+1 : at+1+l])})
				at += 1 + l
				continue
			}
		}
		break
	}
	if len(extra) > 0 {
		w.Extra = extra
	}
	positionless := wind == windPositionless
	var complete bool
	if positionless {
		complete = contains(seen, 'c') && contains(seen, 's') && contains(seen, 'g') && contains(seen, 't')
	} else {
		complete = contains(seen, 'g') && contains(seen, 't')
	}
	why := "a weather report has the gust (g) and temperature (t) fields (APRS12c ch. 12)"
	if positionless {
		why = "a positionless weather report starts with the c, s, g and t fields (APRS12c ch. 12)"
	}
	if !complete && !ctx.tolerate(CodeIncompleteWeather, why, offset) {
		return weatherRun{}, false
	}
	run.length = at
	return run, true
}

// runOnLength gives the length of the value after the letter at at when it is digits that run on
// by two or more past the field's width (a negative temperature's minus sign is one of the
// width), else 0.
func runOnLength(b []byte, at, width int, letter byte) int {
	start := at + 1
	if letter == 't' && start < len(b) && b[start] == '-' {
		start++
	}
	run := 0
	for _, c := range b[start:] {
		if !isDigit(c) {
			break
		}
		run++
	}
	n := run + (start - (at + 1))
	if n >= width+2 {
		return n
	}
	return 0
}

// startsReadableField says whether a defined weather field, not already read and with a value,
// starts at b[at], as the run of fields would read it there.
func startsReadableField(b []byte, at int, wind windMode, seen []byte, run weatherRun, directionKnown, speedKnown bool) bool {
	letter := b[at]
	if !isWeatherLetter(letter) || (letter == 'c' && directionKnown) {
		return false
	}
	snow := letter == 's' && (speedKnown || (wind == windAsFields && !run.direction))
	key := letter
	if letter == 's' && snow {
		key = 'S'
	} else if letter == 'l' {
		key = 'L'
	}
	if contains(seen, key) {
		return false
	}
	width := 3
	switch letter {
	case 'h':
		width = 2
	case 'b':
		width = 5
	}
	_, _, found, _ := weatherValue(newContext(Lenient), b, at, 0, width, letter, snow)
	return found
}

func isWeatherLetter(letter byte) bool {
	switch letter {
	case 'c', 's', 'g', 't', 'r', 'p', 'P', 'h', 'b', 'L', 'l', '#':
		return true
	}
	return false
}

// weatherValue reads the value of the field whose letter is at at: its spec width, or
// (tolerated) a run of 1 to width+1 digits or dots ending at a non-digit (t45, h070, b...). A
// spec-width value that runs on into another digit is read at the run's width if that fits. It
// gives the value (nil when unknown), its length, whether there is a field here, and false when a
// defect was not tolerated.
func weatherValue(ctx *context, b []byte, at, offset, width int, letter byte, snow bool) (*float64, int, bool, bool) {
	avail := len(b) - at - 1
	var exact *float64
	exactOK := false
	if avail >= width {
		exact, exactOK = exactWeatherValue(b[at+1:at+1+width], letter, snow)
	}
	followedByDigit := avail > width && isDigit(b[at+1+width])
	// Snowfall keeps its width: a value of three characters that holds a digit is a number, and a
	// digit after it is not a fourth figure (vectors interpretations.md, "Which weather field a
	// letter is").
	if exactOK && (!followedByDigit || snow) {
		return exact, width, true, true
	}
	dots := avail > 0 && b[at+1] == '.'
	minus := letter == 't' && avail > 0 && b[at+1] == '-'
	start := at + 1
	if minus {
		start++
	}
	run := 0
	for _, c := range b[start:] {
		if dots && c != '.' || !dots && !isDigit(c) {
			break
		}
		run++
	}
	// A short run of dots is an unknown snowfall only when no digit follows it: s..6 and s.0.050
	// are not fields (vectors README, Weather).
	if snow && dots && run < width && start+run < len(b) && isDigit(b[start+run]) {
		return nil, 0, false, true
	}
	n := run
	if minus {
		n++
	}
	// A snowfall value (which may have a decimal point) keeps its width unless it is unknown.
	if run >= 1 && n != width && n <= width+1 && (dots || !snow) {
		if !ctx.tolerate(CodeNonStandardWeatherFieldWidth, "a weather field is not its fixed width (APRS12c ch. 12, UAP 5.31)", offset+at) {
			return nil, 0, false, false
		}
		if dots {
			return nil, n, true, true
		}
		v := weatherNumber(b[start : start+run])
		if minus {
			v = -v
		}
		return &v, n, true, true
	}
	if exactOK {
		return exact, width, true, true
	}
	return nil, 0, false, true
}

// exactWeatherValue reads a value of exactly the spec width: all dots or all spaces (unknown),
// digits, a negative temperature, or a snowfall with one decimal point.
func exactWeatherValue(v []byte, letter byte, snow bool) (*float64, bool) {
	allDots, allSpaces, digitsOnly := true, true, true
	pointCount := 0
	pointsOrDigits := true
	for _, c := range v {
		allDots = allDots && c == '.'
		allSpaces = allSpaces && c == ' '
		digitsOnly = digitsOnly && isDigit(c)
		if c == '.' {
			pointCount++
		}
		pointsOrDigits = pointsOrDigits && (c == '.' || isDigit(c))
	}
	if allDots || allSpaces {
		return nil, true
	}
	if digitsOnly {
		n := weatherNumber(v)
		return &n, true
	}
	if letter == 't' && v[0] == '-' {
		rest := v[1:]
		ok := true
		for _, c := range rest {
			ok = ok && isDigit(c)
		}
		if ok {
			n := -weatherNumber(rest)
			return &n, true
		}
	}
	if snow && pointCount == 1 && pointsOrDigits {
		n, ok := parseFloat(string(v))
		if !ok {
			return nil, false
		}
		return &n, true
	}
	return nil, false
}

func assignWeather(ctx *context, w *Weather, letter byte, snow bool, v *float64, offset int) bool {
	scale := func(v *float64, f func(float64) float64) *float64 {
		if v == nil {
			return nil
		}
		return Ptr(f(*v))
	}
	switch {
	case letter == 'c':
		if v != nil && *v > 360 {
			if !ctx.tolerate(CodeOutOfRangeValue, "a wind direction over 360 degrees was dropped", offset) {
				return false
			}
			w.WindDirectionDegrees = nil
		} else {
			w.WindDirectionDegrees = v
		}
	case letter == 's' && !snow:
		w.WindSpeedMph = v
	case letter == 's':
		w.Snow24hIn = v
	case letter == 'g':
		w.WindGustMph = v
	case letter == 't':
		w.TemperatureF = v
	case letter == 'r':
		w.Rain1hIn = scale(v, func(x float64) float64 { return x / 100 })
	case letter == 'p':
		w.Rain24hIn = scale(v, func(x float64) float64 { return x / 100 })
	case letter == 'P':
		w.RainMidnightIn = scale(v, func(x float64) float64 { return x / 100 })
	case letter == 'h':
		if v != nil && *v > 100 {
			if !ctx.tolerate(CodeOutOfRangeValue, "a humidity over 100% was dropped", offset) {
				return false
			}
			w.HumidityPercent = nil
		} else {
			w.HumidityPercent = scale(v, func(h float64) float64 {
				if h == 0 {
					return 100
				}
				return h
			})
		}
	case letter == 'b':
		w.PressureMbar = scale(v, func(x float64) float64 { return x / 10 })
	case letter == 'L':
		w.LuminosityWM2 = v
	case letter == 'l':
		w.LuminosityWM2 = scale(v, func(x float64) float64 { return x + 1000 })
	case letter == '#':
		if v == nil {
			w.RainRaw = nil
		} else {
			w.RainRaw = Ptr(toU32(*v))
		}
	}
	return true
}

func weatherNumber(b []byte) float64 {
	n := 0.0
	for _, c := range b {
		n = n*10 + float64(c-'0')
	}
	return n
}

// isSoftwareAndUnit says whether rest is exactly the software type and unit: a letter and a 2-4
// character unit of letters, digits, - or _, not all digits (wRSW, eMB64, tU2k). A letter
// followed only by digits is a malformed weather field instead (b0990).
func isSoftwareAndUnit(rest []byte) bool {
	if len(rest) < 3 || len(rest) > 5 || !isAlpha(rest[0]) {
		return false
	}
	allDigit := true
	for _, c := range rest[1:] {
		allDigit = allDigit && isDigit(c)
	}
	if allDigit {
		return false
	}
	for _, c := range rest {
		if !isAlnum(c) && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

// softwareAndUnit reads the software type and unit (see isSoftwareAndUnit) into w.
func softwareAndUnit(rest []byte, w *Weather) bool {
	if !isSoftwareAndUnit(rest) {
		return false
	}
	w.Software = Ptr(rune(rest[0]))
	w.Unit = Ptr(string(rest[1:]))
	return true
}

// sameWeatherText compares the parts of weather that are text rather than numbers: the software
// type, the unit and the extra fields. Written back, they must read back the same, and not as
// fields (h89b1 as a software type and unit would read back as humidity).
func sameWeatherText(a, b *Weather) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if !samePtr(a.Software, b.Software) || !samePtr(a.Unit, b.Unit) || len(a.Extra) != len(b.Extra) {
		return false
	}
	for i := range a.Extra {
		if a.Extra[i] != b.Extra[i] {
			return false
		}
	}
	return true
}

// samePtr compares two optional values.
func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// writeWeatherFields writes the fields after the wind (gust, temperature, rain, humidity,
// pressure, luminosity, snow, raw rain, extra fields), then software and unit. Gust and
// temperature are mandatory, written as dots when unknown.
func writeWeatherFields(out []byte, w *Weather, p *pending) []byte {
	var err *EncodeError
	times := func(v *float64, f float64) *float64 {
		if v == nil {
			return nil
		}
		return Ptr(*v * f)
	}
	out, err = weatherField(out, 'g', w.WindGustMph, 3, false)
	p.run(err)
	out, err = weatherField(out, 't', w.TemperatureF, 3, true)
	p.run(err)
	out, err = optionalWeatherField(out, 'r', times(w.Rain1hIn, 100), 3)
	p.run(err)
	out, err = optionalWeatherField(out, 'p', times(w.Rain24hIn, 100), 3)
	p.run(err)
	out, err = optionalWeatherField(out, 'P', times(w.RainMidnightIn, 100), 3)
	p.run(err)
	if w.HumidityPercent != nil {
		// Rounded once to a whole percent: 99.7 is h00, and 0.4, which rounds to 0, is refused
		// since h00 means 100.
		original := *w.HumidityPercent
		h := roundSteps(original)
		if !inRange(h, 1, 100) {
			code := refusalForValue(original)
			if h >= 0 && h < 1 && original >= 0 {
				code = RefusalValueHasNoForm
			}
			p.note(newEncodeError(code, "humidity must round to 1-100 percent"))
		} else {
			out = append(out, 'h')
			out = pushDigits(out, uint64(toU32(h)%100), 2)
		}
	}
	out, err = optionalWeatherField(out, 'b', times(w.PressureMbar, 10), 5)
	p.run(err)
	if w.LuminosityWM2 != nil {
		// Rounded once to a whole number of W/m2.
		given := *w.LuminosityWM2
		l := roundSteps(given)
		switch {
		case inRange(l, 0, 999):
			out = append(out, 'L')
			out = pushDigits(out, uint64(toU32(l)), 3)
		case inRange(l, 1000, 1999):
			out = append(out, 'l')
			out = pushDigits(out, uint64(toU32(l)-1000), 3)
		default:
			p.note(newEncodeError(refusalForValue(given), "luminosity must round to 0-1999 W/m2"))
		}
	}
	if w.Snow24hIn != nil {
		out = append(out, 's')
		out, err = snowfall(out, *w.Snow24hIn)
		p.run(err)
	}
	if w.RainRaw != nil {
		if *w.RainRaw > 999 {
			p.note(newEncodeError(RefusalValueOutOfRange, "the raw rain counter must be 0-999"))
		} else {
			out = append(out, '#')
			out = pushDigits(out, uint64(*w.RainRaw), 3)
		}
	}
	for _, e := range w.Extra {
		// As they are read: a letter the spec does not define, then two or more digits, dots or
		// -, ending in a digit.
		v := []byte(e.Value)
		ok := runeIsASCIIAlpha(e.Letter) && !isWeatherLetter(byte(e.Letter)) && len(v) >= 2 && isDigit(v[len(v)-1])
		for _, c := range v {
			ok = ok && (isDigit(c) || c == '.' || c == '-')
		}
		if !ok {
			p.note(newEncodeError(RefusalTextReadsAsField, "an extra weather field is a letter the spec does not define and a number"))
			continue
		}
		out = append(out, byte(e.Letter))
		out = append(out, e.Value...)
	}
	switch {
	case w.Software != nil && w.Unit != nil && *w.Software >= 0 && *w.Software < 0x80 && isSoftwareAndUnit([]byte(string(*w.Software)+*w.Unit)):
		out = append(out, byte(*w.Software))
		out = append(out, *w.Unit...)
	case w.Software == nil && w.Unit == nil:
	default:
		p.note(newEncodeError(RefusalTextReadsAsField, "weather software is a letter, and the unit 2-4 letters, digits, - or _, not all digits"))
	}
	return out
}

// weatherField writes a mandatory field: digits, or dots when unknown.
func weatherField(out []byte, letter byte, value *float64, width int, signed bool) ([]byte, *EncodeError) {
	out = append(out, letter)
	if value == nil {
		for i := 0; i < width; i++ {
			out = append(out, '.')
		}
		return out, nil
	}
	return numberField(out, *value, width, signed)
}

func optionalWeatherField(out []byte, letter byte, value *float64, width int) ([]byte, *EncodeError) {
	if value == nil {
		return out, nil
	}
	out = append(out, letter)
	return numberField(out, *value, width, false)
}

// numberField writes a value in its field's steps, rounded to the nearest (vectors README,
// Encoding), in width digits.
func numberField(out []byte, value float64, width int, signed bool) ([]byte, *EncodeError) {
	n := roundSteps(value)
	if math.IsNaN(n) {
		return out, newEncodeError(RefusalNotANumber, "a weather value is not a number")
	}
	max := math.Pow(10, float64(width)) - 1
	min := 0.0
	if signed {
		min = -(math.Pow(10, float64(width)-1) - 1)
	}
	if n > max || n < min {
		return out, newEncodeError(refusalForValue(value), "a weather value is outside what its fixed-width field can carry")
	}
	if n < 0 {
		out = append(out, '-')
		return pushDigits(out, uint64(toU32(-n)), width-1), nil
	}
	return pushDigits(out, uint64(toU32(n)), width), nil
}

// snowfall writes snowfall in its three characters, which may include one decimal point
// (APRS12c ch. 12: "A decimal point is allowed for non-integer values"): rounded to hundredths if
// that leaves it under 1, else to tenths if that leaves it under 10, else to a whole number; then
// a whole number as three digits (012, and 0.995 as 001), one under 1 as . and two digits (.50),
// and any other as a digit, . and a digit (1.5). One that rounds past 999 is refused.
func snowfall(out []byte, inches float64) ([]byte, *EncodeError) {
	if math.IsNaN(inches) || roundSteps(inches*100) < 0 {
		return out, newEncodeError(refusalForValue(inches), "snowfall must be 0-999 inches")
	}
	hundredths := toU32(roundSteps(inches * 100))
	tenths := toU32(roundSteps(inches * 10))
	var text string
	switch {
	case hundredths < 100 && hundredths%100 != 0:
		text = fmt.Sprintf(".%02d", hundredths)
	case hundredths < 100:
		text = fmt.Sprintf("%03d", hundredths/100)
	case tenths < 100 && tenths%10 != 0:
		text = fmt.Sprintf("%d.%d", tenths/10, tenths%10)
	case tenths < 100:
		text = fmt.Sprintf("%03d", tenths/10)
	default:
		whole := roundSteps(inches)
		if whole > 999 {
			return out, newEncodeError(refusalForValue(inches), "snowfall must be 0-999 inches")
		}
		text = fmt.Sprintf("%03d", toU32(whole))
	}
	return append(out, text...), nil
}

// pushDigits writes value in at least width digits, zero-padded.
func pushDigits(out []byte, value uint64, width int) []byte {
	return append(out, fmt.Sprintf("%0*d", width, value)...)
}
