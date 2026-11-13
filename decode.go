package aprs

// beaconPositionLimit is how far into the information field a ! may start an obsolete TNC beacon
// position.
const beaconPositionLimit = 40

// decodeInformation chooses a decoder by the data type identifier (APRS12c ch. 5).
func decodeInformation(ctx *context, destination Address, info []byte) Data {
	end := len(info)
	for end > 0 && (info[end-1] == '\r' || info[end-1] == '\n') {
		end--
	}
	if end < len(info) && !ctx.tolerate(CodeTrailingLineBreak, "the information field ends with CR or LF (APRS12c ch. 5, UAP 5.13)", end) {
		return UnrecognizedMalformed
	}
	info = info[:end]
	if len(info) == 0 {
		return UnrecognizedEmpty
	}

	var data Data
	switch dti := info[0]; {
	case dti == '!' && hasPrefix(info, "!!"):
		data = decodeRawWeather(ctx, info, UltimeterLogging, 2)
	case dti == '!' || dti == '=' || dti == '/' || dti == '@':
		data = decodePositionReport(ctx, info, 0)
	case dti == '`' || dti == '\'':
		data = decodeMicE(ctx, destination, info)
	case dti == 0x1C || dti == 0x1D:
		// Given at the data type identifier, before anything else is checked (vectors README).
		ctx.info(CodeObsoleteFormat, "the Mic-E Rev 0 data type identifiers 0x1C and 0x1D are obsolete (APRS12c ch. 10)", 0)
		data = decodeMicE(ctx, destination, info)
	case dti == ';':
		data = decodeObject(ctx, info)
	case dti == ')':
		data = decodeItem(ctx, info)
	case dti == ':':
		data = decodeMessage(ctx, info)
	case dti == '>':
		data = decodeStatus(ctx, info)
	case dti == 'T':
		data = decodeTelemetry(ctx, info)
	case dti == '_':
		data = decodePositionlessWeather(ctx, info)
	case dti == '#':
		data = decodeRawWeather(ctx, info, PeetBrosHash, 1)
	case dti == '*':
		data = decodeRawWeather(ctx, info, PeetBrosStar, 1)
	case dti == '$' && hasPrefix(info, "$ULTW"):
		data = decodeRawWeather(ctx, info, UltimeterPacket, 5)
	case dti == '$':
		data = decodeNMEA(ctx, info)
	case dti == '[':
		data = decodeMaidenheadBeacon(ctx, info)
	case dti == '?':
		data = decodeQuery(ctx, info)
	case dti == '<':
		data = decodeCapabilities(ctx, info)
	case dti == '}':
		data = decodeThirdParty(ctx, info)
	case dti == '{':
		data = decodeUserDefined(ctx, info)
	case dti == ',':
		data = decodeTestData(ctx, info)
	case dti == '%':
		data = decodeAgrelo(ctx, info)
	case dti == '&' || dti == '+' || dti == '.':
		ctx.info(CodeReservedDataType, "a reserved data type identifier with no defined format (APRS12c ch. 5)", 0)
		return UnrecognizedReservedDataType
	default:
		return notAprs(ctx, info)
	}
	if data == nil {
		return UnrecognizedMalformed
	}
	return data
}

// notAprs: not APRS by its first byte. An obsolete TNC beacon may still carry a ! position
// further in.
func notAprs(ctx *context, info []byte) Data {
	limit := min(len(info), beaconPositionLimit)
	if i := indexByte(info[:limit], '!'); i >= 0 && ctx.options.Tolerates(CodePositionNotAtStart) {
		// In reading order: the position's own diagnostics come after the one that finds it.
		trial := ctx.clone()
		trial.warn(CodePositionNotAtStart, "a ! position found after other text (obsolete TNC beacon rule)", i)
		if data := decodePositionReport(trial, info[i:], i); data != nil {
			*ctx = *trial
			return data
		}
	}
	ctx.info(CodeNotAprs, "the first byte is not an APRS data type identifier (APRS12c ch. 20)", 0)
	return UnrecognizedNotAprs
}

// decodePositionReport reads ! and = (no timestamp), / and @ (timestamp); = and @ mean the
// sender can message. Nil when the report cannot be decoded.
func decodePositionReport(ctx *context, info []byte, offset int) Data {
	dti := info[0]
	messaging := dti == '=' || dti == '@'
	at := 1
	var timestamp *Timestamp
	if dti == '/' || dti == '@' {
		b, ok := get(info, 1, 8)
		var t Timestamp
		if ok {
			t, ok = ParseTimestamp(b)
		}
		if !ok {
			return badTimestamp(ctx, info, offset, messaging)
		}
		if !t.IsValid() && !ctx.tolerate(CodeInvalidTimestamp, "the timestamp is out of range (APRS12c ch. 6)", offset+1) {
			return nil
		}
		timestamp = &t
		at = 8
	}
	fields, ok := decodeBody(ctx, info, at, offset)
	if !ok {
		return nil
	}
	return PositionReport{Timestamp: timestamp, Messaging: messaging, Fields: fields}
}

// badTimestamp handles a / or @ report whose timestamp is missing or garbled, a tolerated defect
// (UAP 5.8). A position straight after the DTI (the timestamp is missing) is tried first: a
// timestamp starts with six digits, so it cannot be mistaken for one. Then seven garbled bytes are
// skipped. Where the position is is judged on the position itself, under the options in force,
// not on anything after it (vectors README, "Timestamps that are not there").
func badTimestamp(ctx *context, info []byte, offset int, messaging bool) Data {
	for _, at := range []int{1, 8} {
		probe := newContext(ctx.options)
		if _, ok := decodePosition(probe, from(info, at), offset+at); !ok {
			continue
		}
		why := "the timestamp is not 6 digits then z, / or h (APRS12c ch. 6); skipped (UAP 5.8)"
		if at == 1 {
			why = "the timestamp is missing: the position follows the data type identifier (UAP 5.8)"
		}
		if !ctx.tolerate(CodeMalformedTimestamp, why, offset+1) {
			return nil
		}
		fields, ok := decodeBody(ctx, info, at, offset)
		if !ok {
			return nil
		}
		return PositionReport{Messaging: messaging, Fields: fields}
	}
	ctx.error(CodeMalformedTimestamp, "the timestamp is not timestamp-shaped and no position follows it (UAP 5.8)", offset+1)
	return nil
}

// decodeBody reads the position body shared by position reports, objects and items: the position
// and symbol, then the data extension, weather and comment. offset is where info starts in the
// field.
func decodeBody(ctx *context, info []byte, at, offset int) (Positioned, bool) {
	rest := from(info, at)
	decoded, ok := decodePosition(ctx, rest, offset+at)
	if !ok {
		return Positioned{}, false
	}
	fields := Positioned{Position: decoded.position, Symbol: decoded.symbol, Compressed: decoded.compressed}
	after := at + decoded.length
	if !decodeComment(ctx, &fields, decoded.cs, info[after:], offset+after) {
		return Positioned{}, false
	}
	return fields, true
}
