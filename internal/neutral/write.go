// Package neutral is the conformance vectors' neutral data form: writing decoded data (for
// comparison) and reading it back (to build data for the encode cases). The rules are in the
// vectors' README. It is not public API: the vectors runner and the diff-dump tool share it.
package neutral

import (
	"fmt"
	"math"

	aprs "github.com/packet-net/aprs-go"
)

// A neutral value is one of: nil, bool, string, int64 (a whole number as JSON writes it),
// float64, []any or map[string]any.

// Diagnostic gives a diagnostic as severity:code.
func Diagnostic(d aprs.Diagnostic) string {
	return d.Severity.String() + ":" + d.Code.ID()
}

// Diagnostics gives each diagnostic as severity:code.
func Diagnostics(ds []aprs.Diagnostic) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = Diagnostic(d)
	}
	return out
}

// Header gives a packet's header: source, destination, path and q-construct.
func Header(p *aprs.Packet) map[string]any {
	o := map[string]any{"source": p.Source.String(), "destination": p.Destination.String()}
	if len(p.Path) > 0 {
		o["path"] = path(p.Path)
	}
	if q, ok := p.QConstruct(); ok {
		qo := map[string]any{"construct": q.Construct}
		if q.Station != nil {
			qo["station"] = q.Station.String()
		}
		o["q_construct"] = qo
	}
	return o
}

