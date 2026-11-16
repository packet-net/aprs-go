package aprs

import (
	"fmt"
	"strconv"

	"github.com/packet-net/aprs-go/internal/hook"
)

// The vectors' neutral data form, written from data: what the encoder's read-back compares (the
// data given with the data its own bytes decode to), and what the vectors runner and the dump
// tool write for a decode. Package internal/neutral reaches it through internal/hook, so that
// the form has one writer.

func init() {
	hook.Neutral = func(d any) map[string]any { return neutralData(d.(Data)) }
	hook.Names.Fix, hook.Names.Source, hook.Names.Origin = fixNames[:], sourceNames[:], originNames[:]
	hook.Names.Shape, hook.Names.Color, hook.Names.Storm = shapeNames[:], colorNames[:], stormNames[:]
	hook.Names.DAO, hook.Names.Tone = daoNames[:], toneNames[:]
}

func nfDiagnostics(ds []Diagnostic) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Severity.String() + ":" + d.Code.ID()
	}
	return out
}

func nfPath(entries []PathEntry) []any {
	out := make([]any, len(entries))
	for i, e := range entries {
		s := e.Address.String()
		if e.Used {
			s += "*"
		}
		out[i] = s
	}
	return out
}

func nfStrs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// nfNum is a float. A NaN or an infinity stays one: it is a value, never absent, so the read-back
// never compares it as nothing (it can only differ from what any bytes decode to).
func nfNum(v float64) any {
	return v
}

func nfOptNum(v *float64) any {
	if v == nil {
		return nil
	}
	return nfNum(*v)
}

func nfOptStr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nfOptInt[T uint8 | uint16 | uint32 | int32](v *T) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

// nfObj writes a key only when it has something to say: no nulls, empty strings or empty lists,
// and booleans only when true.
type nfObj map[string]any

func nfNewObj(kind string) nfObj {
	o := nfObj{}
	if kind != "" {
		o["type"] = kind
	}
	return o
}

func (o nfObj) put(k string, v any) nfObj {
	switch x := v.(type) {
	case nil:
		return o
	case string:
		if x == "" {
			return o
		}
	case []any:
		if len(x) == 0 {
			return o
		}
	case bool:
		if !x {
			return o
		}
	}
	o[k] = v
	return o
}

func nfTimestamp(t *Timestamp) any {
	if t == nil {
		return nil
	}
	return t.String()
}

func nfSymbol(s Symbol) string {
	return string(s.Table) + string(s.Code)
}

// neutralData gives data in the vectors' neutral data form.
func neutralData(d Data) map[string]any {
	return neutralDataAt(d, 0)
}

// maxNesting is how many third-party levels the neutral form is written for; a caller-built
// chain deeper than a decoder reads is cut there, so conversion never recurses far.
const maxNesting = 5

