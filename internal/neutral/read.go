package neutral

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	aprs "github.com/packet-net/aprs-go"
)

// Parse reads JSON into neutral values: a whole number as JSON writes it (no point, no exponent)
// is an int64 when it fits, and any other number a float64.
//
// The tokens NaN, Infinity and -Infinity, which the generator writes for a value that is not a
// finite number, are read as that float.
func Parse(text []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(nonFiniteTokens(text)))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return convert(v), nil
}

const (
	nanToken    = "\x00NaN"
	infToken    = "\x00Inf"
	negInfToken = "\x00-Inf"
)

// nonFiniteTokens replaces the bare NaN, Infinity and -Infinity tokens (outside strings) with
// strings convert turns back into floats.
func nonFiniteTokens(text []byte) []byte {
	if !bytes.Contains(text, []byte("NaN")) && !bytes.Contains(text, []byte("Infinity")) {
		return text
	}
	out := make([]byte, 0, len(text)+16)
	inString := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(text) {
				i++
				out = append(out, text[i])
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case bytes.HasPrefix(text[i:], []byte("NaN")):
			out = append(out, `"\u0000NaN"`...)
			i += 2
		case bytes.HasPrefix(text[i:], []byte("-Infinity")):
			out = append(out, `"\u0000-Inf"`...)
			i += 8
		case bytes.HasPrefix(text[i:], []byte("Infinity")):
			out = append(out, `"\u0000Inf"`...)
			i += 7
		default:
			out = append(out, c)
		}
	}
	return out
}

func convert(v any) any {
	switch x := v.(type) {
	case string:
		switch x {
		case nanToken:
			return math.NaN()
		case infToken:
			return math.Inf(1)
		case negInfToken:
			return math.Inf(-1)
		}
		return x
	case json.Number:
		s := string(x)
		if !strings.ContainsAny(s, ".eE") {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
		f, _ := strconv.ParseFloat(s, 64)
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = convert(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = convert(e)
		}
		return x
	}
	return v
}

// Unreadable says why data in the neutral form could not be made into this package's data.
type Unreadable struct {
	// Refused: its encoder refused a part that has to be encoded to be held (a third-party packet
	// keeps its inner packet's information field, which the neutral form does not carry). When
	// false, this package's data cannot hold something in it.
	Refused bool
	Code    aprs.RefusalCode
	Reason  string
	// Tool: this reader made the refusal, not the library's encoder (the dump record says
	// "via": "tool"), for an input this package cannot hold whose value alone decides a code.
	Tool bool
}

func (u *Unreadable) Error() string { return u.Reason }

func unsupported(format string, args ...any) *Unreadable {
	return &Unreadable{Reason: fmt.Sprintf(format, args...)}
}

func refused(code aprs.RefusalCode, reason string) *Unreadable {
	return &Unreadable{Refused: true, Code: code, Reason: reason}
}

// toolRefused is a refusal the reader makes itself.
func toolRefused(code aprs.RefusalCode, reason string) *Unreadable {
	return &Unreadable{Refused: true, Code: code, Reason: reason, Tool: true}
}

// collector gathers the refusals of inputs this package cannot hold whose value alone decides a
// code (a symbol that is not a table and a code, a path entry that is not an address, a bearing or
// quality out of range). Each is replaced by a valid stand-in so that the rest of the data can be
// encoded, and the result is settled afterwards (Settle): the first code in the Refusal reasons
// order among the encoder's own refusal and these (vectors README, Encode records).
type collector struct{ pending []*Unreadable }

func (c *collector) note(code aprs.RefusalCode, reason string) {
	if c != nil {
		c.pending = append(c.pending, toolRefused(code, reason))
	}
}

// first is the pending refusal that comes first in the order.
func (c *collector) first() *Unreadable {
	var best *Unreadable
	for _, u := range c.pending {
		if best == nil || RefusalRank(u.Code.ID()) < RefusalRank(best.Code.ID()) {
			best = u
		}
	}
	return best
}

// refusalOrder is the README's Refusal reasons order.
var refusalOrder = []string{
	"not-encodable", "invalid-address", "ax25-cannot-carry", "tnc2-cannot-carry", "invalid-third-party-header",
	"invalid-addressee", "invalid-name", "invalid-timestamp", "not-a-number", "value-out-of-range", "value-has-no-form",
	"invalid-code", "invalid-symbol", "invalid-character", "too-long", "padding-space", "malformed", "invalid-message-id",
	"invalid-nmea", "extension-conflict", "field-not-allowed", "symbol-mismatch", "missing-field", "value-given-twice",
	"text-reads-as-field", "does-not-read-back", "information-field-too-long",
}

// RefusalRank is a code's place in the Refusal reasons order.
func RefusalRank(id string) int {
	for i, c := range refusalOrder {
		if c == id {
			return i
		}
	}
	return len(refusalOrder)
}

// Settle gives the refusal of data read with stand-ins: the pending refusal (from ReadDataStandIn),
// unless the encoder's own refusal (nil when it wrote) comes earlier in the order. The size
// refusal, information-field-too-long, is last in the order, so pending wins over it.
func Settle(pending *Unreadable, encoder *aprs.RefusalCode, encoderReason string) (aprs.RefusalCode, string, bool) {
	if pending == nil {
		if encoder == nil {
			return 0, "", false
		}
		return *encoder, encoderReason, false
	}
	if encoder != nil && RefusalRank(encoder.ID()) < RefusalRank(pending.Code.ID()) {
		return *encoder, encoderReason, false
	}
	return pending.Code, pending.Reason, pending.Tool
}

// keys is an object's keys, checked against the ones this package's data can hold, with getters
// that name the key when a value will not fit.
type keys struct {
	o  map[string]any
	at string
	c  *collector
}

func newKeys(v any, at string, allowed []string) (*keys, *Unreadable) {
	o, ok := v.(map[string]any)
	if !ok {
		return nil, unsupported("%s: not an object", at)
	}
	k := &keys{o: o, at: at}
	for key := range o {
		if !containsString(allowed, key) {
			return nil, unsupported("%s: not a field this package has", k.name(key))
		}
	}
	return k, nil
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (k *keys) name(key string) string {
	if k.at == "" {
		return key
	}
	return k.at + "." + key
}

func (k *keys) get(key string) any {
	v, ok := k.o[key]
	if !ok {
		return nil
	}
	return v
}

func (k *keys) str(key string) (*string, *Unreadable) {
	switch v := k.get(key).(type) {
	case nil:
		return nil, nil
	case string:
		return &v, nil
	}
	return nil, unsupported("%s: not text", k.name(key))
}

func (k *keys) text(key string) (string, *Unreadable) {
	s, err := k.str(key)
	if err != nil || s == nil {
		return "", err
	}
	return *s, nil
}

func (k *keys) char(key string) (*rune, *Unreadable) {
	s, err := k.str(key)
	if err != nil || s == nil {
		return nil, err
	}
	if utf8.RuneCountInString(*s) != 1 {
		return nil, unsupported("%s: not one character", k.name(key))
	}
	r, _ := utf8.DecodeRuneInString(*s)
	return &r, nil
}

func (k *keys) flag(key string) (bool, *Unreadable) {
	switch v := k.get(key).(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	}
	return false, unsupported("%s: not a boolean", k.name(key))
}

// asFloat is a JSON number as a float, as serde_json's as_f64 gives it.
func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func (k *keys) float(key string) (*float64, *Unreadable) {
	v := k.get(key)
	if v == nil {
		return nil, nil
	}
	f, ok := asFloat(v)
	if !ok {
		return nil, unsupported("%s: not a number", k.name(key))
	}
	return &f, nil
}

func (k *keys) requiredFloat(key string) (float64, *Unreadable) {
	f, err := k.float(key)
	if err != nil {
		return 0, err
	}
	if f == nil {
		return 0, unsupported("%s: missing, and this package has no value for its absence", k.name(key))
	}
	return *f, nil
}

// whole is a JSON number that is a whole number, as an integer (false past what an int64 holds,
// which no field here holds either).
func whole(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e30 {
			if math.Abs(x) >= 9.2e18 {
				return 0, false
			}
			return int64(x), true
		}
	}
	return 0, false
}

type integer interface {
	uint8 | uint16 | uint32 | int32
}

// fits converts n to T when T holds it.
func fits[T integer](n int64) (T, bool) {
	t := T(n)
	if int64(t) != n {
		return 0, false
	}
	return t, true
}

// integerOf reads a whole number that fits T.
func integerOf[T integer](k *keys, key string) (*T, *Unreadable) {
	v := k.get(key)
	if v == nil {
		return nil, nil
	}
	if n, ok := whole(v); ok {
		if t, ok := fits[T](n); ok {
			return &t, nil
		}
	}
	return nil, unsupported("%s: not a whole number in the range this package holds", k.name(key))
}

func requiredInteger[T integer](k *keys, key string) (T, *Unreadable) {
	v, err := integerOf[T](k, key)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, unsupported("%s: missing, and this package has no value for its absence", k.name(key))
	}
	return *v, nil
}

