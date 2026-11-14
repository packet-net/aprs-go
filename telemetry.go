package aprs

import "unicode/utf8"

// Telemetry reports, T#sss,111,222,333,444,555,xxxxxxxx (APRS12c ch. 13).

func decodeTelemetry(ctx *context, info []byte) Data {
	if !hasPrefix(info, "T#") {
		ctx.error(CodeInvalidTelemetry, "a telemetry report starts T# (APRS12c ch. 13)", 0)
		return nil
	}
	body := info[2:]
	// The structure is ASCII and is checked first; only the comment after the bits is text. "MIC
	// may or may not be followed by a comma"; any other sequence is letters or digits, of any
	// length (real reports count past 999), and ends at a comma.
	var sequence []byte
	var at int
	if hasPrefix(body, "MIC") {
		sequence = body[:3]
		at = 3
		if len(body) > 3 && body[3] == ',' {
			at = 4
		}
	} else {
		end := indexByte(body, ',')
		if end <= 0 {
			ctx.error(CodeInvalidTelemetry, "the telemetry sequence number is not followed by ','", 2)
			return nil
		}
		for _, c := range body[:end] {
			if !isAlnum(c) {
				ctx.error(CodeInvalidTelemetry, "the telemetry sequence number is letters or digits", 2)
				return nil
			}
		}
		sequence = body[:end]
		at = end + 1
	}
	t := Telemetry{Sequence: lossy(sequence)}
	// Up to five values, each ending at a comma or the end; an empty value is missing, and one
	// left empty by a trailing comma still counts.
	for len(t.Analog) < 5 {
		end := len(body)
		if i := indexByte(body[at:], ','); i >= 0 {
			end = at + i
		}
		v := body[at:end]
		switch {
		case !utf8.Valid(v):
			ctx.error(CodeInvalidTelemetry, "a telemetry value is not a number (APRS12c ch. 13)", 2+at)
			return nil
		case len(v) == 0:
			t.Analog = append(t.Analog, nil)
			t.AnalogText = append(t.AnalogText, "")
		case isDecimal(string(v)):
			n, ok := parseFloat(string(v))
			if !ok {
				n = 0
			}
			t.Analog = append(t.Analog, Ptr(n))
			t.AnalogText = append(t.AnalogText, string(v))
		default:
			ctx.error(CodeInvalidTelemetry, "a telemetry value is not a number (APRS12c ch. 13)", 2+at)
			return nil
		}
		at = end
		if at >= len(body) {
			break
		}
		at++
	}
	// The 8 bits only after all five values; anything after them is the comment.
	if len(t.Analog) == 5 && len(body) >= at+8 && onlyBits(string(body[at:at+8])) {
		t.Bits = Ptr(string(body[at : at+8]))
		at += 8
	}
	if (len(t.Analog) < 5 || t.Bits == nil) && !ctx.tolerate(CodeInvalidTelemetry, "a telemetry report carries a sequence, 5 analog values and the 8 digital bits (APRS12c ch. 13)", 2) {
		return nil
	}
	comment, ok := decodeText(ctx, from(body, at), 2+at)
	if !ok {
		return nil
	}
	t.Comment = comment
	return t
}

func encodeTelemetry(t *Telemetry) ([]byte, *EncodeError) {
	p := &pending{}
	// The sequence is fixed-form text: letters and digits, or MIC.
	// A sequence left out (the empty string, as for any absent text) is missing-field.
	seqOK := true
	for i := 0; i < len(t.Sequence); i++ {
		seqOK = seqOK && isAlnum(t.Sequence[i])
	}
	switch {
	case t.Sequence == "":
		p.note(newEncodeError(RefusalMissingField, "a telemetry report needs a sequence (APRS12c ch. 13)"))
	case !seqOK:
		p.note(newEncodeError(RefusalMalformed, "a telemetry sequence is letters and digits"))
	case len(t.Sequence) > 3 && t.Sequence[:3] == "MIC":
		p.note(newEncodeError(RefusalMalformed, "a telemetry sequence that starts MIC is MIC itself (APRS12c ch. 13)"))
	}
	if len(t.Analog) > 5 {
		p.note(newEncodeError(RefusalTooLong, "a telemetry report carries 5 analog values (APRS12c ch. 13)"))
	} else if len(t.Analog) != 5 {
		p.note(newEncodeError(RefusalMalformed, "a telemetry report carries 5 analog values (APRS12c ch. 13)"))
	}
	switch {
	case t.Bits == nil:
		p.note(newEncodeError(RefusalMissingField, "a telemetry report carries the 8 digital bits (APRS12c ch. 13)"))
	case len(*t.Bits) != 8 || !onlyBits(*t.Bits):
		p.note(newEncodeError(RefusalMalformed, "telemetry bits are eight 0 or 1 characters (APRS12c ch. 13)"))
	}
	// No form reads back with the first channel empty after MIC (vectors README, Encoding).
	if t.Sequence == "MIC" && len(t.Analog) > 0 && t.Analog[0] == nil {
		p.note(newEncodeError(RefusalValueHasNoForm, "T#MIC with an empty first channel does not read back as given, so it cannot be sent"))
	}
	if hasControl(t.Comment) {
		p.note(newEncodeError(RefusalInvalidCharacter, "telemetry comment text cannot contain a control character"))
	}
	out := []byte("T#")
	out = append(out, t.Sequence...)
	// MIC may or may not be followed by a comma; it is written without (APRS12c ch. 13).
	if t.Sequence != "MIC" {
		out = append(out, ',')
	}
	for i, v := range t.Analog {
		if v != nil {
			var text *string
			if i < len(t.AnalogText) && t.AnalogText[i] != "" {
				text = &t.AnalogText[i]
			}
			if written, err := telemetryValue(*v, text); p.run(err) {
				out = append(out, written...)
			}
		}
		out = append(out, ',')
	}
	if t.Bits != nil {
		out = append(out, *t.Bits...)
	}
	out = append(out, t.Comment...)
	if err := p.finish(); err != nil {
		return nil, err
	}
	// The read-back is the safety net (a sequence starting MIC is refused as malformed above).
	if back, ok := decodeTelemetry(newContext(Lenient), out).(Telemetry); ok &&
		back.Sequence == t.Sequence && samePtr(back.Bits, t.Bits) && back.Comment == t.Comment && len(back.Analog) == len(t.Analog) {
		same := true
		for i := range back.Analog {
			x, y := back.Analog[i], t.Analog[i]
			if (x == nil) != (y == nil) || (x != nil && *x != *y) {
				same = false
			}
		}
		if same {
			return out, nil
		}
	}
	return nil, newEncodeError(RefusalTextReadsAsField, "the telemetry report would not read back as given")
}
