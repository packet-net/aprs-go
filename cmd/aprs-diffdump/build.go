package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	aprs "github.com/packet-net/aprs-go"
)

// notSupported says a recipe was not built because the builder has no way to say something in
// it, naming the key.
type notSupported struct{ reason string }

func (e *notSupported) Error() string { return e.reason }

func unsupported(format string, a ...any) error {
	return &notSupported{reason: fmt.Sprintf(format, a...)}
}

// recipeArgs is a recipe's arguments. Each one read is marked taken, so that one no step read
// (something the builder has no way to say) can be named.
type recipeArgs struct {
	o     map[string]any
	taken map[string]bool
}

func newArgs(v any) (*recipeArgs, error) {
	o, ok := v.(map[string]any)
	if !ok {
		return nil, unsupported("args: not an object")
	}
	return &recipeArgs{o: o, taken: map[string]bool{}}, nil
}

func (a *recipeArgs) get(k string) any {
	v, ok := a.o[k]
	if !ok {
		return nil
	}
	a.taken[k] = true
	return v
}

func (a *recipeArgs) has(k string) bool {
	return a.o[k] != nil
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func (a *recipeArgs) float(k string) (*float64, error) {
	v := a.get(k)
	if v == nil {
		return nil, nil
	}
	f, ok := asFloat(v)
	if !ok {
		return nil, unsupported("%s: not a number", k)
	}
	return &f, nil
}

func (a *recipeArgs) requiredFloat(k string) (float64, error) {
	f, err := a.float(k)
	if err != nil {
		return 0, err
	}
	if f == nil {
		return 0, unsupported("%s: missing", k)
	}
	return *f, nil
}

// whole is a JSON number that is a whole number, as serde_json's as_i64, or a float with no
// fraction below 1e15.
func whole(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return int64(x), true
		}
	}
	return 0, false
}

// wholeIn reads a whole number between lo and hi, the range of the type the builder's method
// takes.
func (a *recipeArgs) wholeIn(k string, lo, hi int64, what string) (*int64, error) {
	v := a.get(k)
	if v == nil {
		return nil, nil
	}
	n, ok := whole(v)
	if !ok || n < lo || n > hi {
		return nil, unsupported("%s: the builder takes %s", k, what)
	}
	return &n, nil
}

func (a *recipeArgs) text(k string) (*string, error) {
	switch v := a.get(k).(type) {
	case nil:
		return nil, nil
	case string:
		return &v, nil
	}
	return nil, unsupported("%s: not text", k)
}

func (a *recipeArgs) textOr(k string) (string, error) {
	t, err := a.text(k)
	if err != nil || t == nil {
		return "", err
	}
	return *t, nil
}

func (a *recipeArgs) flag(k string) (bool, error) {
	switch v := a.get(k).(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	}
	return false, unsupported("%s: not a boolean", k)
}

// allTaken fails naming the first argument no step read.
func (a *recipeArgs) allTaken() error {
	keys := make([]string, 0, len(a.o))
	for k := range a.o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !a.taken[k] {
			return unsupported("%s: the builder has no such option", k)
		}
	}
	return nil
}

// positionedBuilder is the methods every positioned builder shares.
type positionedBuilder[B any] interface {
	Symbol(aprs.Symbol) B
	CourseDegrees(float64) B
	SpeedKnots(float64) B
	SpeedKmh(float64) B
	AltitudeFeet(float64) B
	AltitudeM(float64) B
	Comment(string) B
	PHG(aprs.PHG) B
	RangeMiles(float64) B
	Frequency(float64) B
	Tone(aprs.Tone, uint16) B
	OffsetKHz(int32) B
	Compressed() B
	Ambiguity(uint8) B
	DAO() B
	Telemetry(aprs.CommentTelemetry) B
}

