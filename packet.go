package aprs

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Address is a station address as it appears in a header: CALL-SSID, or an APRS-IS name such as
// T2SPAIN.
//
// On APRS-IS an address is 1-9 letters, digits or hyphens. Over the air (AX.25) it must also be a
// callsign of at most six upper-case letters and digits with an SSID of 0-15; IsAX25 says whether
// it is. The source of a packet inside a third-party packet may be more: see ThirdPartySource.
// The zero Address is empty, which only a decoder makes (a tolerated empty destination).
type Address struct {
	text string
}

// InvalidAddressError is text that cannot be an address.
type InvalidAddressError struct {
	// Text is the text that was refused.
	Text string
}

func (e *InvalidAddressError) Error() string {
	return fmt.Sprintf("'%s' is not an address: 1-9 letters, digits or -", e.Text)
}

// NewAddress makes an address from text. It fails if the text is empty, longer than 9
// characters, or has characters other than letters, digits and -.
func NewAddress(text string) (Address, error) {
	if !isValidAddress(text) {
		return Address{}, &InvalidAddressError{Text: text}
	}
	return Address{text}, nil
}

// MustAddress is NewAddress for an address known to be good, such as a constant; it panics on a
// bad one.
func MustAddress(text string) Address {
	a, err := NewAddress(text)
	if err != nil {
		panic(err)
	}
	return a
}

func isValidAddress(text string) bool {
	if len(text) < 1 || len(text) > 9 {
		return false
	}
	for i := 0; i < len(text); i++ {
		if !isAlnum(text[i]) && text[i] != '-' {
			return false
		}
	}
	return true
}

// ThirdPartySource makes the source address of a packet carried inside a third-party packet.
// APRS12c ch. 17 lets it be any 1-9 printable ASCII characters other than > and : (PY2SP_R-R, say),
// since it "does not need to adhere to the AX.25 address restrictions". It fails for anything
// else. Such an address is not an APRS-IS address, so use it only there.
func ThirdPartySource(text string) (Address, error) {
	if !isThirdPartySource(text) {
		return Address{}, &InvalidAddressError{Text: text}
	}
	return Address{text}, nil
}

func isThirdPartySource(text string) bool {
	if len(text) < 1 || len(text) > 9 {
		return false
	}
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b < 0x20 || b > 0x7E || b == '>' || b == ':' {
			return false
		}
	}
	return true
}

// String gives the address as text.
func (a Address) String() string {
	return a.text
}

// Callsign gives the part before any -SSID.
func (a Address) Callsign() string {
	if i := strings.IndexByte(a.text, '-'); i >= 0 {
		return a.text[:i]
	}
	return a.text
}

// SSID gives the SSID after -, or 0 if there is none or it is not a number.
func (a Address) SSID() uint8 {
	i := strings.IndexByte(a.text, '-')
	if i < 0 {
		return 0
	}
	n, ok := parseU8(a.text[i+1:])
	if !ok {
		return 0
	}
	return n
}

// parseU8 reads a u8 as Rust's str::parse does: an optional +, then digits, at most 255.
func parseU8(s string) (uint8, bool) {
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > 255 {
			return 0, false
		}
	}
	return uint8(n), true
}

// IsAX25 says whether this address can be sent in an AX.25 frame: a callsign of 1-6 upper-case
// letters and digits, and an SSID of 1-15 written without a leading zero.
func (a Address) IsAX25() bool {
	call := a.Callsign()
	if len(call) < 1 || len(call) > 6 {
		return false
	}
	for i := 0; i < len(call); i++ {
		if !isUpper(call[i]) && !isDigit(call[i]) {
			return false
		}
	}
	i := strings.IndexByte(a.text, '-')
	if i < 0 {
		return true
	}
	ssid := a.text[i+1:]
	if ssid == "" || (len(ssid) > 1 && ssid[0] == '0') {
		return false
	}
	for j := 0; j < len(ssid); j++ {
		if !isDigit(ssid[j]) {
			return false
		}
	}
	n, ok := parseU8(ssid)
	return ok && n >= 1 && n <= 15
}

