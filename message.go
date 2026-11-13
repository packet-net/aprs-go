package aprs

import (
	"strings"
)

// Messages, acks and rejects, bulletins, telemetry metadata and directed queries: everything sent
// as :ADDRESSEE:text (APRS12c ch. 13-15).

// maxMessageText is the message text senders may write (APRS12c ch. 14); receivers accept more.
const maxMessageText = 67

var queryTypes = [...]string{"APRSD", "APRSH", "APRSM", "APRSO", "APRSP", "APRSS", "APRST", "PING?"}

func isQueryType(t string) bool {
	for _, q := range queryTypes {
		if q == t {
			return true
		}
	}
	return false
}

func decodeMessage(ctx *context, info []byte) Data {
	var colon int
	if len(info) > 10 && info[10] == ':' {
		colon = 10
	} else {
		// An addressee that was not padded: the second colon comes early, but not straight away.
		i := -1
		for j := 1; j < len(info) && j < 11; j++ {
			if info[j] == ':' {
				i = j - 1
				break
			}
		}
		if i >= 1 {
			if !ctx.tolerate(CodeUnpaddedAddressee, "the addressee is not padded to 9 characters", 1) {
				return nil
			}
			colon = i + 1
		} else {
			ctx.error(CodeInvalidMessage, "a message is :ADDRESSEE:text, the addressee padded to 9 characters (APRS12c ch. 14)", 0)
			return nil
		}
	}
	raw := info[1:colon]
	if !isPrintableASCII(raw) {
		ctx.error(CodeInvalidMessage, "the addressee is not printable ASCII", 1)
		return nil
	}
	end := len(raw)
	for end > 0 && raw[end-1] == ' ' {
		end--
	}
	addresseeBytes := raw[:end]
	if len(addresseeBytes) == 0 {
		ctx.error(CodeInvalidMessage, "the addressee is empty", 1)
		return nil
	}
	if (contains(addresseeBytes, ' ') || contains(addresseeBytes, ':')) && !ctx.tolerate(CodeInvalidAddresseeCharacters, "the addressee contains a space or : (APRS12c ch. 14)", 1) {
		return nil
	}
	addressee := string(addresseeBytes)
	body := info[colon+1:]
	textOffset := colon + 1

	// Acks and rejects: ackNNNNN / rejNNNNN, optionally }RR (a reply-ack).
	if hasPrefix(body, "ack") || hasPrefix(body, "rej") {
		id, replyAck, state := ackID(ctx, body[3:], textOffset+3)
		switch state {
		case ackRejected:
			return nil
		case ackFound:
			if body[0] == 'r' {
				return Reject{Addressee: addressee, RejectedID: id, ReplyAck: replyAck}
			}
			return Ack{Addressee: addressee, AckedID: id, ReplyAck: replyAck}
		}
	}

	// The structure is checked before the text's encoding, so a strict decoder names the first.
	nws, isBulletin, ok := bulletinKind(ctx, addressee)
	if !ok {
		return nil
	}
	if isBulletin {
		textBytes, messageID, _ := splitMessageID(body, false)
		if !braceOK(ctx, textBytes, textOffset) {
			return nil
		}
		text, ok := decodeText(ctx, textBytes, textOffset)
		if !ok {
			return nil
		}
		if nws {
			return NWSBulletin{Addressee: addressee, Text: text, MessageID: messageID}
		}
		return Bulletin{Addressee: addressee, Text: text, MessageID: messageID}
	}
	if len(body) >= 5 && body[4] == '.' {
		if data, isMetadata := telemetryMetadata(ctx, addressee, body, textOffset); isMetadata {
			return data
		}
	}
	if len(body) > 1 && body[0] == '?' {
		if data := directedQuery(ctx, addressee, body, textOffset); data != nil {
			return data
		}
	}
	textBytes, messageID, replyAck := splitMessageID(body, true)
	if !braceOK(ctx, textBytes, textOffset) {
		return nil
	}
	text, ok := decodeText(ctx, textBytes, textOffset)
	if !ok {
		return nil
	}
	return Message{Addressee: addressee, Text: text, MessageID: messageID, ReplyAck: replyAck}
}