// stepped reads a number this package holds in whole steps (a raw rain counter), rounded to the
// nearest as the encoder rounds any value between its field's steps (vectors README, Encoding).
// One that rounds outside what T holds is outside the field, so the data is refused.
func stepped[T integer](k *keys, key string) (*T, *Unreadable) {
	v := k.get(key)
	if v == nil {
		return nil, nil
	}
	if n, ok := whole(v); ok {
		if t, ok := fits[T](n); ok {
			return &t, nil
		}
		return nil, refused(aprs.RefusalValueOutOfRange, k.name(key)+": outside its field")
	}
	x, ok := asFloat(v)
	if !ok || math.IsNaN(x) || math.IsInf(x, 0) {
		return nil, unsupported("%s: not a number", k.name(key))
	}
	r := roundSteps(x)
	if math.Abs(r) < 9.2e18 {
		if t, ok := fits[T](int64(r)); ok {
			return &t, nil
		}
	}
	return nil, refused(aprs.RefusalValueOutOfRange, k.name(key)+": outside its field once rounded")
}

// roundSteps rounds a value measured in steps to the nearest, halves (within 1e-6) away from
// zero, as the encoder rounds.
func roundSteps(v float64) float64 {
	floor := math.Floor(math.Abs(v))
	fraction := math.Abs(v) - floor
	rounded := floor
	if fraction > 0.5 || math.Abs(fraction-0.5) <= 1e-6 {
		rounded = floor + 1
	}
	if v < 0 {
		return -rounded
	}
	return rounded
}

func (k *keys) strings(key string) ([]string, *Unreadable) {
	switch v := k.get(key).(type) {
	case nil:
		return nil, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				return nil, unsupported("%s: not text", k.name(key))
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, unsupported("%s: not a list", k.name(key))
}

func (k *keys) array(key string) ([]any, bool, *Unreadable) {
	switch v := k.get(key).(type) {
	case nil:
		return nil, false, nil
	case []any:
		return v, true, nil
	}
	return nil, false, unsupported("%s: not a list", k.name(key))
}

func (k *keys) object(key string, allowed []string) (*keys, *Unreadable) {
	v := k.get(key)
	if v == nil {
		return nil, nil
	}
	o, err := newKeys(v, k.name(key), allowed)
	if o != nil {
		o.c = k.c
	}
	return o, err
}

// choice reads one of an enumeration's values, by its neutral name: the index in names.
func (k *keys) choice(key string, names []string) (int, bool, *Unreadable) {
	s, err := k.str(key)
	if err != nil || s == nil {
		return 0, false, err
	}
	for i, n := range names {
		if n == *s {
			return i, true, nil
		}
	}
	return 0, false, unsupported("%s: not a value this package has", k.name(key))
}

func (k *keys) requiredChoice(key string, names []string) (int, *Unreadable) {
	i, ok, err := k.choice(key, names)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, k.missing(key)
	}
	return i, nil
}

func (k *keys) missing(key string) *Unreadable {
	return unsupported("%s: missing, and this package has no value for its absence", k.name(key))
}

// numberAsText is a number in the neutral form as the text this package keeps as sent: a string
// as it is, and a number, which has no text, as the shortest decimal that reads back as the same
// number, with no exponent, no + and no trailing .0 (vectors interpretations.md, "Numbers given
// without their text").
func numberAsText(v any) (string, bool) {
	switch x := v.(type) {
	case int64:
		return strconv.FormatInt(x, 10), true
	case float64:
		if x == 0 {
			return "0", true
		}
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case string:
		return x, true
	}
	return "", false
}