func neutralDataAt(d Data, depth int) map[string]any {
	switch x := derefData(d).(type) {
	case nil:
		return nfNewObj("unrecognized").put("reason", "none")
	case PositionReport:
		o := nfNewObj("position")
		o.put("timestamp", nfTimestamp(x.Timestamp)).put("messaging", x.Messaging)
		nfPositioned(o, &x.Fields)
		return o
	case MicEReport:
		o := nfNewObj("mic-e")
		o.put("mic_e_message", micEMessageName(x.MicEMessage)).put("old_data", x.OldData)
		if x.TypeCode != nil {
			o.put("type_code", string(*x.TypeCode))
		}
		o.put("device_suffix", x.DeviceSuffix).put("locator", nfOptStr(x.Locator))
		legacy := make([]any, len(x.LegacyTelemetry))
		for i, b := range x.LegacyTelemetry {
			legacy[i] = int64(b)
		}
		o.put("legacy_telemetry", legacy)
		if x.DestinationSSID != 0 {
			o.put("destination_ssid", int64(x.DestinationSSID))
		}
		nfPositioned(o, &x.Fields)
		return o
	case ObjectReport:
		o := nfNewObj("object")
		o.put("name", x.Name).put("killed", x.Killed).put("timestamp", nfTimestamp(x.Timestamp))
		nfPositioned(o, &x.Fields)
		return o
	case ItemReport:
		o := nfNewObj("item")
		o.put("name", x.Name).put("killed", x.Killed)
		nfPositioned(o, &x.Fields)
		return o
	case Message:
		o := nfNewObj("message").put("addressee", x.Addressee).put("text", x.Text).put("message_id", nfOptStr(x.MessageID))
		if x.ReplyAck != nil {
			o["reply_ack"] = *x.ReplyAck
		}
		return o
	case Ack:
		o := nfNewObj("ack").put("addressee", x.Addressee).put("acked_id", x.AckedID).put("message_id", nfOptStr(x.MessageID))
		if x.ReplyAck != nil {
			o["reply_ack"] = *x.ReplyAck
		}
		return o
	case Reject:
		o := nfNewObj("reject").put("addressee", x.Addressee).put("rejected_id", x.RejectedID).put("message_id", nfOptStr(x.MessageID))
		if x.ReplyAck != nil {
			o["reply_ack"] = *x.ReplyAck
		}
		return o
	case Bulletin:
		return nfNewObj("bulletin").put("addressee", x.Addressee).put("text", x.Text).put("message_id", nfOptStr(x.MessageID))
	case NWSBulletin:
		return nfNewObj("nws-bulletin").put("addressee", x.Addressee).put("text", x.Text).put("message_id", nfOptStr(x.MessageID))
	case TelemetryNames:
		return nfNewObj("telemetry-names").put("addressee", x.Addressee).put("names", nfStrs(x.Names)).put("message_id", nfOptStr(x.MessageID))
	case TelemetryUnits:
		return nfNewObj("telemetry-units").put("addressee", x.Addressee).put("units", nfStrs(x.Units)).put("message_id", nfOptStr(x.MessageID))
	case TelemetryCoefficients:
		c := make([]any, len(x.Coefficients))
		for i, v := range x.Coefficients {
			c[i] = nfNum(v)
		}
		return nfNewObj("telemetry-coefficients").put("addressee", x.Addressee).put("coefficients", c).put("message_id", nfOptStr(x.MessageID))
	case TelemetryBits:
		return nfNewObj("telemetry-bits").put("addressee", x.Addressee).put("bits", x.Bits).put("project", x.Project).put("message_id", nfOptStr(x.MessageID))
	case DirectedQuery:
		return nfNewObj("directed-query").put("addressee", x.Addressee).put("query_type", x.QueryType).put("target", nfOptStr(x.Target))
	case Status:
		o := nfNewObj("status").put("timestamp", nfTimestamp(x.Timestamp)).put("locator", nfOptStr(x.Locator))
		if x.Symbol != nil {
			o.put("symbol", nfSymbol(*x.Symbol))
		}
		if x.Beam != nil {
			o.put("beam", map[string]any{"heading_code": string(x.Beam.HeadingCode), "power_code": string(x.Beam.PowerCode)})
		}
		return o.put("text", x.Text)
	case Telemetry:
		analog := make([]any, len(x.Analog))
		for i, v := range x.Analog {
			analog[i] = nfOptNum(v)
		}
		return nfNewObj("telemetry").put("sequence", x.Sequence).put("analog", analog).put("bits", nfOptStr(x.Bits)).put("comment", x.Comment)
	case WeatherReport:
		return nfNewObj("weather").put("timestamp", nfTimestamp(x.Timestamp)).put("weather", nfWeather(&x.Weather)).put("comment", x.Comment)
	case RawWeather:
		return nfNewObj("raw-weather").put("format", rawWeatherFormatName(x.Format)).put("data", x.Data)
	case NMEA:
		var fix any
		if x.FixValid != nil {
			fix = "invalid"
			if *x.FixValid {
				fix = "valid"
			}
		}
		return nfNewObj("nmea").put("sentence", x.Sentence).put("has_checksum", x.HasChecksum).put("latitude", nfOptNum(x.Latitude)).
			put("longitude", nfOptNum(x.Longitude)).put("fix", fix).put("course_degrees", nfOptNum(x.CourseDegrees)).
			put("speed_knots", nfOptNum(x.SpeedKnots)).put("altitude_m", nfOptNum(x.AltitudeM)).put("time", nfOptStr(x.Time)).
			put("waypoint", nfOptStr(x.Waypoint)).put("comment", x.Comment)
	case MaidenheadBeacon:
		return nfNewObj("maidenhead-beacon").put("locator", x.Locator).put("comment", x.Comment)
	case Query:
		o := nfNewObj("query").put("query_type", x.QueryType)
		if f := x.Footprint; f != nil {
			o.put("footprint", map[string]any{"latitude": nfNum(f.Latitude), "longitude": nfNum(f.Longitude), "radius_miles": nfNum(f.RadiusMiles)})
		}
		return o
	case Capabilities:
		items := make([]any, len(x.Capabilities))
		for i, c := range x.Capabilities {
			if c.Value != nil {
				items[i] = []any{c.Token, *c.Value}
			} else {
				items[i] = []any{c.Token}
			}
		}
		return nfNewObj("capabilities").put("capabilities", items)
	case ThirdParty:
		if depth >= maxNesting {
			return nfNewObj("unrecognized").put("reason", "malformed")
		}
		p := x.Packet
		if p == nil {
			return nfNewObj("unrecognized").put("reason", "none")
		}
		// A header's source and destination are always written, even when empty.
		inner := nfObj{"source": p.Source.String(), "destination": p.Destination.String()}
		inner.put("path", nfPath(p.Path))
		inner.put("data", neutralDataAt(p.Data, depth+1)).put("diagnostics", nfStrs(nfDiagnostics(p.Diagnostics)))
		return nfNewObj("third-party").put("packet", map[string]any(inner))
	case UserDefined:
		data := make([]rune, len(x.Data))
		for i, b := range x.Data {
			data[i] = rune(b)
		}
		return nfNewObj("user-defined").put("user_id", string(x.UserID)).put("packet_type", string(x.PacketType)).put("data", string(data))
	case TestData:
		return nfNewObj("test").put("data", x.Data)
	case AgreloDF:
		return nfNewObj("agrelo-df").put("bearing_degrees", int64(x.BearingDegrees)).put("quality", int64(x.Quality))
	case Unrecognized:
		return nfNewObj("unrecognized").put("reason", unrecognizedName(x))
	}
	panic(fmt.Sprintf("a data type the neutral form does not know: %T", d))
}