// braceOK: message text never contains {, which starts the message ID (APRS12c ch. 14); one left
// in the text is not followed by a valid ID, a tolerated defect.
func braceOK(ctx *context, text []byte, offset int) bool {
	i := indexByte(text, '{')
	if i < 0 {
		return true
	}
	return ctx.tolerate(CodeBraceInMessageText, "message text contains a { that does not start a message ID (APRS12c ch. 14)", offset+i)
}

func isMessageIDBytes(b []byte) bool {
	if len(b) < 1 || len(b) > 5 {
		return false
	}
	for _, c := range b {
		if !isAlnum(c) {
			return false
		}
	}
	return true
}

type ackState uint8

const (
	notAnAck ackState = iota
	ackFound
	ackRejected
)

// ackID reads the ID after ack or rej: 1-5 letters or digits, optionally } and a reply-ack,
// optionally a (tolerated) message ID of its own.
func ackID(ctx *context, rest []byte, offset int) (string, *string, ackState) {
	brace := indexByte(rest, '{')
	core := rest
	if brace >= 0 {
		core = rest[:brace]
	}
	closing := indexByte(core, '}')
	id := core
	var reply []byte
	hasReply := false
	if closing >= 0 {
		id, reply, hasReply = core[:closing], core[closing+1:], true
	}
	if !isMessageIDBytes(id) || (hasReply && len(reply) > 0 && !isMessageIDBytes(reply)) {
		return "", nil, notAnAck
	}
	if brace >= 0 {
		if !isMessageIDBytes(rest[brace+1:]) {
			return "", nil, notAnAck
		}
		if !ctx.tolerate(CodeMessageIDOnAck, "an ack or rej carries a message ID of its own (UAP 5.32)", offset+brace) {
			return "", nil, ackRejected
		}
	}
	var replyAck *string
	if hasReply {
		replyAck = Ptr(string(reply))
	}
	return string(id), replyAck, ackFound
}

// splitMessageID splits text{MM, and for a message also text{MM} (reply-ack capable) or
// text{MM}AA (and acking AA): the text, the ID and the reply-ack.
func splitMessageID(body []byte, replyAck bool) ([]byte, *string, *string) {
	brace := lastIndexByte(body, '{')
	if brace < 0 {
		return body, nil, nil
	}
	tail := body[brace+1:]
	i := indexByte(tail, '}')
	switch {
	case i < 0 && isMessageIDBytes(tail):
		return body[:brace], Ptr(string(tail)), nil
	case i >= 0 && replyAck && isMessageIDBytes(tail[:i]) && (i+1 == len(tail) || isMessageIDBytes(tail[i+1:])):
		return body[:brace], Ptr(string(tail[:i])), Ptr(string(tail[i+1:]))
	}
	return body, nil, nil
}

// bulletinKind says whether the addressee makes this a bulletin: an NWS bulletin, or BLNn, BLNx
// or BLNnGROUP. False in the last place when a defect was not tolerated.
func bulletinKind(ctx *context, addressee string) (nws, bulletin, ok bool) {
	if strings.HasPrefix(addressee, "NWS-") || strings.HasPrefix(addressee, "NWS_") {
		return true, true, true
	}
	if !strings.HasPrefix(addressee, "BLN") {
		return false, false, true
	}
	rest := addressee[3:]
	switch {
	case len(rest) == 1 && (isDigit(rest[0]) || isUpper(rest[0])):
		return false, true, true
	case len(rest) >= 2 && len(rest) <= 6 && isDigit(rest[0]):
		return false, true, true
	case len(rest) >= 2 && len(rest) <= 6 && isUpper(rest[0]):
		if !ctx.tolerate(CodeLetterGroupBulletin, "a group bulletin has a digit before the group name, not a letter (APRS12c ch. 14)", 1) {
			return false, false, false
		}
		return false, true, true
	}
	return false, false, true
}