// positioned applies the options every positioned builder shares.
func positioned[B positionedBuilder[B]](b B, a *recipeArgs) (B, error) {
	s, err := a.text("symbol")
	if err != nil {
		return b, err
	}
	if sym, err := symbolOf(s); err != nil {
		return b, err
	} else if sym != nil {
		b = b.Symbol(*sym)
	}
	if c, err := a.float("course_degrees"); err != nil {
		return b, err
	} else if c != nil {
		b = b.CourseDegrees(*c)
	}
	if a.has("speed_knots") && a.has("speed_kmh") {
		return b, unsupported("speed_kmh: the builder takes one speed")
	}
	if v, err := a.float("speed_knots"); err != nil {
		return b, err
	} else if v != nil {
		b = b.SpeedKnots(*v)
	}
	if v, err := a.float("speed_kmh"); err != nil {
		return b, err
	} else if v != nil {
		b = b.SpeedKmh(*v)
	}
	if a.has("altitude_feet") && a.has("altitude_m") {
		return b, unsupported("altitude_m: the builder takes one altitude")
	}
	if v, err := a.float("altitude_feet"); err != nil {
		return b, err
	} else if v != nil {
		b = b.AltitudeFeet(*v)
	}
	if v, err := a.float("altitude_m"); err != nil {
		return b, err
	} else if v != nil {
		b = b.AltitudeM(*v)
	}
	if c, err := a.text("comment"); err != nil {
		return b, err
	} else if c != nil {
		b = b.Comment(*c)
	}
	if p := a.get("phg"); p != nil {
		phg, err := phgOf(p)
		if err != nil {
			return b, err
		}
		b = b.PHG(phg)
	}
	if v, err := a.float("range_miles"); err != nil {
		return b, err
	} else if v != nil {
		b = b.RangeMiles(*v)
	}
	if f := a.get("frequency"); f != nil {
		fr, err := frequencyOf(f)
		if err != nil {
			return b, err
		}
		b = b.Frequency(fr.mhz)
		// A tone value with no kind is the plain tone, as it always was.
		if fr.value != nil {
			v := *fr.value
			if v != math.Trunc(v) || !(v >= 0 && v < 65536) {
				return b, unsupported("frequency.tone_value: the builder takes a whole number")
			}
			tone := aprs.ToneTone
			if fr.tone != nil {
				tone = *fr.tone
			}
			b = b.Tone(tone, uint16(v))
		} else if fr.tone != nil {
			return b, unsupported("frequency.tone: the builder takes a tone with its value")
		}
		if fr.offset != nil {
			b = b.OffsetKHz(*fr.offset)
		}
	}
	if c, err := a.flag("compressed"); err != nil {
		return b, err
	} else if c {
		b = b.Compressed()
	}
	if v, err := a.wholeIn("ambiguity", 0, 255, "0-255 digits"); err != nil {
		return b, err
	} else if v != nil {
		b = b.Ambiguity(uint8(*v))
	}
	if d, err := a.flag("dao"); err != nil {
		return b, err
	} else if d {
		b = b.DAO()
	}
	if t := a.get("telemetry"); t != nil {
		ct, err := commentTelemetryOf(t)
		if err != nil {
			return b, err
		}
		b = b.Telemetry(ct)
	}
	return b, nil
}