// telemetryNumber is a telemetry value or sequence as numberAsText, except that a number that is
// a whole number from 0 to 999 is three digits, as APRS12c ch. 13 writes aaa and sss.
func telemetryNumber(v any) (string, bool) {
	if x, ok := asFloat(v); ok && x >= 0 && x <= 999 && x == math.Trunc(x) {
		return fmt.Sprintf("%03d", int(x)), true
	}
	return numberAsText(v)
}

var positionedKeys = []string{
	"latitude", "longitude", "ambiguity", "symbol", "compressed", "compression", "course_degrees", "speed_knots",
	"altitude_feet", "phg", "range_miles", "dfs", "area", "df_bearing", "storm", "dao", "telemetry", "frequency",
	"weather", "signpost", "comment", "type", "timestamp", "messaging",
}

func withPositioned(extra ...string) []string {
	return append(append([]string(nil), positionedKeys...), extra...)
}

// headerDefects are the codes a third-party packet's header carries as part of its data, which
// no clean form writes back.
func isHeaderDefect(c aprs.Code) bool {
	switch c {
	case aprs.CodeEmptyDestination, aprs.CodeEmptyPathEntry, aprs.CodeMultipleUsedMarkers, aprs.CodeNulPaddedAddress, aprs.CodeInvalidAX25AddressCharacters:
		return true
	}
	return false
}

// ReadData makes neutral data into this package's data, or says why it cannot. An input this
// package cannot hold whose value alone decides a refusal code is a tool refusal here; the dump
// tool and the runner use ReadDataStandIn and Settle to compare it with the encoder's own.
func ReadData(v any) (aprs.Data, *Unreadable) {
	d, pending, err := ReadDataStandIn(v)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		return nil, pending
	}
	return d, nil
}

// ReadDataStandIn is ReadData with every input it cannot hold, and whose value alone decides a
// refusal code, left out or given a valid stand-in, and the first of their refusals in the order
// as pending (nil when there is none). An error, if any, is final: when it is a refusal and
// something is pending, the earlier of the two is the error.
func ReadDataStandIn(v any) (aprs.Data, *Unreadable, *Unreadable) {
	c := &collector{}
	d, err := readData(v, c)
	pending := c.first()
	if err != nil && pending != nil && err.Refused {
		if RefusalRank(pending.Code.ID()) < RefusalRank(err.Code.ID()) {
			return nil, nil, pending
		}
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, err
	}
	return d, pending, nil
}

func readData(v any, c *collector) (aprs.Data, *Unreadable) {
	var kind string
	if m, ok := v.(map[string]any); ok {
		kind, _ = m["type"].(string)
	}
	var allowed []string
	switch kind {
	case "position":
		allowed = withPositioned()
	case "mic-e":
		allowed = withPositioned("mic_e_message", "old_data", "type_code", "device_suffix", "locator", "legacy_telemetry", "destination_ssid")
	case "object", "item":
		allowed = withPositioned("name", "killed")
	case "message":
		allowed = []string{"type", "addressee", "text", "message_id", "reply_ack"}
	case "ack":
		allowed = []string{"type", "addressee", "acked_id", "message_id", "reply_ack"}
	case "reject":
		allowed = []string{"type", "addressee", "rejected_id", "message_id", "reply_ack"}
	case "bulletin", "nws-bulletin":
		allowed = []string{"type", "addressee", "text", "message_id"}
	case "telemetry-names":
		allowed = []string{"type", "addressee", "names", "message_id"}
	case "telemetry-units":
		allowed = []string{"type", "addressee", "units", "message_id"}
	case "telemetry-coefficients":
		allowed = []string{"type", "addressee", "coefficients", "message_id"}
	case "telemetry-bits":
		allowed = []string{"type", "addressee", "bits", "project", "message_id"}
	case "directed-query":
		allowed = []string{"type", "addressee", "query_type", "target"}
	case "status":
		allowed = []string{"type", "timestamp", "locator", "symbol", "beam", "text"}
	case "telemetry":
		allowed = []string{"type", "sequence", "analog", "bits", "comment"}
	case "weather":
		allowed = []string{"type", "timestamp", "weather", "comment"}
	case "raw-weather":
		allowed = []string{"type", "format", "data"}
	case "nmea":
		allowed = []string{"type", "sentence", "has_checksum", "latitude", "longitude", "fix", "course_degrees", "speed_knots", "altitude_m", "time", "waypoint", "comment"}
	case "maidenhead-beacon":
		allowed = []string{"type", "locator", "comment"}
	case "query":
		allowed = []string{"type", "query_type", "footprint"}
	case "capabilities":
		allowed = []string{"type", "capabilities"}
	case "third-party":
		allowed = []string{"type", "packet"}
	case "user-defined":
		allowed = []string{"type", "user_id", "packet_type", "data"}
	case "test":
		allowed = []string{"type", "data"}
	case "agrelo-df":
		allowed = []string{"type", "bearing_degrees", "quality"}
	case "unrecognized":
		allowed = []string{"type", "reason"}
	default:
		return nil, unsupported("type: not a data type this package has")
	}
	// Positioned fields that belong to another kind of report.
	filtered := allowed[:0:0]
	for _, k := range allowed {
		switch k {
		case "timestamp":
			if kind != "position" && kind != "object" && kind != "status" && kind != "weather" {
				continue
			}
		case "messaging":
			if kind != "position" {
				continue
			}
		}
		filtered = append(filtered, k)
	}
	o, err := newKeys(v, "", filtered)
	if err != nil {
		return nil, err
	}
	o.c = c
	return readKind(o, kind)
}