// telemetryMetadata reads PARM., UNIT., EQNS. and BITS. (APRS12c ch. 13). False when this is not
// metadata (and a malformed list is not: it stays a plain message); nil data when it is, but its
// text has a defect that was not tolerated.
func telemetryMetadata(ctx *context, addressee string, body []byte, offset int) (Data, bool) {
	kind := string(body[:5])
	if kind != "PARM." && kind != "UNIT." && kind != "EQNS." && kind != "BITS." {
		return nil, false
	}
	list, messageID, _ := splitMessageID(body[5:], false)
	// Checked quietly first: if this turns out not to be metadata, the text is reported once, as a
	// plain message.
	s := forDisplay(list)
	var data Data
	switch kind {
	case "PARM.", "UNIT.":
		labels := strings.Split(s, ",")
		if len(labels) > 13 {
			ctx.info(CodeInvalidTelemetryMetadata, "telemetry metadata names at most 13 channels (APRS12c ch. 13)", offset)
			return nil, false
		}
		if kind == "PARM." {
			data = TelemetryNames{Addressee: addressee, Names: labels, MessageID: messageID}
		} else {
			data = TelemetryUnits{Addressee: addressee, Units: labels, MessageID: messageID}
		}
	case "EQNS.":
		// "The list may stop at any field" (APRS12c ch. 13): trailing commas and spaces are the list
		// stopping. Spaces (U+0020 only) around a coefficient are padding.
		var coefficients []float64
		var texts []string
		for _, part := range strings.Split(strings.TrimRight(s, " ,"), ",") {
			part = strings.Trim(part, " ")
			if !isCoefficient(part) {
				ctx.info(CodeInvalidTelemetryMetadata, "a telemetry equation coefficient is not a number (APRS12c ch. 13)", offset+5)
				return nil, false
			}
			v, ok := parseFloat(part)
			if !ok {
				v = 0
			}
			coefficients = append(coefficients, v)
			texts = append(texts, part)
		}
		if len(coefficients) > 15 {
			ctx.info(CodeInvalidTelemetryMetadata, "telemetry equations are up to 15 numbers, a, b and c for each analog channel (APRS12c ch. 13)", offset+5)
			return nil, false
		}
		data = TelemetryCoefficients{Addressee: addressee, Coefficients: coefficients, CoefficientsText: texts, MessageID: messageID}
	default:
		bits, project, _ := strings.Cut(s, ",")
		if len(bits) != 8 || !onlyBits(bits) {
			ctx.info(CodeInvalidTelemetryMetadata, "telemetry bit sense is eight 0 or 1 characters (APRS12c ch. 13)", offset+5)
			return nil, false
		}
		data = TelemetryBits{Addressee: addressee, Bits: bits, Project: project, MessageID: messageID}
	}
	// It is metadata: now its text's defects are reported.
	if !braceOK(ctx, list, offset+5) {
		return nil, true
	}
	if _, ok := decodeText(ctx, list, offset+5); !ok {
		return nil, true
	}
	return data, true
}

func onlyBits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' && s[i] != '1' {
			return false
		}
	}
	return true
}

// isCoefficient: a telemetry equation coefficient is a number as a telemetry value is written
// (isDecimal), optionally with an exponent (e or E, an optional sign, digits), that is a finite
// number (0eN is 0 however large N is; 1e400 is not a coefficient). Vectors interpretations.md,
// "Numbers in telemetry".
func isCoefficient(t string) bool {
	mantissa, exponent, hasExponent := t, "", false
	if i := strings.IndexAny(t, "eE"); i >= 0 {
		mantissa, exponent, hasExponent = t[:i], t[i+1:], true
	}
	if hasExponent {
		e := exponent
		if len(e) > 0 && (e[0] == '-' || e[0] == '+') {
			e = e[1:]
		}
		if e == "" || !allDigits([]byte(e)) {
			return false
		}
	}
	if !isDecimal(mantissa) {
		return false
	}
	// A mantissa of zero is zero whatever the exponent; otherwise the value must be finite.
	zero := true
	for i := 0; i < len(mantissa); i++ {
		if c := mantissa[i]; c != '0' && c != '.' && c != '-' {
			zero = false
		}
	}
	if zero {
		return true
	}
	v, ok := parseFloat(t)
	return ok && isFinite(v)
}

// says reports whether text is a number of the form form allows that still says value.
func says(text string, value float64, form func(string) bool) bool {
	if !form(text) {
		return false
	}
	n, ok := parseFloat(text)
	return ok && n == value
}

// numberAsSent writes a number as the text it was sent as, when that is a number of the form the
// spec allows (form) and still says value, and otherwise as the shortest decimal that reads back
// the same, with no exponent. A number with no such form (infinite, not a number) is refused.
func numberAsSent(value float64, text *string, form func(string) bool, what string) (string, *EncodeError) {
	if text != nil && says(*text, value, form) {
		return *text, nil
	}
	if !isFinite(value) {
		return "", newEncodeError(RefusalNotANumber, what+" is a finite number")
	}
	return numberText(value), nil
}