func path(entries []aprs.PathEntry) []any {
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

func strs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// num is a float as JSON can hold it: a NaN or an infinity is null.
func num(v float64) any {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}

func optNum(v *float64) any {
	if v == nil {
		return nil
	}
	return num(*v)
}

func optStr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func optInt[T uint8 | uint16 | uint32 | int32](v *T) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

// obj writes a key only when it has something to say: no nulls, empty strings or empty lists,
// and booleans only when true.
type obj map[string]any

func newObj(kind string) obj {
	o := obj{}
	if kind != "" {
		o["type"] = kind
	}
	return o
}

func (o obj) put(k string, v any) obj {
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

func timestamp(t *aprs.Timestamp) any {
	if t == nil {
		return nil
	}
	return t.String()
}

func symbol(s aprs.Symbol) string {
	return string(s.Table) + string(s.Code)
}

// Data gives data in the neutral form.
func Data(d aprs.Data) map[string]any {
	switch x := d.(type) {
	case aprs.PositionReport:
		o := newObj("position")
		o.put("timestamp", timestamp(x.Timestamp)).put("messaging", x.Messaging)
		positioned(o, &x.Fields)
		return o
	case aprs.MicEReport:
		o := newObj("mic-e")
		o.put("mic_e_message", MicEMessageName(x.MicEMessage)).put("old_data", x.OldData)
		if x.TypeCode != nil {
			o.put("type_code", string(*x.TypeCode))
		}
		o.put("device_suffix", x.DeviceSuffix).put("locator", optStr(x.Locator))
		legacy := make([]any, len(x.LegacyTelemetry))
		for i, b := range x.LegacyTelemetry {
			legacy[i] = int64(b)
		}
		o.put("legacy_telemetry", legacy)
		if x.DestinationSSID != 0 {
			o.put("destination_ssid", int64(x.DestinationSSID))
		}
		positioned(o, &x.Fields)
		return o
	case aprs.ObjectReport:
		o := newObj("object")
		o.put("name", x.Name).put("killed", x.Killed).put("timestamp", timestamp(x.Timestamp))
		positioned(o, &x.Fields)
		return o
	case aprs.ItemReport:
		o := newObj("item")
		o.put("name", x.Name).put("killed", x.Killed)
		positioned(o, &x.Fields)
		return o
	case aprs.Message:
		o := newObj("message").put("addressee", x.Addressee).put("text", x.Text).put("message_id", optStr(x.MessageID))
		if x.ReplyAck != nil {
			o["reply_ack"] = *x.ReplyAck
		}
		return o
	case aprs.Ack:
		o := newObj("ack").put("addressee", x.Addressee).put("acked_id", x.AckedID).put("message_id", optStr(x.MessageID))
		if x.ReplyAck != nil {
			o["reply_ack"] = *x.ReplyAck
		}
		return o
	case aprs.Reject:
		o := newObj("reject").put("addressee", x.Addressee).put("rejected_id", x.RejectedID).put("message_id", optStr(x.MessageID))
		if x.ReplyAck != nil {
			o["reply_ack"] = *x.ReplyAck
		}
		return o
	case aprs.Bulletin:
		return newObj("bulletin").put("addressee", x.Addressee).put("text", x.Text).put("message_id", optStr(x.MessageID))
	case aprs.NWSBulletin:
		return newObj("nws-bulletin").put("addressee", x.Addressee).put("text", x.Text).put("message_id", optStr(x.MessageID))
	case aprs.TelemetryNames:
		return newObj("telemetry-names").put("addressee", x.Addressee).put("names", strs(x.Names)).put("message_id", optStr(x.MessageID))
	case aprs.TelemetryUnits:
		return newObj("telemetry-units").put("addressee", x.Addressee).put("units", strs(x.Units)).put("message_id", optStr(x.MessageID))
	case aprs.TelemetryCoefficients:
		c := make([]any, len(x.Coefficients))
		for i, v := range x.Coefficients {
			c[i] = num(v)
		}
		return newObj("telemetry-coefficients").put("addressee", x.Addressee).put("coefficients", c).put("message_id", optStr(x.MessageID))
	case aprs.TelemetryBits:
		return newObj("telemetry-bits").put("addressee", x.Addressee).put("bits", x.Bits).put("project", x.Project).put("message_id", optStr(x.MessageID))
	case aprs.DirectedQuery:
		return newObj("directed-query").put("addressee", x.Addressee).put("query_type", x.QueryType).put("target", optStr(x.Target))
	case aprs.Status:
		o := newObj("status").put("timestamp", timestamp(x.Timestamp)).put("locator", optStr(x.Locator))
		if x.Symbol != nil {
			o.put("symbol", symbol(*x.Symbol))
		}
		if x.Beam != nil {
			o.put("beam", map[string]any{"heading_code": string(x.Beam.HeadingCode), "power_code": string(x.Beam.PowerCode)})
		}
		return o.put("text", x.Text)
	case aprs.Telemetry:
		analog := make([]any, len(x.Analog))
		for i, v := range x.Analog {
			analog[i] = optNum(v)
		}
		return newObj("telemetry").put("sequence", x.Sequence).put("analog", analog).put("bits", optStr(x.Bits)).put("comment", x.Comment)
	case aprs.WeatherReport:
		return newObj("weather").put("timestamp", timestamp(x.Timestamp)).put("weather", weather(&x.Weather)).put("comment", x.Comment)
	case aprs.RawWeather:
		return newObj("raw-weather").put("format", RawWeatherFormatName(x.Format)).put("data", x.Data)
	case aprs.NMEA:
		var fix any
		if x.FixValid != nil {
			fix = "invalid"
			if *x.FixValid {
				fix = "valid"
			}
		}
		return newObj("nmea").put("sentence", x.Sentence).put("has_checksum", x.HasChecksum).put("latitude", optNum(x.Latitude)).
			put("longitude", optNum(x.Longitude)).put("fix", fix).put("course_degrees", optNum(x.CourseDegrees)).
			put("speed_knots", optNum(x.SpeedKnots)).put("altitude_m", optNum(x.AltitudeM)).put("time", optStr(x.Time)).
			put("waypoint", optStr(x.Waypoint)).put("comment", x.Comment)
	case aprs.MaidenheadBeacon:
		return newObj("maidenhead-beacon").put("locator", x.Locator).put("comment", x.Comment)
	case aprs.Query:
		o := newObj("query").put("query_type", x.QueryType)
		if f := x.Footprint; f != nil {
			o.put("footprint", map[string]any{"latitude": num(f.Latitude), "longitude": num(f.Longitude), "radius_miles": num(f.RadiusMiles)})
		}
		return o
	case aprs.Capabilities:
		items := make([]any, len(x.Capabilities))
		for i, c := range x.Capabilities {
			if c.Value != nil {
				items[i] = []any{c.Token, *c.Value}
			} else {
				items[i] = []any{c.Token}
			}
		}
		return newObj("capabilities").put("capabilities", items)
	case aprs.ThirdParty:
		p := x.Packet
		// A header's source and destination are always written, even when empty.
		inner := obj{"source": p.Source.String(), "destination": p.Destination.String()}
		inner.put("path", path(p.Path))
		inner.put("data", Data(p.Data)).put("diagnostics", strs(Diagnostics(p.Diagnostics)))
		return newObj("third-party").put("packet", map[string]any(inner))
	case aprs.UserDefined:
		data := make([]rune, len(x.Data))
		for i, b := range x.Data {
			data[i] = rune(b)
		}
		return newObj("user-defined").put("user_id", string(x.UserID)).put("packet_type", string(x.PacketType)).put("data", string(data))
	case aprs.TestData:
		return newObj("test").put("data", x.Data)
	case aprs.AgreloDF:
		return newObj("agrelo-df").put("bearing_degrees", int64(x.BearingDegrees)).put("quality", int64(x.Quality))
	case aprs.Unrecognized:
		return newObj("unrecognized").put("reason", UnrecognizedName(x))
	}
	panic(fmt.Sprintf("a data type the neutral form does not know: %T", d))
}

// MicEMessageName gives a Mic-E message code's neutral name.
func MicEMessageName(m aprs.MicEMessage) string {
	if n, ok := m.Custom(); ok {
		return fmt.Sprintf("custom%d", n)
	}
	switch m {
	case aprs.MicEOffDuty:
		return "off-duty"
	case aprs.MicEEnRoute:
		return "en-route"
	case aprs.MicEInService:
		return "in-service"
	case aprs.MicEReturning:
		return "returning"
	case aprs.MicECommitted:
		return "committed"
	case aprs.MicESpecial:
		return "special"
	case aprs.MicEPriority:
		return "priority"
	case aprs.MicEEmergency:
		return "emergency"
	}
	return "unknown"
}

// RawWeatherFormatName gives a raw weather format's neutral name.
func RawWeatherFormatName(f aprs.RawWeatherFormat) string {
	return [...]string{"peet-bros-hash", "peet-bros-star", "ultimeter-packet", "ultimeter-logging"}[f]
}

// UnrecognizedName gives why nothing was decoded, as the neutral form names it.
func UnrecognizedName(u aprs.Unrecognized) string {
	return [...]string{"empty", "not-aprs", "reserved-data-type", "malformed"}[u]
}

var (
	fixNames     = [...]string{"old", "current"}
	sourceNames  = [...]string{"other", "gll", "gga", "rmc"}
	originNames  = [...]string{"compressed", "tnc-beacon-text", "software", "reserved3", "kpc3", "pico", "other-tracker", "digipeater-conversion"}
	shapeNames   = [...]string{"open-circle", "line-down-right", "open-ellipse", "open-triangle", "open-box", "filled-circle", "line-down-left", "filled-ellipse", "filled-triangle", "filled-box"}
	colorNames   = [...]string{"black", "blue", "green", "cyan", "red", "violet", "yellow", "gray", "black-low", "blue-low", "green-low", "cyan-low", "red-low", "violet-low", "yellow-low", "gray-low"}
	stormNames   = [...]string{"tropical-storm", "hurricane", "tropical-depression"}
	daoNames     = [...]string{"none", "thousandths", "base91"}
	toneNames    = [...]string{"off", "tone", "ctcss", "dcs", "tone-burst"}
)

func positioned(o obj, f *aprs.Positioned) {
	o.put("latitude", num(f.Position.Latitude)).put("longitude", num(f.Position.Longitude))
	if f.Position.Ambiguity > 0 {
		o.put("ambiguity", int64(f.Position.Ambiguity))
	}
	o.put("symbol", symbol(f.Symbol)).put("compressed", f.Compressed)
	if t := f.Compression; t != nil {
		o.put("compression", map[string]any{"fix": fixNames[t.Fix], "source": sourceNames[t.Source], "origin": originNames[t.Origin]})
	}
	o.put("course_degrees", optNum(f.CourseDegrees)).put("speed_knots", optNum(f.SpeedKnots)).put("altitude_feet", optNum(f.AltitudeFeet))
	if p := f.PHG; p != nil {
		po := obj{}
		po.put("power", int64(p.Power)).put("height", int64(p.Height)).put("gain", int64(p.Gain)).put("directivity", int64(p.Directivity))
		po.put("beacons_per_hour", optInt(p.BeaconsPerHour))
		o.put("phg", map[string]any(po))
	}
	o.put("range_miles", optNum(f.RangeMiles))
	if d := f.DFS; d != nil {
		o.put("dfs", map[string]any{"strength": int64(d.Strength), "height": int64(d.Height), "gain": int64(d.Gain), "directivity": int64(d.Directivity)})
	}
	if a := f.Area; a != nil {
		ao := obj{}
		ao.put("shape", shapeNames[a.Shape]).put("lat_offset", int64(a.LatOffset)).put("color", colorNames[a.Color])
		ao.put("lon_offset", int64(a.LonOffset)).put("corridor_width_miles", optInt(a.CorridorWidthMiles))
		o.put("area", map[string]any(ao))
	}
	if b := f.DFBearing; b != nil {
		o.put("df_bearing", map[string]any{"bearing_degrees": int64(b.BearingDegrees), "number": int64(b.Number), "range": int64(b.Range), "quality": int64(b.Quality)})
	}
	if s := f.Storm; s != nil {
		so := newObj(stormNames[s.Kind])
		so.put("sustained_wind_knots", optInt(s.SustainedWindKnots)).put("gust_knots", optInt(s.GustKnots)).
			put("central_pressure_mbar", optInt(s.CentralPressureMbar)).put("hurricane_radius_nm", optInt(s.HurricaneRadiusNM)).
			put("tropical_storm_radius_nm", optInt(s.TropicalStormRadiusNM)).put("whole_gale_radius_nm", optInt(s.WholeGaleRadiusNM))
		o.put("storm", map[string]any(so))
	}
	if d := f.DAO; d != nil {
		o.put("dao", map[string]any{"datum": string(d.Datum), "precision": daoNames[d.Precision]})
	}
	if t := f.Telemetry; t != nil {
		to := obj{}
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
		fo := obj{}
		fo.put("mhz", num(v.MHz))
		if v.Tone != nil {
			fo.put("tone", toneNames[*v.Tone])
		}
		fo.put("tone_value", optInt(v.ToneValue)).put("offset_khz", optInt(v.OffsetKHz)).put("range", optInt(v.Range))
		fo.put("range_km", v.RangeKm).put("narrow", v.Narrow).put("ten_khz_resolution", v.TenKHzResolution)
		o.put("frequency", map[string]any(fo))
	}
	if f.Weather != nil {
		// A weather object is written even when empty: it says the report is a weather report.
		o["weather"] = weather(f.Weather)
	}
	o.put("signpost", optStr(f.Signpost)).put("comment", f.Comment)
}

func weather(w *aprs.Weather) map[string]any {
	o := obj{}
	o.put("wind_direction_degrees", optNum(w.WindDirectionDegrees)).
		put("wind_speed_mph", optNum(w.WindSpeedMph)).
		put("wind_gust_mph", optNum(w.WindGustMph)).
		put("temperature_f", optNum(w.TemperatureF)).
		put("rain_1h_in", optNum(w.Rain1hIn)).
		put("rain_24h_in", optNum(w.Rain24hIn)).
		put("rain_midnight_in", optNum(w.RainMidnightIn)).
		put("rain_raw", optInt(w.RainRaw)).
		put("humidity_percent", optNum(w.HumidityPercent)).
		put("pressure_mbar", optNum(w.PressureMbar)).
		put("luminosity_w_m2", optNum(w.LuminosityWM2)).
		put("snow_24h_in", optNum(w.Snow24hIn))
	if w.Software != nil {
		o.put("software", string(*w.Software))
	}
	o.put("unit", optStr(w.Unit))
	extra := make([]any, len(w.Extra))
	for i, e := range w.Extra {
		extra[i] = map[string]any{"letter": string(e.Letter), "value": e.Value}
	}
	o.put("extra", extra)
	return o
}