func readKind(o *keys, kind string) (aprs.Data, *Unreadable) {
	var err *Unreadable
	str := func(key string) *string {
		if err != nil {
			return nil
		}
		var s *string
		s, err = o.str(key)
		return s
	}
	text := func(key string) string {
		if err != nil {
			return ""
		}
		var s string
		s, err = o.text(key)
		return s
	}
	flag := func(key string) bool {
		if err != nil {
			return false
		}
		var b bool
		b, err = o.flag(key)
		return b
	}
	switch kind {
	case "position":
		r := aprs.PositionReport{}
		r.Timestamp, err = readTimestamp(o)
		r.Messaging = flag("messaging")
		if err == nil {
			r.Fields, err = readPositioned(o)
		}
		return r, err
	case "mic-e":
		r := aprs.MicEReport{}
		m, merr := readMicEMessage(o)
		if merr != nil {
			return nil, merr
		}
		if m == nil {
			return nil, o.missing("mic_e_message")
		}
		r.MicEMessage = *m
		r.OldData = flag("old_data")
		if err != nil {
			return nil, err
		}
		if r.TypeCode, err = o.char("type_code"); err != nil {
			return nil, err
		}
		r.DeviceSuffix = text("device_suffix")
		r.Locator = str("locator")
		if err != nil {
			return nil, err
		}
		if a, ok, aerr := o.array("legacy_telemetry"); aerr != nil {
			return nil, aerr
		} else if ok {
			for _, x := range a {
				n, ok := whole(x)
				if !ok || n < 0 || n > 255 {
					return nil, unsupported("legacy_telemetry: values are bytes, 0-255")
				}
				r.LegacyTelemetry = append(r.LegacyTelemetry, byte(n))
			}
		}
		ssid, serr := integerOf[uint8](o, "destination_ssid")
		if serr != nil {
			return nil, serr
		}
		if ssid != nil {
			r.DestinationSSID = *ssid
		}
		r.Fields, err = readPositioned(o)
		return r, err
	case "object":
		r := aprs.ObjectReport{Name: text("name"), Killed: flag("killed")}
		if err != nil {
			return nil, err
		}
		if r.Timestamp, err = readTimestamp(o); err != nil {
			return nil, err
		}
		r.Fields, err = readPositioned(o)
		return r, err
	case "item":
		r := aprs.ItemReport{Name: text("name"), Killed: flag("killed")}
		if err != nil {
			return nil, err
		}
		r.Fields, err = readPositioned(o)
		return r, err
	case "message":
		r := aprs.Message{Addressee: text("addressee"), Text: text("text"), MessageID: str("message_id"), ReplyAck: str("reply_ack")}
		return r, err
	case "ack":
		r := aprs.Ack{Addressee: text("addressee"), AckedID: text("acked_id"), MessageID: str("message_id"), ReplyAck: str("reply_ack")}
		return r, err
	case "reject":
		r := aprs.Reject{Addressee: text("addressee"), RejectedID: text("rejected_id"), MessageID: str("message_id"), ReplyAck: str("reply_ack")}
		return r, err
	case "bulletin":
		return aprs.Bulletin{Addressee: text("addressee"), Text: text("text"), MessageID: str("message_id")}, err
	case "nws-bulletin":
		return aprs.NWSBulletin{Addressee: text("addressee"), Text: text("text"), MessageID: str("message_id")}, err
	case "telemetry-names":
		r := aprs.TelemetryNames{Addressee: text("addressee")}
		if err != nil {
			return nil, err
		}
		if r.Names, err = o.strings("names"); err != nil {
			return nil, err
		}
		r.MessageID = str("message_id")
		return r, err
	case "telemetry-units":
		r := aprs.TelemetryUnits{Addressee: text("addressee")}
		if err != nil {
			return nil, err
		}
		if r.Units, err = o.strings("units"); err != nil {
			return nil, err
		}
		r.MessageID = str("message_id")
		return r, err
	case "telemetry-coefficients":
		r := aprs.TelemetryCoefficients{Addressee: text("addressee")}
		if err != nil {
			return nil, err
		}
		a, _, aerr := o.array("coefficients")
		if aerr != nil {
			return nil, aerr
		}
		for _, x := range a {
			f, ok := asFloat(x)
			if !ok {
				return nil, unsupported("coefficients: each is a number")
			}
			r.Coefficients = append(r.Coefficients, f)
		}
		r.MessageID = str("message_id")
		return r, err
	case "telemetry-bits":
		return aprs.TelemetryBits{Addressee: text("addressee"), Bits: text("bits"), Project: text("project"), MessageID: str("message_id")}, err
	case "directed-query":
		return aprs.DirectedQuery{Addressee: text("addressee"), QueryType: text("query_type"), Target: str("target")}, err
	case "status":
		r := aprs.Status{}
		if r.Timestamp, err = readTimestamp(o); err != nil {
			return nil, err
		}
		r.Locator = str("locator")
		if err != nil {
			return nil, err
		}
		if r.Symbol, err = readSymbol(o); err != nil {
			return nil, err
		}
		b, berr := o.object("beam", []string{"heading_code", "power_code"})
		if berr != nil {
			return nil, berr
		}
		if b != nil {
			h, herr := b.char("heading_code")
			if herr != nil {
				return nil, herr
			}
			if h == nil {
				return nil, b.missing("heading_code")
			}
			p, perr := b.char("power_code")
			if perr != nil {
				return nil, perr
			}
			if p == nil {
				return nil, b.missing("power_code")
			}
			r.Beam = &aprs.BeamHeading{HeadingCode: *h, PowerCode: *p}
		}
		r.Text = text("text")
		return r, err
	case "telemetry":
		r := aprs.Telemetry{}
		if v := o.get("sequence"); v != nil {
			s, ok := telemetryNumber(v)
			if !ok {
				return nil, unsupported("sequence: not text or a number")
			}
			r.Sequence = s
		}
		a, _, aerr := o.array("analog")
		if aerr != nil {
			return nil, aerr
		}
		for _, x := range a {
			if x == nil {
				r.Analog = append(r.Analog, nil)
				continue
			}
			f, ok := asFloat(x)
			if !ok {
				return nil, unsupported("analog: each is a number or null")
			}
			r.Analog = append(r.Analog, &f)
		}
		r.Bits = str("bits")
		r.Comment = text("comment")
		return r, err
	case "weather":
		r := aprs.WeatherReport{}
		if r.Timestamp, err = readTimestamp(o); err != nil {
			return nil, err
		}
		w, werr := readWeather(o)
		if werr != nil {
			return nil, werr
		}
		if w != nil {
			r.Weather = *w
		}
		r.Comment = text("comment")
		return r, err
	case "raw-weather":
		f, ferr := o.requiredChoice("format", []string{"peet-bros-hash", "peet-bros-star", "ultimeter-packet", "ultimeter-logging"})
		if ferr != nil {
			return nil, ferr
		}
		return aprs.RawWeather{Format: aprs.RawWeatherFormat(f), Data: text("data")}, err
	case "nmea":
		r := aprs.NMEA{Sentence: text("sentence"), HasChecksum: flag("has_checksum")}
		if err != nil {
			return nil, err
		}
		for _, f := range []struct {
			key string
			to  **float64
		}{{"latitude", &r.Latitude}, {"longitude", &r.Longitude}} {
			if *f.to, err = o.float(f.key); err != nil {
				return nil, err
			}
		}
		fix, ok, ferr := o.choice("fix", []string{"valid", "invalid"})
		if ferr != nil {
			return nil, ferr
		}
		if ok {
			r.FixValid = aprs.Ptr(fix == 0)
		}
		for _, f := range []struct {
			key string
			to  **float64
		}{{"course_degrees", &r.CourseDegrees}, {"speed_knots", &r.SpeedKnots}, {"altitude_m", &r.AltitudeM}} {
			if *f.to, err = o.float(f.key); err != nil {
				return nil, err
			}
		}
		r.Time = str("time")
		r.Waypoint = str("waypoint")
		r.Comment = text("comment")
		return r, err
	case "maidenhead-beacon":
		return aprs.MaidenheadBeacon{Locator: text("locator"), Comment: text("comment")}, err
	case "query":
		r := aprs.Query{QueryType: text("query_type")}
		if err != nil {
			return nil, err
		}
		// The neutral form has the numbers, not their text: the encoder writes each after a space
		// when it is positive (APRS12c ch. 15), and rounds the radius.
		f, ferr := o.object("footprint", []string{"latitude", "longitude", "radius_miles"})
		if ferr != nil {
			return nil, ferr
		}
		if f != nil {
			lat, e1 := f.requiredFloat("latitude")
			if e1 != nil {
				return nil, e1
			}
			lon, e2 := f.requiredFloat("longitude")
			if e2 != nil {
				return nil, e2
			}
			radius, e3 := f.requiredFloat("radius_miles")
			if e3 != nil {
				return nil, e3
			}
			fp := aprs.NewFootprint(lat, lon, radius)
			r.Footprint = &fp
		}
		return r, nil
	case "capabilities":
		r := aprs.Capabilities{}
		a, _, aerr := o.array("capabilities")
		if aerr != nil {
			return nil, aerr
		}
		for _, p := range a {
			pair, ok := p.([]any)
			bad := unsupported("capabilities: each is [token] or [token, value]")
			if !ok || len(pair) < 1 || len(pair) > 2 {
				return nil, bad
			}
			t, ok := pair[0].(string)
			if !ok {
				return nil, bad
			}
			c := aprs.Capability{Token: t}
			if len(pair) == 2 {
				v, ok := pair[1].(string)
				if !ok {
					return nil, bad
				}
				c.Value = &v
			}
			r.Capabilities = append(r.Capabilities, c)
		}
		return r, nil
	case "third-party":
		return readThirdParty(o)
	case "user-defined":
		byteText := func(key string) ([]byte, *Unreadable) {
			t, terr := o.text(key)
			if terr != nil {
				return nil, terr
			}
			var out []byte
			for _, r := range t {
				if r > 0xFF {
					return nil, unsupported("%s: user-defined data is bytes, U+0000-U+00FF", key)
				}
				out = append(out, byte(r))
			}
			return out, nil
		}
		u, uerr := o.char("user_id")
		if uerr != nil {
			return nil, uerr
		}
		if u == nil {
			return nil, o.missing("user_id")
		}
		p, perr := o.char("packet_type")
		if perr != nil {
			return nil, perr
		}
		if p == nil {
			return nil, o.missing("packet_type")
		}
		data, derr := byteText("data")
		if derr != nil {
			return nil, derr
		}
		return aprs.UserDefined{UserID: *u, PacketType: *p, Data: data}, nil
	case "test":
		return aprs.TestData{Data: text("data")}, err
	case "agrelo-df":
		b, berr := agreloBearing(o)
		if berr != nil {
			return nil, berr
		}
		q, qerr := agreloQuality(o)
		if qerr != nil {
			return nil, qerr
		}
		return aprs.AgreloDF{BearingDegrees: b, Quality: q}, nil
	}
	reason, ok, rerr := o.choice("reason", []string{"empty", "not-aprs", "reserved-data-type", "malformed"})
	if rerr != nil {
		return nil, rerr
	}
	if !ok {
		return aprs.UnrecognizedMalformed, nil
	}
	return aprs.Unrecognized(reason), nil
}