// telemetryValue writes a telemetry value: the text it was sent as when that still says the
// number, else a whole number from 0 to 999 as three digits, as APRS12c ch. 13 writes aaa (005,
// 190.0 as 190), else the shortest decimal that reads back the same, with no exponent.
func telemetryValue(value float64, text *string) (string, *EncodeError) {
	if text != nil && says(*text, value, isDecimal) {
		return *text, nil
	}
	if inRange(value, 0, 999) && floorF(value) == value {
		return pad3(toU32(value)), nil
	}
	return numberAsSent(value, nil, isDecimal, "a telemetry value")
}

// isDecimal: a telemetry value is an optional -, then digits with an optional decimal point, with
// at least one digit (APRS12c ch. 13); no +, no spaces, nothing else.
func isDecimal(t string) bool {
	t = strings.TrimPrefix(t, "-")
	whole, frac, _ := strings.Cut(t, ".")
	if whole == "" && frac == "" {
		return false
	}
	for i := 0; i < len(whole); i++ {
		if !isDigit(whole[i]) {
			return false
		}
	}
	for i := 0; i < len(frac); i++ {
		if !isDigit(frac[i]) {
			return false
		}
	}
	return true
}

// directedQuery reads a directed query: ? and a query type, optionally the one callsign some types
// ask about (APRS12c ch. 15). A query type is upper-case letters, apart from the spec's own PING?;
// one the spec does not define is still a query, which the recipient ignores. A defined type's
// target comes straight after it, and an undefined type's after one space; one space between the
// type and the target is a separator, and spaces after the target are padding. Nil (with an Info
// for what looked like a query) when it is a plain message.
func directedQuery(ctx *context, addressee string, body []byte, offset int) Data {
	s := latin1(body[1:])
	notAQuery := func(why string) Data {
		ctx.info(CodeInvalidQuery, why, offset)
		return nil
	}
	if strings.ContainsRune(s, '{') {
		return notAQuery("a directed query never has a message ID; read as a message (UAP 5.18)")
	}
	query := func(queryType, rest string) Data {
		target := strings.TrimRight(rest, " ")
		target = strings.TrimPrefix(target, " ")
		if target != "" && !isValidAddress(target) {
			return notAQuery("a query target is one callsign, 1-9 letters, digits or -; read as a message")
		}
		q := DirectedQuery{Addressee: addressee, QueryType: queryType}
		if target != "" {
			q.Target = Ptr(target)
		}
		return q
	}
	for _, t := range queryTypes {
		if strings.HasPrefix(s, t) {
			return query(t, s[len(t):])
		}
		if len(s) >= len(t) && asciiEqualFold(s[:len(t)], t) {
			return notAQuery("query types are upper case; read as a message (UAP 5.18)")
		}
	}
	// A type the spec does not define: upper-case letters, then a space or the end.
	end := 0
	for end < len(s) && isUpper(s[end]) {
		end++
	}
	if end == 0 || (end < len(s) && s[end] != ' ') {
		return nil
	}
	return query(s[:end], s[end:])
}

// asciiEqualFold compares two texts byte for byte, ignoring the case of ASCII letters only.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toUpper(a[i]) != toUpper(b[i]) {
			return false
		}
	}
	return true
}

// queryTypeOK says whether a directed query of this type, written as the encoder writes it, reads
// back as that type: upper-case letters (or PING?), and not one that starts with a type the spec
// defines.
func queryTypeOK(t string) bool {
	if isQueryType(t) {
		return true
	}
	if t == "" {
		return false
	}
	for i := 0; i < len(t); i++ {
		if !isUpper(t[i]) {
			return false
		}
	}
	for _, q := range queryTypes {
		if strings.HasPrefix(t, q) {
			return false
		}
	}
	return true
}

func isBulletinAddressee(a string) bool {
	_, bulletin, ok := bulletinKind(newContext(Lenient), a)
	return ok && bulletin
}

