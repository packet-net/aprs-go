package aprs

// Objects and items (APRS12c ch. 11).

// decodeObject reads ; + name (9, space-padded) + * alive or _ killed + timestamp + position.
func decodeObject(ctx *context, info []byte) Data {
	if len(info) < 11 {
		ctx.error(CodeTruncated, "an object report is shorter than ; and a 9-character name and * or _", 0)
		return nil
	}
	marker := func(b byte) bool { return b == '*' || b == '_' }
	var nameEnd int
	if marker(info[10]) {
		nameEnd = 10
	} else {
		// A name that was not padded to 9 characters: the marker comes early.
		i := -1
		for j := 1; j < 10; j++ {
			if marker(info[j]) {
				i = j - 1
				break
			}
		}
		if i < 0 {
			ctx.error(CodeInvalidObjectName, "no * or _ after the 9-character object name", 10)
			return nil
		}
		if !ctx.tolerate(CodeObjectNameNotPadded, "the object name is not padded to 9 characters", 1) {
			return nil
		}
		nameEnd = i + 1
	}
	if !isPrintableASCII(info[1:nameEnd]) {
		ctx.error(CodeInvalidObjectName, "the object name is not printable ASCII", 1)
		return nil
	}
	nameBytes := trimEndSpaces(info[1:nameEnd])
	if len(nameBytes) == 0 {
		ctx.error(CodeInvalidObjectName, "the object name is empty", 1)
		return nil
	}
	name := string(nameBytes)
	killed := info[nameEnd] == '_'

	at := nameEnd + 1
	var timestamp *Timestamp
	b, ok := get(info, at, at+7)
	var t Timestamp
	if ok {
		t, ok = ParseTimestamp(b)
	}
	switch {
	case ok:
		if !t.IsValid() && !ctx.tolerate(CodeInvalidTimestamp, "the timestamp is out of range (APRS12c ch. 6)", at) {
			return nil
		}
		timestamp = &t
		at += 7
	case garbledTimestamp(ctx, info, at):
		// Timestamp-shaped (six digits, or a z, / or h suffix) but not a timestamp, then a
		// position: garbled, not missing. Read straight after the marker instead, those seven
		// bytes would decode as a compressed position.
		if !ctx.tolerate(CodeMalformedTimestamp, "the timestamp is not 6 digits then z, / or h (APRS12c ch. 6); skipped", at) {
			return nil
		}
		at += 7
	default:
		if !ctx.tolerate(CodeObjectWithoutTimestamp, "an object report has no timestamp (APRS12c ch. 11)", at) {
			return nil
		}
	}

	fields, ok := decodeBody(ctx, info, at, 0)
	if !ok {
		return nil
	}
	return ObjectReport{Name: name, Killed: killed, Timestamp: timestamp, Fields: fields}
}

// garbledTimestamp: seven timestamp-shaped bytes with a position after them, judged on the
// position itself (latitude, table, longitude and symbol, or the 13 compressed bytes) under the
// options in force: a defect later in the report does not change what these bytes are.
func garbledTimestamp(ctx *context, info []byte, at int) bool {
	if len(info) <= at+7 {
		return false
	}
	if !allDigits(info[at:at+6]) && info[at+6] != 'z' && info[at+6] != '/' && info[at+6] != 'h' {
		return false
	}
	_, ok := decodePosition(newContext(ctx.options), info[at+7:], at+7)
	return ok
}

// decodeItem reads ) + name (3-9) + ! alive or _ killed + position.
func decodeItem(ctx *context, info []byte) Data {
	// The marker ends a name of at least 3 characters, so it is looked for from the fourth byte on.
	end := -1
	for i := 4; i < min(len(info), 11); i++ {
		if info[i] == '!' || info[i] == '_' {
			end = i
			break
		}
	}
	if end < 0 {
		ctx.error(CodeInvalidItemName, "no ! or _ after a 3-9 character item name (APRS12c ch. 11)", 1)
		return nil
	}
	nameBytes := info[1:end]
	if !isPrintableASCII(nameBytes) {
		ctx.error(CodeInvalidItemName, "the item name is not printable ASCII", 1)
		return nil
	}
	name := string(nameBytes)
	killed := info[end] == '_'
	fields, ok := decodeBody(ctx, info, end+1, 0)
	if !ok {
		return nil
	}
	return ItemReport{Name: name, Killed: killed, Fields: fields}
}

func trimEndSpaces(b []byte) []byte {
	end := len(b)
	for end > 0 && b[end-1] == ' ' {
		end--
	}
	return b[:end]
}