// PathEntry is an entry in the digipeater path.
type PathEntry struct {
	Address Address
	// Used: the packet has been through this digipeater (the H bit in AX.25; * on the last such
	// entry in TNC2 text).
	Used bool
}

// QConstruct is an APRS-IS q-construct in the path (qAR, qAC, ...) and the station after it.
type QConstruct struct {
	// Construct, such as qAR.
	Construct string
	// Station is the station named after it (for qAR, the IGate), or nil.
	Station *Address
}

// HeaderError is a header that cannot be decoded, so the packet cannot be either.
type HeaderError struct {
	// Diagnostics say what is wrong: at least one Error, plus any warnings found before it.
	Diagnostics []Diagnostic
}

func (e *HeaderError) Error() string {
	for _, d := range e.Diagnostics {
		if d.Severity == Error {
			return "unusable header: " + d.String()
		}
	}
	return "unusable header"
}

// Packet is an APRS packet: the header, the information field as received, what it decodes to,
// and everything the decoder noticed on the way.
type Packet struct {
	// Source is who sent it.
	Source Address
	// Destination is usually a software or device identifier (APxxxx); for Mic-E, half the position.
	Destination Address
	// Path is the digipeater path, including any APRS-IS q-construct.
	Path []PathEntry
	// Information is the information field exactly as received (for a TNC2 line, without a
	// trailing CR or LF).
	Information []byte
	// Data is what the information field says.
	Data Data
	// Diagnostics is what the decoder noticed, header first.
	Diagnostics []Diagnostic
	// ThirdParty: this packet was carried inside a third-party packet. Its header is kept as sent:
	// an APRS-IS q-construct is read only in the outer header, so QConstruct finds none here.
	ThirdParty bool
}