func encodeMessage(data Data) ([]byte, *EncodeError) {
	var out []byte
	p := &pending{}
	switch d := data.(type) {
	case Message:
		out = writeAddressee(out, d.Addressee, p)
		// An addressee that makes a bulletin of it is not the form a message needs.
		if isBulletinAddressee(d.Addressee) {
			p.note(newEncodeError(RefusalInvalidAddressee, "an addressee of NWS-, NWS_ or BLN and a bulletin form makes a bulletin, not a message"))
		}
		out = messageText(out, d.Text, true, p)
		out = idAndReply(out, d.MessageID, d.ReplyAck, p)
	case Ack:
		out = writeAddressee(out, d.Addressee, p)
		out = ackOrReject(out, "ack", d.AckedID, d.MessageID, d.ReplyAck, p)
	case Reject:
		out = writeAddressee(out, d.Addressee, p)
		out = ackOrReject(out, "rej", d.RejectedID, d.MessageID, d.ReplyAck, p)
	case Bulletin:
		out = encodeBulletin(out, d.Addressee, d.Text, d.MessageID, false, p)
	case NWSBulletin:
		out = encodeBulletin(out, d.Addressee, d.Text, d.MessageID, true, p)
	case TelemetryNames:
		out = encodeLabels(out, d.Addressee, "PARM.", d.Names, d.MessageID, p)
	case TelemetryUnits:
		out = encodeLabels(out, d.Addressee, "UNIT.", d.Units, d.MessageID, p)
	case TelemetryCoefficients:
		if len(d.Coefficients) > 15 {
			p.note(newEncodeError(RefusalTooLong, "telemetry equations are up to 15 numbers (APRS12c ch. 13)"))
		}
		var texts []string
		for i, c := range d.Coefficients {
			var text *string
			if i < len(d.CoefficientsText) {
				text = &d.CoefficientsText[i]
			}
			if t, err := numberAsSent(c, text, isCoefficient, "a telemetry coefficient"); p.run(err) {
				texts = append(texts, t)
			}
		}
		out = writeAddressee(out, d.Addressee, p)
		out = append(out, "EQNS."...)
		out = append(out, strings.Join(texts, ",")...)
		out = idAndReply(out, d.MessageID, nil, p)
	case TelemetryBits:
		if strings.ContainsRune(d.Project, '{') || hasLineBreak([]byte(d.Project)) {
			p.note(newEncodeError(RefusalInvalidCharacter, "a telemetry project title has no { or line break (APRS12c ch. 13)"))
		}
		if runeCount(d.Project) > 23 {
			p.note(newEncodeError(RefusalTooLong, "a telemetry project title is up to 23 characters (APRS12c ch. 13)"))
		}
		if len(d.Bits) != 8 || !onlyBits(d.Bits) {
			p.note(newEncodeError(RefusalMalformed, "telemetry bit sense is eight 0 or 1 characters (APRS12c ch. 13)"))
		}
		out = writeAddressee(out, d.Addressee, p)
		out = append(out, "BITS."...)
		out = append(out, d.Bits...)
		if d.Project != "" {
			out = append(out, ',')
			out = append(out, d.Project...)
		}
		out = idAndReply(out, d.MessageID, nil, p)
	case DirectedQuery:
		if !queryTypeOK(d.QueryType) {
			p.note(newEncodeError(RefusalInvalidCode, "a directed query type is upper-case letters: ?APRSD, ?APRSH, ?APRSM, ?APRSO, ?APRSP, ?APRSS, ?APRST, ?PING? or one the spec does not define (APRS12c ch. 15)"))
		}
		out = writeAddressee(out, d.Addressee, p)
		out = append(out, '?')
		out = append(out, d.QueryType...)
		if d.Target != nil {
			t := *d.Target
			if !isValidAddress(t) {
				p.note(newEncodeError(RefusalInvalidAddressee, "a directed query's target is one callsign: 1-9 letters, digits or -"))
			}
			// Straight after a type the spec defines, APRSH's padded to 9 characters ("APRSH:
			// callsigns must be padded to 9 characters", APRS12c's 1.2 notes); after one space for
			// any other type, which has no fixed length.
			if !isQueryType(d.QueryType) {
				out = append(out, ' ')
			}
			out = append(out, t...)
			if d.QueryType == "APRSH" {
				for i := len(t); i < 9; i++ {
					out = append(out, ' ')
				}
			}
		}
	default:
		return nil, newEncodeError(RefusalNotEncodable, "not a message")
	}
	if err := p.finish(); err != nil {
		return nil, err
	}
	// What was written must read back as what was given: a message whose text looks like an ack,
	// telemetry metadata or a query, or whose addressee makes it a bulletin, would not.
	back := decodeMessage(newContext(Lenient), out)
	if !sameMessageData(back, data) {
		// An addressee that makes a bulletin of it is not the form this report needs.
		var to *string
		switch d := data.(type) {
		case Message:
			to = &d.Addressee
		case Ack:
			to = &d.Addressee
		case Reject:
			to = &d.Addressee
		}
		if to != nil && isBulletinAddressee(*to) {
			return nil, newEncodeError(RefusalInvalidAddressee, "an addressee of NWS-, NWS_ or BLN and a bulletin form makes a bulletin, not this report")
		}
		return nil, newEncodeError(RefusalTextReadsAsField, "it would read back as something else: text that reads as an ack, telemetry metadata or a query, or an addressee that makes it a bulletin")
	}
	return out, nil
}