// nameOf is a table's name for an enumeration value, total: a value outside its type, which only a
// program can build and the encoder refuses, has a name that no value of the type has, so nothing
// ever indexes past a table.
func nameOf(table []string, i int) string {
	if i < 0 || i >= len(table) {
		return "?" + strconv.Itoa(i)
	}
	return table[i]
}

// MicEMessageName gives a Mic-E message code's neutral name.
func micEMessageName(m MicEMessage) string {
	if n, ok := m.Custom(); ok {
		return fmt.Sprintf("custom%d", n)
	}
	switch m {
	case MicEOffDuty:
		return "off-duty"
	case MicEEnRoute:
		return "en-route"
	case MicEInService:
		return "in-service"
	case MicEReturning:
		return "returning"
	case MicECommitted:
		return "committed"
	case MicESpecial:
		return "special"
	case MicEPriority:
		return "priority"
	case MicEEmergency:
		return "emergency"
	}
	return "unknown"
}

// RawWeatherFormatName gives a raw weather format's neutral name.
func rawWeatherFormatName(f RawWeatherFormat) string {
	return nameOf([]string{"peet-bros-hash", "peet-bros-star", "ultimeter-packet", "ultimeter-logging"}, int(f))
}

// UnrecognizedName gives why nothing was decoded, as the neutral form names it.
func unrecognizedName(u Unrecognized) string {
	return nameOf([]string{"empty", "not-aprs", "reserved-data-type", "malformed"}, int(u))
}

var (
	fixNames    = [...]string{"old", "current"}
	sourceNames = [...]string{"other", "gll", "gga", "rmc"}
	originNames = [...]string{"compressed", "tnc-beacon-text", "software", "reserved3", "kpc3", "pico", "other-tracker", "digipeater-conversion"}
	shapeNames  = [...]string{"open-circle", "line-down-right", "open-ellipse", "open-triangle", "open-box", "filled-circle", "line-down-left", "filled-ellipse", "filled-triangle", "filled-box"}
	colorNames  = [...]string{"black", "blue", "green", "cyan", "red", "violet", "yellow", "gray", "black-low", "blue-low", "green-low", "cyan-low", "red-low", "violet-low", "yellow-low", "gray-low"}
	stormNames  = [...]string{"tropical-storm", "hurricane", "tropical-depression"}
	daoNames    = [...]string{"none", "thousandths", "base91"}
	toneNames   = [...]string{"off", "tone", "ctcss", "dcs", "tone-burst"}
)