// DecodeTNC2 decodes a TNC2 / APRS-IS text line, SOURCE>DEST,PATH:information. The only error is
// an unusable header, a *HeaderError; anything wrong in the information field is in the packet's
// diagnostics.
func DecodeTNC2(line []byte, options ParseOptions) (*Packet, error) {
	p, err := decodeTNC2Line(line, options, 0)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func decodeTNC2Line(line []byte, options ParseOptions, depth int) (*Packet, *HeaderError) {
	thirdParty := depth > 0
	ctx := newContext(options)
	ctx.depth = depth
	colon := indexByte(line, ':')
	if colon < 0 {
		ctx.error(CodeInvalidHeader, "no ':' ends the header (SOURCE>DEST[,PATH]:)", none)
		return nil, ctx.headerError()
	}
	headerBytes, info := line[:colon], line[colon+1:]
	if !utf8.Valid(headerBytes) {
		ctx.error(CodeInvalidHeader, "the header is not text", 0)
		return nil, ctx.headerError()
	}
	header := string(headerBytes)
	gt := strings.IndexByte(header, '>')
	if gt < 0 {
		ctx.error(CodeInvalidHeader, "no '>' between source and destination", none)
		return nil, ctx.headerError()
	}
	sourceText, rest := header[:gt], header[gt+1:]
	if sourceText == "" {
		ctx.error(CodeInvalidHeader, "the source address is empty", 0)
		return nil, ctx.headerError()
	}
	fields := strings.Split(rest, ",")
	destinationText := fields[0]
	var source Address
	if thirdParty && isThirdPartySource(sourceText) {
		source = Address{sourceText}
	} else {
		a, err := headerAddress(ctx, sourceText)
		if err != nil {
			return nil, err
		}
		source = a
	}
	var destination Address
	if destinationText == "" {
		if !ctx.tolerate(CodeEmptyDestination, "the destination address is empty (UAP 5.2)", none) {
			return nil, ctx.headerError()
		}
	} else {
		a, err := headerAddress(ctx, destinationText)
		if err != nil {
			return nil, err
		}
		destination = a
	}

	var path []PathEntry
	usedMarkers := 0
	for _, field := range fields[1:] {
		if field == "" {
			if !ctx.tolerate(CodeEmptyPathEntry, "the path has an empty entry (UAP 5.6)", none) {
				return nil, ctx.headerError()
			}
			continue
		}
		text, used := field, false
		if strings.HasSuffix(field, "*") {
			text, used = field[:len(field)-1], true
		}
		if used {
			usedMarkers++
		}
		a, err := headerAddress(ctx, text)
		if err != nil {
			return nil, err
		}
		path = append(path, PathEntry{Address: a, Used: used})
	}

	if usedMarkers > 1 && !ctx.tolerate(CodeMultipleUsedMarkers, "more than one path entry is marked used with * (UAP 5.30)", none) {
		return nil, ctx.headerError()
	}

	// Every entry up to the last one marked used has been used.
	last := -1
	for i, e := range path {
		if e.Used {
			last = i
		}
	}
	for i := 0; i < last; i++ {
		path[i].Used = true
	}

	// A packet inside a third-party packet shares the outer packet's information field rather
	// than copying it, so nested third-party packets cost memory in proportion to the line, not
	// its square.
	if !thirdParty {
		info = append([]byte(nil), info...)
	}
	p := decodeWithContext(ctx, source, destination, path, info)
	p.ThirdParty = thirdParty
	return p, nil
}

// DecodeAX25 decodes an AX.25 UI frame in KISS form: addresses, control and PID, information; no
// flags, no FCS. The only error is an unusable header, a *HeaderError.
func DecodeAX25(frame []byte, options ParseOptions) (*Packet, error) {
	p, err := decodeAX25(frame, options)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func decodeAX25(frame []byte, options ParseOptions) (*Packet, *HeaderError) {
	ctx := newContext(options)
	var addresses [][]byte
	at := 0
	for {
		if len(frame) < at+7 {
			ctx.error(CodeNotAprsFrame, "the frame ends inside its address field", at)
			return nil, ctx.headerError()
		}
		chunk := frame[at : at+7]
		addresses = append(addresses, chunk)
		at += 7
		if chunk[6]&1 == 1 {
			break
		}
	}

	if len(addresses) < 2 {
		ctx.error(CodeNotAprsFrame, "the frame has no source address", 0)
		return nil, ctx.headerError()
	}
	if len(addresses) > 10 {
		ctx.error(CodeTooManyDigipeaters, "the frame has more than 8 digipeater addresses", 0)
		return nil, ctx.headerError()
	}
	if len(frame) < at+2 || frame[at]&^0x10 != 0x03 || frame[at+1] != 0xF0 {
		ctx.error(CodeNotAprsFrame, "not a UI frame with PID 0xF0 (APRS12c ch. 3)", at)
		return nil, ctx.headerError()
	}

	decoded := make([]Address, 0, len(addresses))
	for i, b := range addresses {
		role := rolePath
		switch i {
		case 0:
			role = roleDestination
		case 1:
			role = roleSource
		}
		a, err := ax25Address(ctx, b, i*7, role)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, a)
	}

	destination, source := decoded[0], decoded[1]
	var path []PathEntry
	for i, b := range addresses[2:] {
		if decoded[i+2].text == "" {
			continue
		}
		path = append(path, PathEntry{Address: decoded[i+2], Used: b[6]&0x80 != 0})
	}
	return decodeWithContext(ctx, source, destination, path, append([]byte(nil), frame[at+2:]...)), nil
}

// Decode decodes an information field whose header you already have.
func Decode(source, destination Address, path []PathEntry, information []byte, options ParseOptions) *Packet {
	return decodeWithContext(newContext(options), source, destination, path, append([]byte(nil), information...))
}

func decodeWithContext(ctx *context, source, destination Address, path []PathEntry, information []byte) *Packet {
	// information is the packet's own: the caller copied it, or it is a third-party packet's
	// share of the outer packet's.
	data := decodeInformation(ctx, destination, information)
	return &Packet{Source: source, Destination: destination, Path: path, Information: information, Data: data, Diagnostics: ctx.diagnostics}
}

// NewPacket builds a packet from data, encoding its information field. For Mic-E data the
// destination address is worked out from the report (it carries the latitude, message code and
// digipeat path, APRS12c ch. 10), and destination is not used. An empty address (the zero
// Address) is refused with RefusalInvalidAddress. data may also be a pointer to one of the data
// types. A refusal is an *EncodeError.
func NewPacket(source, destination Address, path []PathEntry, data Data) (*Packet, error) {
	p, err := newPacket(source, destination, path, data)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newPacket(source, destination Address, path []PathEntry, data Data) (*Packet, *EncodeError) {
	data, err := prepare(data)
	if err != nil {
		return nil, err
	}
	pend := &pending{}
	// The zero Address is empty, which no header can carry.
	_, micE := data.(MicEReport)
	empty := source.text == "" || (destination.text == "" && !micE)
	for _, e := range path {
		empty = empty || e.Address.text == ""
	}
	if empty {
		pend.note(newEncodeError(RefusalInvalidAddress, "a header address is empty: give the source, destination and each path entry an address"))
	}
	var information []byte
	if m, ok := data.(MicEReport); ok {
		if pend.run(validate(m)) {
			d, info, err := encodeMicE(&m)
			if pend.run(err) {
				destination, information = d, info
			}
		}
	} else {
		info, err := encode(data)
		if pend.run(err) {
			information = info
		}
	}
	if err := pend.finish(); err != nil {
		return nil, err
	}
	path = append([]PathEntry(nil), path...)
	decoded := Decode(source, destination, path, information, Strict)
	return &Packet{Source: source, Destination: destination, Path: path, Information: information, Data: data, Diagnostics: decoded.Diagnostics}, nil
}

// HasWarnings says whether any diagnostic is a warning.
func (p *Packet) HasWarnings() bool {
	for _, d := range p.Diagnostics {
		if d.Severity == Warning {
			return true
		}
	}
	return false
}

// HasErrors says whether any diagnostic is an error.
func (p *Packet) HasErrors() bool {
	for _, d := range p.Diagnostics {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// QConstruct gives the first APRS-IS q-construct in the path, and the station after it. There is
// none for a packet carried inside a third-party packet, whose path is kept as sent: a
// q-construct is read only in the outer header.
func (p *Packet) QConstruct() (QConstruct, bool) {
	if p.ThirdParty {
		return QConstruct{}, false
	}
	for i, e := range p.Path {
		if isQConstruct(e.Address.text) {
			q := QConstruct{Construct: e.Address.text}
			if i+1 < len(p.Path) {
				station := p.Path[i+1].Address
				q.Station = &station
			}
			return q, true
		}
	}
	return QConstruct{}, false
}

// TNC2 gives the packet as a TNC2 text line (without a line ending). The last used path entry is
// marked *.
func (p *Packet) TNC2() []byte {
	out := make([]byte, 0, 32+len(p.Information))
	out = append(out, p.Source.text...)
	out = append(out, '>')
	out = append(out, p.Destination.text...)
	last := -1
	for i, e := range p.Path {
		if e.Used {
			last = i
		}
	}
	for i, e := range p.Path {
		out = append(out, ',')
		out = append(out, e.Address.text...)
		if i == last {
			out = append(out, '*')
		}
	}
	out = append(out, ':')
	return append(out, p.Information...)
}

// String gives the TNC2 line as text.
func (p *Packet) String() string {
	return string(p.TNC2())
}

// AX25 gives the packet as an AX.25 UI frame in KISS form (no flags, no FCS). It fails, with an
// *EncodeError, if an address cannot be sent over the air, or the path is longer than 8 entries.
func (p *Packet) AX25() ([]byte, error) {
	out, err := p.ax25()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Packet) ax25() ([]byte, *EncodeError) {
	if len(p.Path) > 8 {
		return nil, newEncodeError(RefusalAX25CannotCarry, "an AX.25 frame has room for at most 8 digipeaters")
	}
	out := make([]byte, 0, 16+7*len(p.Path)+len(p.Information))
	var err *EncodeError
	if out, err = writeAX25Address(out, p.Destination, 0xE0); err != nil {
		return nil, err
	}
	if out, err = writeAX25Address(out, p.Source, 0x60); err != nil {
		return nil, err
	}
	for _, e := range p.Path {
		flags := byte(0x60)
		if e.Used {
			flags = 0xE0
		}
		if out, err = writeAX25Address(out, e.Address, flags); err != nil {
			return nil, err
		}
	}
	out[len(out)-1] |= 1
	out = append(out, 0x03, 0xF0)
	return append(out, p.Information...), nil
}

// isQConstruct: q, an upper-case letter, then a letter of either case: qAr and qAo (UAP section 4)
// are in use.
func isQConstruct(text string) bool {
	return len(text) == 3 && text[0] == 'q' && isUpper(text[1]) && isAlpha(text[2])
}

func headerAddress(ctx *context, text string) (Address, *HeaderError) {
	if isValidAddress(text) {
		return Address{text}, nil
	}
	ctx.error(CodeInvalidAddress, fmt.Sprintf("'%s' is not an address: 1-9 letters, digits or -", text), none)
	return Address{}, ctx.headerError()
}

type addressRole int

const (
	roleSource addressRole = iota
	roleDestination
	rolePath
)

func ax25Address(ctx *context, b []byte, offset int, role addressRole) (Address, *HeaderError) {
	chars := make([]byte, 6)
	for i := 0; i < 6; i++ {
		chars[i] = b[i] >> 1
	}
	end := 6
	for end > 0 && chars[end-1] == ' ' {
		end--
	}
	call := chars[:end]
	if contains(call, 0) {
		// Only trailing NULs are padding; a NUL inside a callsign is garbage.
		nulStart := indexByte(call, 0)
		for _, c := range call[nulStart:] {
			if c != 0 {
				ctx.error(CodeInvalidAddress, "an AX.25 address has a NUL inside it", offset)
				return Address{}, ctx.headerError()
			}
		}
		if !ctx.tolerate(CodeNulPaddedAddress, "an AX.25 address is padded with NUL rather than spaces (UAP 5.29)", offset) {
			return Address{}, ctx.headerError()
		}
		call = call[:nulStart]
	}
	if len(call) == 0 {
		// A call of spaces only is empty, as an empty TNC2 address is: an empty source is
		// unusable, an empty destination or digipeater is tolerated.
		switch role {
		case roleDestination:
			if !ctx.tolerate(CodeEmptyDestination, "the destination address is empty (UAP 5.2)", offset) {
				return Address{}, ctx.headerError()
			}
		case rolePath:
			if !ctx.tolerate(CodeEmptyPathEntry, "the path has an empty entry (UAP 5.6)", offset) {
				return Address{}, ctx.headerError()
			}
		default:
			ctx.error(CodeInvalidAddress, "an AX.25 address is empty", offset)
			return Address{}, ctx.headerError()
		}
		return Address{}, nil
	}
	lower := false
	for _, c := range call {
		switch {
		case isUpper(c) || isDigit(c):
		case isLower(c):
			lower = true
		default:
			// Anything else (punctuation, a space inside, a control character) would write a
			// TNC2 header that reads back as a different one.
			ctx.error(CodeInvalidAddress, "an AX.25 address is upper-case letters and digits (AX.25 2.2 section 3.12)", offset)
			return Address{}, ctx.headerError()
		}
	}
	if lower && !ctx.tolerate(CodeInvalidAX25AddressCharacters, "an AX.25 address has lower-case letters", offset) {
		return Address{}, ctx.headerError()
	}
	text := string(call)
	if ssid := (b[6] >> 1) & 0x0F; ssid != 0 {
		text += fmt.Sprintf("-%d", ssid)
	}
	return Address{text}, nil
}

func writeAX25Address(out []byte, a Address, flags byte) ([]byte, *EncodeError) {
	if !a.IsAX25() {
		return out, newEncodeError(RefusalAX25CannotCarry, fmt.Sprintf("'%s' cannot be sent in an AX.25 frame: 1-6 upper-case letters and digits, SSID 0-15", a.text))
	}
	call := a.Callsign()
	for i := 0; i < 6; i++ {
		c := byte(' ')
		if i < len(call) {
			c = call[i]
		}
		out = append(out, c<<1)
	}
	return append(out, flags|(a.SSID()<<1)), nil
}