func encodeBulletin(out []byte, addressee, text string, messageID *string, isNWS bool, p *pending) []byte {
	nws := strings.HasPrefix(addressee, "NWS-") || strings.HasPrefix(addressee, "NWS_")
	if isNWS != nws {
		p.note(newEncodeError(RefusalInvalidAddressee, "an NWS bulletin is addressed NWS-xxxxx, and other bulletins are not"))
	}
	if !nws {
		rest := strings.TrimPrefix(addressee, "BLN")
		if !strings.HasPrefix(addressee, "BLN") {
			rest = ""
		}
		ok := false
		switch {
		case len(rest) == 1:
			ok = isDigit(rest[0]) || isUpper(rest[0])
		case len(rest) > 1:
			ok = isDigit(rest[0]) && len(rest)-1 <= 5
		}
		if !ok {
			p.note(newEncodeError(RefusalInvalidAddressee, "a bulletin is addressed BLN and a digit or letter, or BLN, a digit and a group name of up to 5 (APRS12c ch. 14)"))
		}
	}
	out = writeAddressee(out, addressee, p)
	// NWS bulletin text has no length limit; the 67 characters are for messages and other bulletins.
	out = messageText(out, text, !nws, p)
	return idAndReply(out, messageID, nil, p)
}

func encodeLabels(out []byte, addressee, prefix string, labels []string, messageID *string, p *pending) []byte {
	for _, l := range labels {
		if strings.ContainsAny(l, ",{") || hasLineBreak([]byte(l)) {
			p.note(newEncodeError(RefusalInvalidCharacter, "telemetry metadata names are without commas, braces or line breaks (APRS12c ch. 13)"))
			break
		}
	}
	if len(labels) > 13 {
		p.note(newEncodeError(RefusalTooLong, "telemetry metadata names at most 13 channels (APRS12c ch. 13)"))
	}
	out = writeAddressee(out, addressee, p)
	out = append(out, prefix...)
	out = append(out, strings.Join(labels, ",")...)
	return idAndReply(out, messageID, nil, p)
}

// writeAddressee writes an addressee, which is nine characters with no space or :.
func writeAddressee(out []byte, a string, p *pending) []byte {
	b := []byte(a)
	if len(b) == 0 || len(b) > 9 || !isPrintableASCII(b) || contains(b, ' ') || contains(b, ':') {
		p.note(newEncodeError(RefusalInvalidAddressee, "an addressee is 1-9 printable characters without spaces or : (APRS12c ch. 14)"))
	}
	out = append(out, ':')
	out = append(out, b...)
	for i := len(b); i < 9; i++ {
		out = append(out, ' ')
	}
	return append(out, ':')
}

func messageText(out []byte, t string, limited bool, p *pending) []byte {
	if strings.ContainsRune(t, '{') {
		p.note(newEncodeError(RefusalInvalidCharacter, "message text must not contain '{', which starts the message ID (APRS12c ch. 14)"))
	}
	if hasLineBreak([]byte(t)) {
		p.note(newEncodeError(RefusalInvalidCharacter, "message text cannot contain a line break"))
	}
	if limited && runeCount(t) > maxMessageText {
		p.note(newEncodeError(RefusalTooLong, "message text is limited to 67 characters (APRS12c ch. 14)"))
	}
	return append(out, t...)
}