func readThirdParty(o *keys) (aprs.Data, *Unreadable) {
	// Count the levels of the input iteratively before recursing into any of them.
	depth := 1
	for cur := o.o; depth <= 5; depth++ {
		inner, _ := cur["packet"].(map[string]any)
		data, _ := inner["data"].(map[string]any)
		if t, _ := data["type"].(string); t != "third-party" {
			break
		}
		cur = data
	}
	if depth > 4 {
		return nil, refused(aprs.RefusalInvalidThirdPartyHeader, "third-party packets nest at most four deep")
	}
	// The inner packet as the neutral form gives it: its source may be any third-party source
	// (APRS12c ch. 17), and its diagnostics are part of the data. This package keeps the inner
	// information field as received, which the neutral form does not carry, so it is the inner
	// data encoded.
	p, err := o.object("packet", []string{"source", "destination", "path", "data", "diagnostics"})
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, o.missing("packet")
	}
	sourceText, err := p.text("source")
	if err != nil {
		return nil, err
	}
	source, serr := aprs.ThirdPartySource(sourceText)
	if serr != nil {
		return nil, unsupported("packet.source: not a third-party source")
	}
	destinationText, err := p.text("destination")
	if err != nil {
		return nil, err
	}
	if destinationText == "" {
		return nil, refused(aprs.RefusalInvalidThirdPartyHeader, "the third-party packet has no destination")
	}
	destination, derr := aprs.NewAddress(destinationText)
	if derr != nil {
		return nil, unsupported("packet.destination: not an address")
	}
	pathTexts, err := p.strings("path")
	if err != nil {
		return nil, err
	}
	var entries []aprs.PathEntry
	for _, e := range pathTexts {
		entry, ok := ReadPathEntry(e)
		if !ok {
			o.c.note(aprs.RefusalInvalidThirdPartyHeader, "path: not an address")
			continue
		}
		entries = append(entries, entry)
	}
	diagnosticTexts, err := p.strings("diagnostics")
	if err != nil {
		return nil, err
	}
	var diagnostics []aprs.Diagnostic
	for _, d := range diagnosticTexts {
		diagnostic, derr := readDiagnostic(d)
		if derr != nil {
			return nil, derr
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	dataValue := p.get("data")
	if dataValue == nil {
		return nil, p.missing("data")
	}
	data, err := readData(dataValue, o.c)
	if err != nil && !err.Refused {
		return nil, err
	}
	// not-encodable is the first refusal reason: nothing outranks it, a header defect included.
	if err != nil && err.Code == aprs.RefusalNotEncodable {
		return nil, err
	}
	// Undecoded data has no neutral form of its bytes: not-encodable, the first of the refusal
	// reasons, which no header fault outranks.
	if d, ok := data.(aprs.Unrecognized); ok && d != aprs.UnrecognizedEmpty {
		return nil, toolRefused(aprs.RefusalNotEncodable, "packet.data: undecoded, and its bytes are not in the neutral form")
	}
	// A header the encoder cannot write back is refused before the inner data's own faults are
	// named (invalid-third-party-header comes before them in the refusal order).
	headerDefect := false
	for _, d := range diagnostics {
		headerDefect = headerDefect || isHeaderDefect(d.Code)
	}
	last := -1
	for i, e := range entries {
		if e.Used {
			last = i
		}
	}
	usedOutOfOrder := false
	for i := 0; i < last; i++ {
		usedOutOfOrder = usedOutOfOrder || !entries[i].Used
	}
	if headerDefect || usedOutOfOrder {
		return nil, refused(aprs.RefusalInvalidThirdPartyHeader, "the third-party packet's header cannot be written back")
	}
	if err != nil {
		return nil, err
	}
	var information []byte
	switch d := data.(type) {
	case aprs.Unrecognized:
		if d != aprs.UnrecognizedEmpty {
			return nil, toolRefused(aprs.RefusalNotEncodable, "packet.data: undecoded, and its bytes are not in the neutral form")
		}
	default:
		info, eerr := aprs.Encode(data)
		if eerr != nil {
			e := eerr.(*aprs.EncodeError)
			return nil, refused(e.Code(), "the inner packet's data: "+e.Error())
		}
		information = info
	}
	inner := aprs.Decode(source, destination, entries, information, aprs.Lenient)
	inner.Data = data
	inner.Diagnostics = diagnostics
	inner.ThirdParty = true
	return aprs.ThirdParty{Packet: inner}, nil
}

// readDiagnostic reads a diagnostic from its neutral form, severity:code; the message and offset
// are not part of it.
func readDiagnostic(text string) (aprs.Diagnostic, *Unreadable) {
	severityText, codeText, ok := strings.Cut(text, ":")
	if !ok {
		return aprs.Diagnostic{}, unsupported("packet.diagnostics: not severity:code")
	}
	var severity aprs.Severity
	switch severityText {
	case "info":
		severity = aprs.Info
	case "warning":
		severity = aprs.Warning
	case "error":
		severity = aprs.Error
	default:
		return aprs.Diagnostic{}, unsupported("packet.diagnostics: not a severity")
	}
	code, ok := aprs.CodeFromID(codeText)
	if !ok {
		return aprs.Diagnostic{}, unsupported("packet.diagnostics: not a code this package has")
	}
	return aprs.Diagnostic{Severity: severity, Code: code, Offset: -1}, nil
}

// ReadPathEntry reads a path entry as the neutral form writes it, ADDRESS or ADDRESS*.
func ReadPathEntry(text string) (aprs.PathEntry, bool) {
	address, used := text, false
	if strings.HasSuffix(text, "*") {
		address, used = text[:len(text)-1], true
	}
	a, err := aprs.NewAddress(address)
	if err != nil {
		return aprs.PathEntry{}, false
	}
	return aprs.PathEntry{Address: a, Used: used}, true
}

func readTimestamp(o *keys) (*aprs.Timestamp, *Unreadable) {
	s, err := o.str("timestamp")
	if err != nil || s == nil {
		return nil, err
	}
	b := []byte(*s)
	if t, ok := aprs.ParseTimestamp(b); ok {
		return &t, nil
	}
	if t, ok := aprs.ParseTimestampMDHM(b); ok {
		return &t, nil
	}
	return nil, unsupported("timestamp: not a timestamp as on air")
}

func readSymbol(o *keys) (*aprs.Symbol, *Unreadable) {
	s, err := o.str("symbol")
	if err != nil || s == nil {
		return nil, err
	}
	r := []rune(*s)
	if len(r) != 2 {
		o.c.note(aprs.RefusalInvalidSymbol, "symbol: not a table and a code")
		return &aprs.Symbol{Table: '/', Code: '-'}, nil
	}
	return &aprs.Symbol{Table: r[0], Code: r[1]}, nil
}

func readMicEMessage(o *keys) (*aprs.MicEMessage, *Unreadable) {
	s, err := o.str("mic_e_message")
	if err != nil || s == nil {
		return nil, err
	}
	names := map[string]aprs.MicEMessage{
		"off-duty": aprs.MicEOffDuty, "en-route": aprs.MicEEnRoute, "in-service": aprs.MicEInService,
		"returning": aprs.MicEReturning, "committed": aprs.MicECommitted, "special": aprs.MicESpecial,
		"priority": aprs.MicEPriority, "emergency": aprs.MicEEmergency, "unknown": aprs.MicEUnknown,
	}
	if m, ok := names[*s]; ok {
		return &m, nil
	}
	if n, ok := strings.CutPrefix(*s, "custom"); ok {
		if v, err := parseU8(n); err == nil {
			m := aprs.MicECustom(v)
			return &m, nil
		}
	}
	return nil, unsupported("mic_e_message: not a value this package has")
}

// parseU8 reads a u8 as Rust's str::parse does: an optional +, then digits, at most 255.
func parseU8(s string) (uint8, error) {
	t := strings.TrimPrefix(s, "+")
	if t == "" || strings.ContainsAny(t, "+-") {
		return 0, fmt.Errorf("not a u8")
	}
	n, err := strconv.ParseUint(t, 10, 8)
	return uint8(n), err
}

func readPositioned(o *keys) (aprs.Positioned, *Unreadable) {
	f := aprs.Positioned{}
	var err *Unreadable
	if f.Position.Latitude, err = o.requiredFloat("latitude"); err != nil {
		return f, err
	}
	if f.Position.Longitude, err = o.requiredFloat("longitude"); err != nil {
		return f, err
	}
	ambiguity, err := integerOf[uint8](o, "ambiguity")
	if err != nil {
		return f, err
	}
	if ambiguity != nil {
		f.Position.Ambiguity = *ambiguity
	}
	symbol, err := readSymbol(o)
	if err != nil {
		return f, err
	}
	// A symbol the neutral data leaves out is empty text: the zero Symbol, which the encoder
	// refuses as invalid-symbol.
	if symbol != nil {
		f.Symbol = *symbol
	}
	if f.Compressed, err = o.flag("compressed"); err != nil {
		return f, err
	}
	if c, err := o.object("compression", []string{"fix", "source", "origin"}); err != nil {
		return f, err
	} else if c != nil {
		fix, err := c.requiredChoice("fix", fixNames[:])
		if err != nil {
			return f, err
		}
		source, err := c.requiredChoice("source", sourceNames[:])
		if err != nil {
			return f, err
		}
		origin, err := c.requiredChoice("origin", originNames[:])
		if err != nil {
			return f, err
		}
		f.Compression = &aprs.CompressionType{Fix: aprs.CompressionFix(fix), Source: aprs.CompressionSource(source), Origin: aprs.CompressionOrigin(origin)}
	}
	if f.CourseDegrees, err = o.float("course_degrees"); err != nil {
		return f, err
	}
	if f.SpeedKnots, err = o.float("speed_knots"); err != nil {
		return f, err
	}
	if f.AltitudeFeet, err = o.float("altitude_feet"); err != nil {
		return f, err
	}
	if p, err := o.object("phg", []string{"power", "height", "gain", "directivity", "beacons_per_hour"}); err != nil {
		return f, err
	} else if p != nil {
		phg := aprs.PHG{}
		for _, x := range []struct {
			key string
			to  *uint8
		}{{"power", &phg.Power}, {"height", &phg.Height}, {"gain", &phg.Gain}, {"directivity", &phg.Directivity}} {
			if *x.to, err = requiredInteger[uint8](p, x.key); err != nil {
				return f, err
			}
		}
		if phg.BeaconsPerHour, err = integerOf[uint8](p, "beacons_per_hour"); err != nil {
			return f, err
		}
		f.PHG = &phg
	}
	if f.RangeMiles, err = o.float("range_miles"); err != nil {
		return f, err
	}
	if d, err := o.object("dfs", []string{"strength", "height", "gain", "directivity"}); err != nil {
		return f, err
	} else if d != nil {
		dfs := aprs.DFS{}
		for _, x := range []struct {
			key string
			to  *uint8
		}{{"strength", &dfs.Strength}, {"height", &dfs.Height}, {"gain", &dfs.Gain}, {"directivity", &dfs.Directivity}} {
			if *x.to, err = requiredInteger[uint8](d, x.key); err != nil {
				return f, err
			}
		}
		f.DFS = &dfs
	}
	if a, err := o.object("area", []string{"shape", "lat_offset", "color", "lon_offset", "corridor_width_miles"}); err != nil {
		return f, err
	} else if a != nil {
		area := aprs.AreaObject{}
		shape, err := a.requiredChoice("shape", shapeNames[:])
		if err != nil {
			return f, err
		}
		area.Shape = aprs.AreaShape(shape)
		if area.LatOffset, err = requiredInteger[uint8](a, "lat_offset"); err != nil {
			return f, err
		}
		color, err := a.requiredChoice("color", colorNames[:])
		if err != nil {
			return f, err
		}
		area.Color = aprs.AreaColor(color)
		if area.LonOffset, err = requiredInteger[uint8](a, "lon_offset"); err != nil {
			return f, err
		}
		if area.CorridorWidthMiles, err = integerOf[uint16](a, "corridor_width_miles"); err != nil {
			return f, err
		}
		f.Area = &area
	}
	if b, err := o.object("df_bearing", []string{"bearing_degrees", "number", "range", "quality"}); err != nil {
		return f, err
	} else if b != nil {
		bearing := aprs.DFBearing{}
		if bearing.BearingDegrees, err = requiredInteger[uint16](b, "bearing_degrees"); err != nil {
			return f, err
		}
		for _, x := range []struct {
			key string
			to  *uint8
		}{{"number", &bearing.Number}, {"range", &bearing.Range}, {"quality", &bearing.Quality}} {
			if *x.to, err = requiredInteger[uint8](b, x.key); err != nil {
				return f, err
			}
		}
		f.DFBearing = &bearing
	}
	if s, err := o.object("storm", []string{"type", "sustained_wind_knots", "gust_knots", "central_pressure_mbar", "hurricane_radius_nm", "tropical_storm_radius_nm", "whole_gale_radius_nm"}); err != nil {
		return f, err
	} else if s != nil {
		storm := aprs.Storm{}
		kind, err := s.requiredChoice("type", stormNames[:])
		if err != nil {
			return f, err
		}
		storm.Kind = aprs.StormKind(kind)
		for _, x := range []struct {
			key string
			to  **uint16
		}{
			{"sustained_wind_knots", &storm.SustainedWindKnots}, {"gust_knots", &storm.GustKnots},
			{"central_pressure_mbar", &storm.CentralPressureMbar}, {"hurricane_radius_nm", &storm.HurricaneRadiusNM},
			{"tropical_storm_radius_nm", &storm.TropicalStormRadiusNM}, {"whole_gale_radius_nm", &storm.WholeGaleRadiusNM},
		} {
			if *x.to, err = integerOf[uint16](s, x.key); err != nil {
				return f, err
			}
		}
		f.Storm = &storm
	}
	if d, err := o.object("dao", []string{"datum", "precision"}); err != nil {
		return f, err
	} else if d != nil {
		datum, err := d.char("datum")
		if err != nil {
			return f, err
		}
		if datum == nil {
			return f, d.missing("datum")
		}
		precision, _, err := d.choice("precision", daoNames[:])
		if err != nil {
			return f, err
		}
		f.DAO = &aprs.DAO{Datum: *datum, Precision: aprs.DAOPrecision(precision)}
	}
	if t, err := o.object("telemetry", []string{"sequence", "analog", "digital"}); err != nil {
		return f, err
	} else if t != nil {
		telemetry := aprs.CommentTelemetry{}
		if telemetry.Sequence, err = requiredInteger[uint16](t, "sequence"); err != nil {
			return f, err
		}
		a, _, err := t.array("analog")
		if err != nil {
			return f, err
		}
		for _, x := range a {
			n, ok := whole(x)
			if !ok || n < 0 || n > 65535 {
				return f, unsupported("telemetry.analog: each is a whole number, 0-65535")
			}
			telemetry.Analog = append(telemetry.Analog, uint16(n))
		}
		if telemetry.Digital, err = integerOf[uint8](t, "digital"); err != nil {
			return f, err
		}
		f.Telemetry = &telemetry
	}
	if v, err := o.object("frequency", []string{"mhz", "tone", "tone_value", "offset_khz", "range", "range_km", "narrow", "ten_khz_resolution"}); err != nil {
		return f, err
	} else if v != nil {
		frequency := aprs.VoiceFrequency{}
		if frequency.MHz, err = v.requiredFloat("mhz"); err != nil {
			return f, err
		}
		tone, ok, err := v.choice("tone", toneNames[:])
		if err != nil {
			return f, err
		}
		if ok {
			frequency.Tone = aprs.Ptr(aprs.Tone(tone))
		}
		if frequency.ToneValue, err = integerOf[uint16](v, "tone_value"); err != nil {
			return f, err
		}
		if frequency.OffsetKHz, err = integerOf[int32](v, "offset_khz"); err != nil {
			return f, err
		}
		if frequency.Range, err = integerOf[uint16](v, "range"); err != nil {
			return f, err
		}
		if frequency.RangeKm, err = v.flag("range_km"); err != nil {
			return f, err
		}
		if frequency.Narrow, err = v.flag("narrow"); err != nil {
			return f, err
		}
		if frequency.TenKHzResolution, err = v.flag("ten_khz_resolution"); err != nil {
			return f, err
		}
		f.Frequency = &frequency
	}
	if f.Weather, err = readWeather(o); err != nil {
		return f, err
	}
	if f.Signpost, err = o.str("signpost"); err != nil {
		return f, err
	}
	f.Comment, err = o.text("comment")
	return f, err
}

func readWeather(o *keys) (*aprs.Weather, *Unreadable) {
	w, err := o.object("weather", []string{
		"wind_direction_degrees", "wind_speed_mph", "wind_gust_mph", "temperature_f", "rain_1h_in", "rain_24h_in",
		"rain_midnight_in", "rain_raw", "humidity_percent", "pressure_mbar", "luminosity_w_m2", "snow_24h_in",
		"software", "unit", "extra",
	})
	if err != nil || w == nil {
		return nil, err
	}
	out := aprs.Weather{}
	a, _, err := w.array("extra")
	if err != nil {
		return nil, err
	}
	for _, e := range a {
		ek, err := newKeys(e, "weather.extra", []string{"letter", "value"})
		if err != nil {
			return nil, err
		}
		letter, err := ek.char("letter")
		if err != nil {
			return nil, err
		}
		if letter == nil {
			return nil, ek.missing("letter")
		}
		value, err := ek.text("value")
		if err != nil {
			return nil, err
		}
		out.Extra = append(out.Extra, aprs.WeatherField{Letter: *letter, Value: value})
	}
	for _, x := range []struct {
		key string
		to  **float64
	}{
		{"wind_direction_degrees", &out.WindDirectionDegrees}, {"wind_speed_mph", &out.WindSpeedMph},
		{"wind_gust_mph", &out.WindGustMph}, {"temperature_f", &out.TemperatureF}, {"rain_1h_in", &out.Rain1hIn},
		{"rain_24h_in", &out.Rain24hIn}, {"rain_midnight_in", &out.RainMidnightIn},
	} {
		if *x.to, err = w.float(x.key); err != nil {
			return nil, err
		}
	}
	if out.RainRaw, err = stepped[uint32](w, "rain_raw"); err != nil {
		return nil, err
	}
	for _, x := range []struct {
		key string
		to  **float64
	}{
		{"humidity_percent", &out.HumidityPercent}, {"pressure_mbar", &out.PressureMbar},
		{"luminosity_w_m2", &out.LuminosityWM2}, {"snow_24h_in", &out.Snow24hIn},
	} {
		if *x.to, err = w.float(x.key); err != nil {
			return nil, err
		}
	}
	if out.Software, err = w.char("software"); err != nil {
		return nil, err
	}
	if out.Unit, err = w.str("unit"); err != nil {
		return nil, err
	}
	return &out, nil
}

// agreloBearing reads an Agrelo bearing as an encoder takes it: rounded to the nearest degree like
// a course (halves away from zero), then range-checked. This package holds 0 to 65535, and the
// encoder judges 0 to 360; a bearing past what is held, or not a number, is a refusal made here
// (value-out-of-range, not-a-number) with 0 as the stand-in.
func agreloBearing(o *keys) (uint16, *Unreadable) {
	v := o.get("bearing_degrees")
	if v == nil {
		return 0, o.missing("bearing_degrees")
	}
	x, ok := asFloat(v)
	if !ok {
		return 0, unsupported("%s: not a number", o.name("bearing_degrees"))
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		o.c.note(aprs.RefusalNotANumber, "bearing_degrees: not a number")
		return 0, nil
	}
	r := roundSteps(x)
	if r < 0 || r > math.MaxUint16 {
		o.c.note(aprs.RefusalValueOutOfRange, "bearing_degrees: outside 0-360 once rounded")
		return 0, nil
	}
	return uint16(r), nil
}

// agreloQuality reads an Agrelo quality, which is not rounded: a whole number, held 0 to 255, and
// one outside 0-9 is value-out-of-range, a non-finite one not-a-number.
func agreloQuality(o *keys) (uint8, *Unreadable) {
	v := o.get("quality")
	if v == nil {
		return 0, o.missing("quality")
	}
	x, ok := asFloat(v)
	if !ok {
		return 0, unsupported("%s: not a number", o.name("quality"))
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		o.c.note(aprs.RefusalNotANumber, "quality: not a number")
		return 0, nil
	}
	if x != math.Trunc(x) {
		if x < 0 || x > 9 {
			o.c.note(aprs.RefusalValueOutOfRange, "quality: outside 0-9")
			return 0, nil
		}
		return 0, unsupported("%s: not a whole number", o.name("quality"))
	}
	if x < 0 || x > 9 {
		o.c.note(aprs.RefusalValueOutOfRange, "quality: outside 0-9")
		return 0, nil
	}
	return uint8(x), nil
}
