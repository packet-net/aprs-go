package aprs

// Status reports (APRS12c ch. 16): text, optionally a DHM zulu timestamp, a Maidenhead grid
// locator with a symbol, and a meteor-scatter beam heading and power.

func decodeStatus(ctx *context, info []byte) Data {
	rest := info[1:]
	at := 1
	status := Status{}

	var t Timestamp
	tsOK := false
	if b, ok := get(rest, 0, 7); ok {
		t, tsOK = ParseTimestamp(b)
		tsOK = tsOK && t.Kind == DayHourMinute && t.UTC
	}
	if tsOK {
		if !t.IsValid() && !ctx.tolerate(CodeInvalidTimestamp, "the timestamp is out of range (APRS12c ch. 6)", at) {
			return nil
		}
		status.Timestamp = &t
		rest = rest[7:]
		at += 7
	} else if n, symbol, ok := grid(rest); ok {
		status.Locator = Ptr(asciiUpper(lossy(rest[:n])))
		status.Symbol = &symbol
		rest = rest[n+2:]
		at += n + 2
		if len(rest) > 0 {
			if rest[0] == ' ' {
				rest = rest[1:]
				at++
			} else if !ctx.tolerate(CodeMissingSpaceAfterLocator, "status text straight after the grid locator and symbol; a space comes first (UAP 5.17)", at) {
				return nil
			}
		}
	}

	body := rest
	if n := len(rest); n >= 3 && rest[n-3] == '^' {
		h, p := rest[n-2], rest[n-1]
		if (isDigit(h) || isUpper(h)) && p >= '1' && p <= 'K' && !(p >= 'L' && p <= 'Z') {
			status.Beam = &BeamHeading{HeadingCode: rune(h), PowerCode: rune(p)}
			body = rest[:n-3]
		}
	}
	text, ok := decodeText(ctx, body, at)
	if !ok {
		return nil
	}
	status.Text = text
	return status
}

// grid reads a 6- or 4-character locator followed by a symbol. A bare 6-character locator
// followed by text is plain text; the 4-character reading is tried only when the first six
// characters are not a locator (interpretations.md).
func grid(rest []byte) (int, Symbol, bool) {
	symbolAt := func(i int) (Symbol, bool) {
		if i+1 >= len(rest) {
			return Symbol{}, false
		}
		return decodeSymbol(newContext(Strict), rest[i], rest[i+1], 0, 0, false)
	}
	if len(rest) < 6 {
		return 0, Symbol{}, false
	}
	if isLocator(rest[:6]) {
		s, ok := symbolAt(6)
		return 6, s, ok
	}
	if isLocator(rest[:4]) {
		s, ok := symbolAt(4)
		return 4, s, ok
	}
	return 0, Symbol{}, false
}

func isLocator(b []byte) bool {
	field := func(c byte) bool { u := toUpper(c); return u >= 'A' && u <= 'R' }
	sub := func(c byte) bool { u := toUpper(c); return u >= 'A' && u <= 'X' }
	switch len(b) {
	case 4:
		return field(b[0]) && field(b[1]) && isDigit(b[2]) && isDigit(b[3])
	case 6:
		return field(b[0]) && field(b[1]) && isDigit(b[2]) && isDigit(b[3]) && sub(b[4]) && sub(b[5])
	}
	return false
}

func encodeStatus(s *Status) ([]byte, *EncodeError) {
	out := []byte{'>'}
	p := &pending{}
	if s.Timestamp != nil {
		t := *s.Timestamp
		if !(t.Kind == DayHourMinute && t.UTC) {
			p.note(newEncodeError(RefusalInvalidTimestamp, "a status timestamp is DHM zulu, DDHHMMz (APRS12c ch. 16)"))
		} else {
			var err *EncodeError
			out, err = dhmOrHMS(out, t)
			p.run(err)
		}
		if s.Locator != nil {
			p.note(newEncodeError(RefusalFieldNotAllowed, "a status report with a grid locator cannot have a timestamp (APRS12c ch. 16)"))
		}
	}
	if s.Locator != nil && !(isLocator([]byte(*s.Locator)) && *s.Locator == asciiUpper(*s.Locator)) {
		// Fixed-form text is malformed whatever else is wrong; an encoder writes a locator in
		// upper case as given and does not fix its case.
		p.note(newEncodeError(RefusalMalformed, "a grid locator is 4 or 6 upper-case characters, e.g. IO91 or IO91SX"))
	}
	switch {
	case s.Locator != nil && s.Symbol != nil:
		p.run(checkSymbol(*s.Symbol))
		out = append(out, asciiUpper(*s.Locator)...)
		out = append(out, byte(s.Symbol.Table), byte(s.Symbol.Code))
		if s.Text != "" || s.Beam != nil {
			out = append(out, ' ')
		}
	case s.Locator == nil && s.Symbol == nil:
	default:
		p.note(newEncodeError(RefusalMissingField, "a status grid locator and its symbol go together"))
	}
	if hasLineBreak([]byte(s.Text)) {
		p.note(newEncodeError(RefusalInvalidCharacter, "status text cannot contain a line break"))
	}
	out = append(out, s.Text...)
	if b := s.Beam; b != nil {
		h, pw := byte(b.HeadingCode), byte(b.PowerCode)
		if !(isDigit(h) || isUpper(h)) || !(pw >= '1' && pw <= 'K') || (pw >= 'L' && pw <= 'Z') {
			p.note(newEncodeError(RefusalInvalidCode, "a beam heading is 0-9 or A-Z, and the power code 1-9 or : to K (APRS12c ch. 16)"))
		}
		out = append(out, '^', h, pw)
	}
	if err := p.finish(); err != nil {
		return nil, err
	}
	// The text must read back as itself: status text that looks like a timestamp or a grid
	// locator would not.
	if back, ok := decodeStatus(newContext(Lenient), out).(Status); ok {
		upper := func(s *string) *string {
			if s == nil {
				return nil
			}
			return Ptr(asciiUpper(*s))
		}
		if sameStatus(&back, s) || (samePtr(upper(back.Locator), upper(s.Locator)) && back.Text == s.Text && samePtr(back.Beam, s.Beam) && sameTimestamp(back.Timestamp, s.Timestamp)) {
			return out, nil
		}
	}
	return nil, newEncodeError(RefusalTextReadsAsField, "status text that would read back as a timestamp, grid locator or beam heading")
}

func sameStatus(a, b *Status) bool {
	return sameTimestamp(a.Timestamp, b.Timestamp) && samePtr(a.Locator, b.Locator) && samePtr(a.Symbol, b.Symbol) && samePtr(a.Beam, b.Beam) && a.Text == b.Text
}