func idOK(id string) bool {
	return isMessageIDBytes([]byte(id))
}

func idAndReply(out []byte, id, replyAck *string, p *pending) []byte {
	switch {
	case id == nil && replyAck == nil:
	case id == nil:
		// A reply-ack wrong on its own is wrong whatever else is given.
		if r := *replyAck; r != "" && !idOK(r) {
			p.note(newEncodeError(RefusalInvalidMessageID, "a reply-ack is 1-5 letters or digits"))
		}
		p.note(newEncodeError(RefusalMissingField, "a reply-ack goes with a message ID"))
	default:
		if !idOK(*id) {
			p.note(newEncodeError(RefusalInvalidMessageID, "a message ID is 1-5 letters or digits (APRS12c ch. 14)"))
		}
		out = append(out, '{')
		out = append(out, *id...)
		if replyAck != nil {
			r := *replyAck
			if r != "" && !idOK(r) {
				p.note(newEncodeError(RefusalInvalidMessageID, "a reply-ack is 1-5 letters or digits"))
			}
			out = append(out, '}')
			out = append(out, r...)
		}
	}
	return out
}

func ackOrReject(out []byte, kind, id string, messageID, replyAck *string, p *pending) []byte {
	if messageID != nil {
		p.note(newEncodeError(RefusalFieldNotAllowed, "an ack or rej carries no message ID of its own (UAP 5.32)"))
	}
	if !idOK(id) {
		p.note(newEncodeError(RefusalInvalidMessageID, "the message ID being acked or rejected is 1-5 letters or digits"))
	}
	out = append(out, kind...)
	out = append(out, id...)
	if replyAck != nil {
		r := *replyAck
		if r != "" && !idOK(r) {
			p.note(newEncodeError(RefusalInvalidMessageID, "a reply-ack is 1-5 letters or digits"))
		}
		out = append(out, '}')
		out = append(out, r...)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameMessageData compares two message-like data as the encoder's read-back does: the
// coefficients' text as sent is not compared.
func sameMessageData(a, b Data) bool {
	switch x := a.(type) {
	case Message:
		y, ok := b.(Message)
		return ok && x.Addressee == y.Addressee && x.Text == y.Text && samePtr(x.MessageID, y.MessageID) && samePtr(x.ReplyAck, y.ReplyAck)
	case Ack:
		y, ok := b.(Ack)
		return ok && x.Addressee == y.Addressee && x.AckedID == y.AckedID && samePtr(x.MessageID, y.MessageID) && samePtr(x.ReplyAck, y.ReplyAck)
	case Reject:
		y, ok := b.(Reject)
		return ok && x.Addressee == y.Addressee && x.RejectedID == y.RejectedID && samePtr(x.MessageID, y.MessageID) && samePtr(x.ReplyAck, y.ReplyAck)
	case Bulletin:
		y, ok := b.(Bulletin)
		return ok && x.Addressee == y.Addressee && x.Text == y.Text && samePtr(x.MessageID, y.MessageID)
	case NWSBulletin:
		y, ok := b.(NWSBulletin)
		return ok && x.Addressee == y.Addressee && x.Text == y.Text && samePtr(x.MessageID, y.MessageID)
	case TelemetryNames:
		y, ok := b.(TelemetryNames)
		return ok && x.Addressee == y.Addressee && sameStrings(x.Names, y.Names) && samePtr(x.MessageID, y.MessageID)
	case TelemetryUnits:
		y, ok := b.(TelemetryUnits)
		return ok && x.Addressee == y.Addressee && sameStrings(x.Units, y.Units) && samePtr(x.MessageID, y.MessageID)
	case TelemetryCoefficients:
		y, ok := b.(TelemetryCoefficients)
		return ok && x.Addressee == y.Addressee && sameFloats(x.Coefficients, y.Coefficients) && samePtr(x.MessageID, y.MessageID)
	case TelemetryBits:
		y, ok := b.(TelemetryBits)
		return ok && x.Addressee == y.Addressee && x.Bits == y.Bits && x.Project == y.Project && samePtr(x.MessageID, y.MessageID)
	case DirectedQuery:
		y, ok := b.(DirectedQuery)
		return ok && x.Addressee == y.Addressee && x.QueryType == y.QueryType && samePtr(x.Target, y.Target)
	}
	return false
}