func build(input map[string]any) (*aprs.Packet, error) {
	station, err := stationOf(input["station"])
	if err != nil {
		return nil, err
	}
	args, err := newArgs(input["args"])
	if err != nil {
		return nil, err
	}
	report, _ := input["report"].(string)
	switch report {
	case "position":
		lat, err := args.requiredFloat("latitude")
		if err != nil {
			return nil, err
		}
		lon, err := args.requiredFloat("longitude")
		if err != nil {
			return nil, err
		}
		b, err := positioned(station.Position(lat, lon), args)
		if err != nil {
			return nil, err
		}
		if m, err := args.flag("messaging"); err != nil {
			return nil, err
		} else if m {
			b = b.Messaging()
		}
		if t, err := timestampOf(args); err != nil {
			return nil, err
		} else if t != nil {
			b = b.Timestamp(*t)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "object":
		name, err := args.textOr("name")
		if err != nil {
			return nil, err
		}
		b := station.Object(name)
		lat, err := args.requiredFloat("latitude")
		if err != nil {
			return nil, err
		}
		lon, err := args.requiredFloat("longitude")
		if err != nil {
			return nil, err
		}
		if b, err = positioned(b.At(lat, lon), args); err != nil {
			return nil, err
		}
		if t, err := timestampOf(args); err != nil {
			return nil, err
		} else if t != nil {
			b = b.Timestamp(*t)
		}
		if k, err := args.flag("killed"); err != nil {
			return nil, err
		} else if k {
			b = b.Killed()
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "item":
		name, err := args.textOr("name")
		if err != nil {
			return nil, err
		}
		b := station.Item(name)
		lat, err := args.requiredFloat("latitude")
		if err != nil {
			return nil, err
		}
		lon, err := args.requiredFloat("longitude")
		if err != nil {
			return nil, err
		}
		if b, err = positioned(b.At(lat, lon), args); err != nil {
			return nil, err
		}
		if k, err := args.flag("killed"); err != nil {
			return nil, err
		} else if k {
			b = b.Killed()
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "mic-e":
		lat, err := args.requiredFloat("latitude")
		if err != nil {
			return nil, err
		}
		lon, err := args.requiredFloat("longitude")
		if err != nil {
			return nil, err
		}
		b, err := positioned(station.MicE(lat, lon), args)
		if err != nil {
			return nil, err
		}
		if m, err := args.text("mic_e_message"); err != nil {
			return nil, err
		} else if m != nil {
			msg, err := micEMessageOf(*m)
			if err != nil {
				return nil, err
			}
			b = b.MicEMessage(msg)
		}
		if m, err := args.flag("messaging"); err != nil {
			return nil, err
		} else if m {
			b = b.Messaging()
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "weather":
		return weatherOf(station, args)
	case "message":
		addressee, err := args.textOr("addressee")
		if err != nil {
			return nil, err
		}
		text, err := args.textOr("text")
		if err != nil {
			return nil, err
		}
		b := station.Message(addressee, text)
		if id, err := args.text("message_id"); err != nil {
			return nil, err
		} else if id != nil {
			b = b.MessageID(*id)
		}
		if r, err := args.text("reply_ack"); err != nil {
			return nil, err
		} else if r != nil {
			b = b.ReplyAck(*r)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "ack", "reject":
		addressee, err := args.textOr("addressee")
		if err != nil {
			return nil, err
		}
		id, err := args.textOr("message_id")
		if err != nil {
			return nil, err
		}
		b := station.Ack(addressee, id)
		if report == "reject" {
			b = station.Reject(addressee, id)
		}
		if r, err := args.text("reply_ack"); err != nil {
			return nil, err
		} else if r != nil {
			b = b.ReplyAck(*r)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "bulletin":
		idText, err := args.textOr("id")
		if err != nil {
			return nil, err
		}
		id, ok := oneChar(idText)
		if !ok {
			return nil, unsupported("id: the builder takes one character")
		}
		text, err := args.textOr("text")
		if err != nil {
			return nil, err
		}
		group, err := args.text("group")
		if err != nil {
			return nil, err
		}
		b := station.Bulletin(id, text)
		if group != nil {
			b = station.GroupBulletin(id, *group, text)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "status":
		text, err := args.textOr("text")
		if err != nil {
			return nil, err
		}
		b := station.Status(text)
		if t, err := timestampOf(args); err != nil {
			return nil, err
		} else if t != nil {
			b = b.Timestamp(*t)
		}
		if l, err := args.text("locator"); err != nil {
			return nil, err
		} else if l != nil {
			b = b.Locator(*l)
		}
		s, err := args.text("symbol")
		if err != nil {
			return nil, err
		}
		if sym, err := symbolOf(s); err != nil {
			return nil, err
		} else if sym != nil {
			b = b.Symbol(*sym)
		}
		if beam := args.get("beam"); beam != nil {
			code := func(k string) (rune, error) {
				if o, ok := beam.(map[string]any); ok {
					if s, ok := o[k].(string); ok {
						if c, ok := oneChar(s); ok {
							return c, nil
						}
					}
				}
				return 0, unsupported("beam.%s: the builder takes one character", k)
			}
			h, err := code("heading_code")
			if err != nil {
				return nil, err
			}
			p, err := code("power_code")
			if err != nil {
				return nil, err
			}
			b = b.Beam(aprs.BeamHeading{HeadingCode: h, PowerCode: p})
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "telemetry":
		sequence, err := args.wholeIn("sequence", 0, 65535, "a number 0-65535")
		if err != nil {
			return nil, err
		}
		var seq uint16
		if sequence != nil {
			seq = uint16(*sequence)
		}
		b := station.Telemetry(seq)
		if v := args.get("analog"); v != nil {
			list, ok := v.([]any)
			if !ok {
				return nil, unsupported("analog: not a list")
			}
			// A channel may be empty on its own, and the ones not given are sent empty.
			given := 0
			for i, x := range list {
				if x != nil {
					given = i + 1
				}
			}
			values := make([]*float64, 0, given)
			for _, x := range list[:given] {
				if x == nil {
					values = append(values, nil)
					continue
				}
				f, ok := asFloat(x)
				if !ok {
					return nil, unsupported("analog: not a number")
				}
				values = append(values, &f)
			}
			b = b.Analog(values...)
		}
		if bits, err := args.text("bits"); err != nil {
			return nil, err
		} else if bits != nil {
			b = b.Bits(*bits)
		}
		if c, err := args.text("comment"); err != nil {
			return nil, err
		} else if c != nil {
			b = b.Comment(*c)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "telemetry-names", "telemetry-units":
		addressee, err := args.text("addressee")
		if err != nil {
			return nil, err
		}
		key := "units"
		if report == "telemetry-names" {
			key = "names"
		}
		labels, err := stringsOf(args, key)
		if err != nil {
			return nil, err
		}
		b := station.TelemetryUnits(labels...)
		if report == "telemetry-names" {
			b = station.TelemetryNames(labels...)
		}
		if addressee != nil {
			b = b.Addressee(*addressee)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "telemetry-coefficients":
		addressee, err := args.text("addressee")
		if err != nil {
			return nil, err
		}
		var coefficients []float64
		if v := args.get("coefficients"); v != nil {
			list, ok := v.([]any)
			if !ok {
				return nil, unsupported("coefficients: the builder takes numbers")
			}
			for _, x := range list {
				f, ok := asFloat(x)
				if !ok {
					return nil, unsupported("coefficients: the builder takes numbers")
				}
				coefficients = append(coefficients, f)
			}
		}
		b := station.TelemetryCoefficients(coefficients...)
		if addressee != nil {
			b = b.Addressee(*addressee)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	case "telemetry-bits":
		addressee, err := args.text("addressee")
		if err != nil {
			return nil, err
		}
		bits, err := args.textOr("bits")
		if err != nil {
			return nil, err
		}
		project, err := args.textOr("project")
		if err != nil {
			return nil, err
		}
		b := station.TelemetryBits(bits, project)
		if addressee != nil {
			b = b.Addressee(*addressee)
		}
		if err := args.allTaken(); err != nil {
			return nil, err
		}
		return b.Build()
	}
	return nil, unsupported("report: the builder has no such report")
}

func stationOf(v any) (aprs.Station, error) {
	o, _ := v.(map[string]any)
	text := func(k string) (string, bool) {
		s, ok := o[k].(string)
		return s, ok
	}
	source, _ := text("source")
	station, err := aprs.NewStation(source)
	if err != nil {
		return station, err
	}
	if d, ok := text("destination"); ok {
		if station, err = station.To(d); err != nil {
			return station, err
		}
	}
	if path, ok := o["path"].([]any); ok {
		var entries []string
		for _, e := range path {
			if s, ok := e.(string); ok {
				entries = append(entries, s)
			}
		}
		if station, err = station.Via(entries...); err != nil {
			return station, err
		}
	}
	return station, nil
}

func stringsOf(a *recipeArgs, k string) ([]string, error) {
	switch v := a.get(k).(type) {
	case nil:
		return nil, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				return nil, unsupported("%s: not text", k)
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, unsupported("%s: not a list", k)
}

func oneChar(t string) (rune, bool) {
	if utf8.RuneCountInString(t) != 1 {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(t)
	return r, true
}

func symbolOf(t *string) (*aprs.Symbol, error) {
	if t == nil {
		return nil, nil
	}
	r := []rune(*t)
	if len(r) != 2 {
		return nil, unsupported("symbol: the builder takes a table and a code")
	}
	s := aprs.NewSymbol(r[0], r[1])
	return &s, nil
}

func phgOf(p any) (aprs.PHG, error) {
	o, _ := p.(map[string]any)
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "power", "height", "gain", "directivity", "beacons_per_hour":
		default:
			return aprs.PHG{}, unsupported("phg.%s: the builder has no such code", k)
		}
	}
	code := func(k string) (uint8, error) {
		if n, ok := whole(o[k]); ok && n >= 0 && n <= 255 {
			return uint8(n), nil
		}
		return 0, unsupported("phg.%s: the builder takes a code 0-255", k)
	}
	var rate *uint8
	if o["beacons_per_hour"] != nil {
		r, err := code("beacons_per_hour")
		if err != nil {
			return aprs.PHG{}, err
		}
		rate = &r
	}
	power, err := code("power")
	if err != nil {
		return aprs.PHG{}, err
	}
	height, err := code("height")
	if err != nil {
		return aprs.PHG{}, err
	}
	gain, err := code("gain")
	if err != nil {
		return aprs.PHG{}, err
	}
	directivity, err := code("directivity")
	if err != nil {
		return aprs.PHG{}, err
	}
	return aprs.PHG{Power: power, Height: height, Gain: gain, Directivity: directivity, BeaconsPerHour: rate}, nil
}

// recipeFrequency is a recipe's frequency.
type recipeFrequency struct {
	mhz  float64
	tone *aprs.Tone
	// value is the tone's value, in Hz or the DCS code.
	value  *float64
	offset *int32
}

func frequencyOf(f any) (recipeFrequency, error) {
	o, _ := f.(map[string]any)
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "mhz", "tone", "tone_value", "offset_khz":
		default:
			return recipeFrequency{}, unsupported("frequency.%s: the builder has no such option", k)
		}
	}
	mhz, ok := asFloat(o["mhz"])
	if !ok {
		return recipeFrequency{}, unsupported("frequency.mhz: missing")
	}
	r := recipeFrequency{mhz: mhz}
	if t, ok := o["tone"].(string); ok {
		var tone aprs.Tone
		switch t {
		case "tone":
			tone = aprs.ToneTone
		case "ctcss":
			tone = aprs.ToneCTCSS
		case "dcs":
			tone = aprs.ToneDCS
		default:
			return recipeFrequency{}, unsupported("frequency.tone: the builder has no such tone")
		}
		r.tone = &tone
	}
	if v, ok := asFloat(o["tone_value"]); ok {
		r.value = &v
	}
	if v := o["offset_khz"]; v != nil {
		n, ok := whole(v)
		if !ok || n < math.MinInt32 || n > math.MaxInt32 {
			return recipeFrequency{}, unsupported("frequency.offset_khz: the builder takes whole kHz")
		}
		offset := int32(n)
		r.offset = &offset
	}
	return r, nil
}

// commentTelemetryOf reads base-91 comment telemetry: the sequence, the analog values and any
// digital bits.
func commentTelemetryOf(t any) (aprs.CommentTelemetry, error) {
	o, _ := t.(map[string]any)
	value := func(v any) (uint16, bool) {
		n, ok := whole(v)
		if !ok || n < 0 || n > 65535 {
			return 0, false
		}
		return uint16(n), true
	}
	sequence, ok := value(o["sequence"])
	if !ok {
		return aprs.CommentTelemetry{}, unsupported("telemetry.sequence: the builder takes 0-65535")
	}
	ct := aprs.CommentTelemetry{Sequence: sequence}
	if list, ok := o["analog"].([]any); ok {
		for _, x := range list {
			v, ok := value(x)
			if !ok {
				return aprs.CommentTelemetry{}, unsupported("telemetry.analog: the builder takes values 0-65535")
			}
			ct.Analog = append(ct.Analog, v)
		}
	}
	if d := o["digital"]; d != nil {
		n, ok := whole(d)
		if !ok || n < 0 || n > 255 {
			return aprs.CommentTelemetry{}, unsupported("telemetry.digital: the builder takes 0-255")
		}
		digital := uint8(n)
		ct.Digital = &digital
	}
	return ct, nil
}

func micEMessageOf(t string) (aprs.MicEMessage, error) {
	switch t {
	case "off-duty":
		return aprs.MicEOffDuty, nil
	case "en-route":
		return aprs.MicEEnRoute, nil
	case "in-service":
		return aprs.MicEInService, nil
	case "returning":
		return aprs.MicEReturning, nil
	case "committed":
		return aprs.MicECommitted, nil
	case "special":
		return aprs.MicESpecial, nil
	case "priority":
		return aprs.MicEPriority, nil
	case "emergency":
		return aprs.MicEEmergency, nil
	}
	if n, ok := strings.CutPrefix(t, "custom"); ok {
		if v, ok := parseU8(n); ok {
			return aprs.MicECustom(v), nil
		}
	}
	return 0, unsupported("mic_e_message: the builder has no such message")
}

// parseU8 reads a u8 as Rust's str::parse does: an optional +, then digits, at most 255.
func parseU8(s string) (uint8, bool) {
	t := strings.TrimPrefix(s, "+")
	if t == "" || strings.ContainsAny(t, "+-") {
		return 0, false
	}
	n, err := strconv.ParseUint(t, 10, 8)
	return uint8(n), err == nil
}

// timestampOf reads {"utc": "2026-09-27T09:23:45Z", "format": "dhm"} as the timestamp a program
// would give the builder: day, hours and minutes (dhm), hours, minutes and seconds (hms), or month,
// day, hours and minutes (mdhm).
func timestampOf(a *recipeArgs) (*aprs.Timestamp, error) {
	t := a.get("timestamp")
	if t == nil {
		return nil, nil
	}
	o, _ := t.(map[string]any)
	utc, _ := o["utc"].(string)
	field := func(at int) (uint8, error) {
		if at+2 <= len(utc) {
			if v, ok := parseU8(utc[at : at+2]); ok {
				return v, nil
			}
		}
		return 0, unsupported("timestamp.utc: not an ISO 8601 time")
	}
	var v [5]uint8
	for i, at := range []int{5, 8, 11, 14, 17} {
		x, err := field(at)
		if err != nil {
			return nil, err
		}
		v[i] = x
	}
	month, day, hour, minute, second := v[0], v[1], v[2], v[3], v[4]
	format := "dhm"
	if f, ok := o["format"].(string); ok {
		format = f
	}
	var ts aprs.Timestamp
	switch format {
	case "dhm":
		ts = aprs.TimestampDHM(day, hour, minute)
	case "hms":
		ts = aprs.TimestampHMS(hour, minute, second)
	case "mdhm":
		ts = aprs.TimestampMDHM(month, day, hour, minute)
	default:
		return nil, unsupported("timestamp.format: the builder has no such format")
	}
	return &ts, nil
}

func weatherOf(station aprs.Station, a *recipeArgs) (*aprs.Packet, error) {
	b := station.Weather()
	lat, err := a.float("latitude")
	if err != nil {
		return nil, err
	}
	lon, err := a.float("longitude")
	if err != nil {
		return nil, err
	}
	switch {
	case lat != nil && lon != nil:
		b = b.At(*lat, *lon)
	case lat == nil && lon == nil:
	default:
		return nil, unsupported("latitude: the builder takes a latitude and a longitude together")
	}
	s, err := a.text("symbol")
	if err != nil {
		return nil, err
	}
	if sym, err := symbolOf(s); err != nil {
		return nil, err
	} else if sym != nil {
		b = b.Symbol(*sym)
	}
	if t, err := timestampOf(a); err != nil {
		return nil, err
	} else if t != nil {
		b = b.Timestamp(*t)
	}
	set := func(k string, f func(aprs.WeatherBuilder, float64) aprs.WeatherBuilder) error {
		v, err := a.float(k)
		if err != nil {
			return err
		}
		if v != nil {
			b = f(b, *v)
		}
		return nil
	}
	if err := set("wind_direction_degrees", aprs.WeatherBuilder.WindDirectionDegrees); err != nil {
		return nil, err
	}
	if err := set("wind_speed_mph", aprs.WeatherBuilder.WindSpeedMph); err != nil {
		return nil, err
	}
	if err := set("wind_gust_mph", aprs.WeatherBuilder.WindGustMph); err != nil {
		return nil, err
	}
	if a.has("temperature_f") && a.has("temperature_c") {
		return nil, unsupported("temperature_c: the builder takes one temperature")
	}
	if err := set("temperature_f", aprs.WeatherBuilder.TemperatureF); err != nil {
		return nil, err
	}
	if err := set("temperature_c", aprs.WeatherBuilder.TemperatureC); err != nil {
		return nil, err
	}
	rain := []struct {
		inches, mm  string
		setIn, setM func(aprs.WeatherBuilder, float64) aprs.WeatherBuilder
	}{
		{"rain_1h_in", "rain_1h_mm", aprs.WeatherBuilder.Rain1hIn, aprs.WeatherBuilder.Rain1hMm},
		{"rain_24h_in", "rain_24h_mm", aprs.WeatherBuilder.Rain24hIn, aprs.WeatherBuilder.Rain24hMm},
		{"rain_midnight_in", "rain_midnight_mm", aprs.WeatherBuilder.RainMidnightIn, aprs.WeatherBuilder.RainMidnightMm},
	}
	for _, r := range rain {
		if a.has(r.inches) && a.has(r.mm) {
			return nil, unsupported("%s: the builder takes one rainfall for each period", r.mm)
		}
		if err := set(r.inches, r.setIn); err != nil {
			return nil, err
		}
		if err := set(r.mm, r.setM); err != nil {
			return nil, err
		}
	}
	for _, x := range []struct {
		k string
		f func(aprs.WeatherBuilder, float64) aprs.WeatherBuilder
	}{
		{"humidity_percent", aprs.WeatherBuilder.HumidityPercent},
		{"pressure_mbar", aprs.WeatherBuilder.PressureMbar},
		{"luminosity_w_m2", aprs.WeatherBuilder.LuminosityWM2},
		{"snow_24h_in", aprs.WeatherBuilder.Snow24hIn},
	} {
		if err := set(x.k, x.f); err != nil {
			return nil, err
		}
	}
	if err := a.allTaken(); err != nil {
		return nil, err
	}
	return b.Build()
}