func nfPositioned(o nfObj, f *Positioned) {
	o.put("latitude", nfNum(f.Position.Latitude)).put("longitude", nfNum(f.Position.Longitude))
	if f.Position.Ambiguity > 0 {
		o.put("ambiguity", int64(f.Position.Ambiguity))
	}
	o.put("symbol", nfSymbol(f.Symbol)).put("compressed", f.Compressed)
	if t := f.Compression; t != nil {
		o.put("compression", map[string]any{"fix": nameOf(fixNames[:], int(t.Fix)), "source": nameOf(sourceNames[:], int(t.Source)), "origin": nameOf(originNames[:], int(t.Origin))})
	}
	o.put("course_degrees", nfOptNum(f.CourseDegrees)).put("speed_knots", nfOptNum(f.SpeedKnots)).put("altitude_feet", nfOptNum(f.AltitudeFeet))
	if p := f.PHG; p != nil {
		po := nfObj{}
		po.put("power", int64(p.Power)).put("height", int64(p.Height)).put("gain", int64(p.Gain)).put("directivity", int64(p.Directivity))
		po.put("beacons_per_hour", nfOptInt(p.BeaconsPerHour))
		o.put("phg", map[string]any(po))
	}
	o.put("range_miles", nfOptNum(f.RangeMiles))
	if d := f.DFS; d != nil {
		o.put("dfs", map[string]any{"strength": int64(d.Strength), "height": int64(d.Height), "gain": int64(d.Gain), "directivity": int64(d.Directivity)})
	}
	if a := f.Area; a != nil {
		ao := nfObj{}
		ao.put("shape", nameOf(shapeNames[:], int(a.Shape))).put("lat_offset", int64(a.LatOffset)).put("color", nameOf(colorNames[:], int(a.Color)))
		ao.put("lon_offset", int64(a.LonOffset)).put("corridor_width_miles", nfOptInt(a.CorridorWidthMiles))
		o.put("area", map[string]any(ao))
	}
	if b := f.DFBearing; b != nil {
		o.put("df_bearing", map[string]any{"bearing_degrees": int64(b.BearingDegrees), "number": int64(b.Number), "range": int64(b.Range), "quality": int64(b.Quality)})
	}
	if s := f.Storm; s != nil {
		so := nfNewObj(nameOf(stormNames[:], int(s.Kind)))
		so.put("sustained_wind_knots", nfOptInt(s.SustainedWindKnots)).put("gust_knots", nfOptInt(s.GustKnots)).
			put("central_pressure_mbar", nfOptInt(s.CentralPressureMbar)).put("hurricane_radius_nm", nfOptInt(s.HurricaneRadiusNM)).
			put("tropical_storm_radius_nm", nfOptInt(s.TropicalStormRadiusNM)).put("whole_gale_radius_nm", nfOptInt(s.WholeGaleRadiusNM))
		o.put("storm", map[string]any(so))
	}
	if d := f.DAO; d != nil {
		o.put("dao", map[string]any{"datum": string(d.Datum), "precision": nameOf(daoNames[:], int(d.Precision))})
	}
	if t := f.Telemetry; t != nil {
		to := nfObj{}
		analog := make([]any, len(t.Analog))
		for i, v := range t.Analog {
			analog[i] = int64(v)
		}
		to.put("sequence", int64(t.Sequence)).put("analog", analog)
		if t.Digital != nil {
			to["digital"] = int64(*t.Digital)
		}
		o.put("telemetry", map[string]any(to))
	}
	if v := f.Frequency; v != nil {
		fo := nfObj{}
		fo.put("mhz", nfNum(v.MHz))
		if v.Tone != nil {
			fo.put("tone", nameOf(toneNames[:], int(*v.Tone)))
		}
		fo.put("tone_value", nfOptInt(v.ToneValue)).put("offset_khz", nfOptInt(v.OffsetKHz)).put("range", nfOptInt(v.Range))
		fo.put("range_km", v.RangeKm).put("narrow", v.Narrow).put("ten_khz_resolution", v.TenKHzResolution)
		o.put("frequency", map[string]any(fo))
	}
	if f.Weather != nil {
		// A weather object is written even when empty: it says the report is a weather report.
		o["weather"] = nfWeather(f.Weather)
	}
	o.put("signpost", nfOptStr(f.Signpost)).put("comment", f.Comment)
}

func nfWeather(w *Weather) map[string]any {
	o := nfObj{}
	o.put("wind_direction_degrees", nfOptNum(w.WindDirectionDegrees)).
		put("wind_speed_mph", nfOptNum(w.WindSpeedMph)).
		put("wind_gust_mph", nfOptNum(w.WindGustMph)).
		put("temperature_f", nfOptNum(w.TemperatureF)).
		put("rain_1h_in", nfOptNum(w.Rain1hIn)).
		put("rain_24h_in", nfOptNum(w.Rain24hIn)).
		put("rain_midnight_in", nfOptNum(w.RainMidnightIn)).
		put("rain_raw", nfOptInt(w.RainRaw)).
		put("humidity_percent", nfOptNum(w.HumidityPercent)).
		put("pressure_mbar", nfOptNum(w.PressureMbar)).
		put("luminosity_w_m2", nfOptNum(w.LuminosityWM2)).
		put("snow_24h_in", nfOptNum(w.Snow24hIn))
	if w.Software != nil {
		o.put("software", string(*w.Software))
	}
	o.put("unit", nfOptStr(w.Unit))
	extra := make([]any, len(w.Extra))
	for i, e := range w.Extra {
		extra[i] = map[string]any{"letter": string(e.Letter), "value": e.Value}
	}
	o.put("extra", extra)
	return o
}
