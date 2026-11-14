package aprs

import (
	"math"
	"strings"
)

// DefaultDestination is the destination address a Station uses until To sets one: APZ001, in the
// range the APRS device identification database keeps for experimental software. An application
// should use its own allocated destination (its "tocall").
const DefaultDestination = "APZ001"

// Station is a sending station and its header: the source, destination and path every packet
// built from it shares, and the starting point for each kind of packet.
//
// A station is a value: To and Via return a new one, so it can be kept and reused. Build encodes
// the packet, so anything APRS12c does not allow is refused there with an *EncodeError, as
// NewPacket would. Data gives the Data alone, for anything the builder does not cover; Station.Data
// sends it.
//
// A setter has the name of the field it fills, so its unit is visible at the call site
// (AltitudeFeet, SpeedKmh), and a setter that converts changes only the unit (AltitudeM,
// TemperatureC, Rain1hMm). A value between the steps a report holds is not rounded by the
// builder: the encoder rounds it, once, when the packet is built.
type Station struct {
	source      Address
	destination Address
	path        []PathEntry
}

// NewStation starts from the station sending the packets, such as M0LTE-9. It fails, with an
// *InvalidAddressError, if source is not an address.
func NewStation(source string) (Station, error) {
	s, err := NewAddress(source)
	if err != nil {
		return Station{}, err
	}
	return Station{source: s, destination: Address{DefaultDestination}}, nil
}

// To gives this station with another destination address, usually the application's allocated
// tocall. On an error it gives the station unchanged.
func (s Station) To(destination string) (Station, error) {
	d, err := NewAddress(destination)
	if err != nil {
		return s, err
	}
	s.destination = d
	return s, nil
}

// Via gives this station with a digipeater path, such as Via("WIDE1-1", "WIDE2-1"), replacing
// any set before. On an error it gives the station unchanged.
func (s Station) Via(path ...string) (Station, error) {
	entries := make([]PathEntry, 0, len(path))
	for _, p := range path {
		a, err := NewAddress(p)
		if err != nil {
			return s, err
		}
		entries = append(entries, PathEntry{Address: a})
	}
	s.path = entries
	return s, nil
}

// Source gives the source address.
func (s Station) Source() Address { return s.source }

// Destination gives the destination address. A Mic-E report computes its own and ignores this.
func (s Station) Destination() Address { return s.destination }

// Path gives the digipeater path.
func (s Station) Path() []PathEntry { return append([]PathEntry(nil), s.path...) }

// Position starts a position report for this station (APRS12c ch. 8 and 9).
func (s Station) Position(latitude, longitude float64) PositionBuilder {
	return PositionBuilder{station: s, common: commonAt(latitude, longitude)}
}

// Object starts an object: something other than this station placed on the map under its own
// name, which needs At and a Timestamp (APRS12c ch. 11).
func (s Station) Object(name string) ObjectBuilder {
	return ObjectBuilder{station: s, name: name}
}

// Item starts an item: like an object but with no timestamp, for things that do not move, which
// needs At (APRS12c ch. 11).
func (s Station) Item(name string) ItemBuilder {
	return ItemBuilder{station: s, name: name}
}

// MicE starts a Mic-E position report, which carries part of its position in the destination
// address, so ignores this station's destination (APRS12c ch. 10).
func (s Station) MicE(latitude, longitude float64) MicEBuilder {
	return MicEBuilder{station: s, common: commonAt(latitude, longitude), micEMessage: MicEOffDuty}
}

// Weather starts a weather report. With At it is a position report with the weather station
// symbol, as APRS12c recommends; without, a positionless weather report (APRS12c ch. 12).
func (s Station) Weather() WeatherBuilder {
	return WeatherBuilder{station: s, symbol: SymbolWeatherStation}
}

// Message starts a message to another station (APRS12c ch. 14).
func (s Station) Message(addressee, text string) MessageBuilder {
	return MessageBuilder{station: s, message: Message{Addressee: addressee, Text: text}}
}

// Ack starts an acknowledgement of message messageID from addressee (APRS12c ch. 14).
func (s Station) Ack(addressee, messageID string) AckBuilder {
	return AckBuilder{station: s, addressee: addressee, id: messageID}
}

// Reject starts a rejection of message messageID from addressee (APRS12c ch. 14).
func (s Station) Reject(addressee, messageID string) AckBuilder {
	return AckBuilder{station: s, addressee: addressee, id: messageID, reject: true}
}

// Bulletin starts a general bulletin (id '0'-'9') or an announcement ('A'-'Z') (APRS12c ch. 14).
func (s Station) Bulletin(id rune, text string) DataBuilder {
	return s.Data(Bulletin{Addressee: "BLN" + string(id), Text: text})
}

// GroupBulletin starts a group bulletin, such as BLN4WX for id '4' and group "WX" (APRS12c ch.
// 14).
func (s Station) GroupBulletin(id rune, group, text string) DataBuilder {
	return s.Data(Bulletin{Addressee: "BLN" + string(id) + group, Text: text})
}

// Status starts a status report (APRS12c ch. 16).
func (s Station) Status(text string) StatusBuilder {
	return StatusBuilder{station: s, status: Status{Text: text}}
}

// Telemetry starts a telemetry report with this sequence number, sent as three digits; a
// sequence over 999 is refused (APRS12c ch. 13).
func (s Station) Telemetry(sequence uint16) TelemetryBuilder {
	return TelemetryBuilder{station: s, sequence: sequence, bits: "00000000"}
}

// TelemetryNames starts the names of telemetry channels, A1-A5 then B1-B8 (PARM., APRS12c ch.
// 13), by default this station's own.
func (s Station) TelemetryNames(names ...string) TelemetryMetadataBuilder {
	return TelemetryMetadataBuilder{station: s, data: TelemetryNames{Addressee: s.source.text, Names: append([]string(nil), names...)}}
}

// TelemetryUnits starts the units of analog telemetry channels, then the labels of the digital
// ones (UNIT., APRS12c ch. 13), by default this station's own.
func (s Station) TelemetryUnits(units ...string) TelemetryMetadataBuilder {
	return TelemetryMetadataBuilder{station: s, data: TelemetryUnits{Addressee: s.source.text, Units: append([]string(nil), units...)}}
}

// TelemetryCoefficients starts the scaling for analog telemetry channels: a, b and c for each in
// turn, giving a x v^2 + b x v + c (EQNS., APRS12c ch. 13), by default this station's own.
func (s Station) TelemetryCoefficients(coefficients ...float64) TelemetryMetadataBuilder {
	return TelemetryMetadataBuilder{station: s, data: TelemetryCoefficients{Addressee: s.source.text, Coefficients: append([]float64(nil), coefficients...)}}
}

// TelemetryBits starts which state of each digital channel matches its label (eight 0 or 1
// characters, B1 first), and the project title (BITS., APRS12c ch. 13), by default this station's
// own.
func (s Station) TelemetryBits(bits, project string) TelemetryMetadataBuilder {
	return TelemetryMetadataBuilder{station: s, data: TelemetryBits{Addressee: s.source.text, Bits: bits, Project: project}}
}

// Data sends any data built by hand from this station.
func (s Station) Data(data Data) DataBuilder {
	return DataBuilder{station: s, data: data}
}

func (s Station) packet(data Data) (*Packet, error) {
	return NewPacket(s.source, s.destination, s.path, data)
}

// DataBuilder sends a Data built by hand, from Station.Data and the other one-step packets.
type DataBuilder struct {
	station Station
	data    Data
}

// Data gives the data, without a header.
func (b DataBuilder) Data() (Data, error) { return b.data, nil }

// Build gives the packet: the station's header and this data, encoded.
func (b DataBuilder) Build() (*Packet, error) { return b.station.packet(b.data) }

// TelemetryMetadataBuilder is telemetry metadata (PARM., UNIT., EQNS. or BITS.), from
// Station.TelemetryNames and the methods after it. It describes the sending station's own
// telemetry unless Addressee says another, as a gateway does for a sensor station.
type TelemetryMetadataBuilder struct {
	station Station
	data    Data
}

// Addressee sets the station the metadata describes, instead of the sending station itself.
func (b TelemetryMetadataBuilder) Addressee(addressee string) TelemetryMetadataBuilder {
	switch d := b.data.(type) {
	case TelemetryNames:
		d.Addressee = addressee
		b.data = d
	case TelemetryUnits:
		d.Addressee = addressee
		b.data = d
	case TelemetryCoefficients:
		d.Addressee = addressee
		b.data = d
	case TelemetryBits:
		d.Addressee = addressee
		b.data = d
	}
	return b
}

// Data gives the data, without a header.
func (b TelemetryMetadataBuilder) Data() (Data, error) { return b.data, nil }

// Build gives the packet: the station's header and this metadata, encoded.
func (b TelemetryMetadataBuilder) Build() (*Packet, error) { return b.station.packet(b.data) }

type toneSetting struct {
	tone  Tone
	value uint16
}

// common is what every report that puts something on the map carries besides its own fields.
type common struct {
	position  *Position
	ambiguity uint8
	symbol    *Symbol
	fields    Positioned
	tone      *toneSetting
	offsetKHz *int32
	// altitudeM is an altitude given in metres, which a Mic-E report carries in metres and any
	// other in feet.
	altitudeM *float64
}

func commonAt(latitude, longitude float64) common {
	return common{position: &Position{Latitude: latitude, Longitude: longitude}}
}

// positioned gives the shared fields, checked for what the builder itself needs; the encoder
// checks the rest.
func (c *common) positioned(what string, micE bool) (Positioned, error) {
	if c.position == nil {
		return Positioned{}, newEncodeError(RefusalMissingField, what+" needs a position: call At(latitude, longitude)")
	}
	// A symbol not given stays the zero Symbol, which the encoder refuses as invalid-symbol at its
	// place in the order of refusals.
	position := *c.position
	fields := c.fields
	// An ambiguous position reports the box that holds it: its digits cut, not rounded, then
	// blanked, and the box's centre is what a decoder reads (vectors README, Builder rules). A
	// compressed report refuses ambiguity whatever the box, so it keeps the position as given.
	if c.ambiguity >= 1 && c.ambiguity <= 4 && !fields.Compressed {
		lat, lon := boxCentre(position.Latitude, c.ambiguity), boxCentre(position.Longitude, c.ambiguity)
		// A box centred past 90 or 180 degrees has no form: the position is kept as given, and
		// the encoder names that in its own order, after anything out of range.
		if (math.Abs(position.Latitude) <= 90 && math.Abs(lat) > 90) || (math.Abs(position.Longitude) <= 180 && math.Abs(lon) > 180) {
			lat, lon = position.Latitude, position.Longitude
		}
		fields.Position = Position{Latitude: lat, Longitude: lon, Ambiguity: c.ambiguity}
	} else {
		fields.Position = Position{Latitude: position.Latitude, Longitude: position.Longitude, Ambiguity: c.ambiguity}
	}
	if c.symbol != nil {
		fields.Symbol = *c.symbol
	}
	if c.altitudeM != nil {
		// Mic-E carries its altitude in whole metres (APRS12c ch. 10); a /A= is in feet.
		metres := *c.altitudeM
		if micE {
			fields.AltitudeFeet = Ptr(roundSteps(metres) * metresToFeet)
		} else {
			fields.AltitudeFeet = Ptr(metres * metresToFeet)
		}
	}
	if micE && fields.AltitudeFeet != nil {
		// A Mic-E altitude goes in the Mic-E altitude bytes, rounded once to the nearest metre
		// from the value given, whatever its unit: 1000 feet is 305 metres. One outside what
		// those bytes hold is refused, not moved to a /A= (vectors README, Builder rules); that
		// is MicEBuilder.Data's to name, at its place in the order of refusals, so it is kept as
		// given here.
		feet := *fields.AltitudeFeet
		if c.altitudeM == nil {
			if metres := roundSteps(feet / metresToFeet); inRange(metres, -10000, 743570) {
				fields.AltitudeFeet = Ptr(metres * metresToFeet)
			}
		}
	}
	// A course that rounds to 0 is the spec's unknown course, which Mic-E sends as 0 (APRS12c ch. 10).
	if micE && fields.CourseDegrees != nil && roundSteps(*fields.CourseDegrees) == 0 {
		fields.CourseDegrees = nil
	}
	if c.tone != nil || c.offsetKHz != nil {
		if fields.Frequency == nil {
			return Positioned{}, newEncodeError(RefusalMissingField, "a tone or offset needs a frequency: call Frequency(mhz) first")
		}
		frequency := *fields.Frequency
		if c.tone != nil {
			frequency.Tone = Ptr(c.tone.tone)
			// Toff and the 1750 Hz burst have no value of their own.
			frequency.ToneValue = nil
			if c.tone.tone == ToneTone || c.tone.tone == ToneCTCSS || c.tone.tone == ToneDCS {
				frequency.ToneValue = Ptr(c.tone.value)
			}
		}
		if c.offsetKHz != nil {
			frequency.OffsetKHz = Ptr(*c.offsetKHz)
		}
		fields.Frequency = &frequency
	}
	return fields, nil
}

// boxCentre gives the centre of the box that holds degrees once ambiguity (1-4) digits are
// blanked: the digits are cut to the box, not rounded, so the box is the one the position is in.
func boxCentre(degrees float64, ambiguity uint8) float64 {
	// The box size in hundredths of a minute: a hundredth's digit, a tenth's, a minute's, ten
	// minutes', or a degree.
	size := [...]float64{1, 10, 100, 1000, 6000}[ambiguity]
	// A value within 1e-6 of a box's edge, measured in boxes, is on the edge: 80.49 degrees is 80
	// degrees 29.4 minutes, though in binary it computes as 29.39999...
	boxes := math.Abs(degrees) * 6000 / size
	centre := (math.Floor(boxes+1e-6)*size + size/2) / 6000
	if math.Signbit(degrees) {
		return -centre
	}
	return centre
}

// PositionBuilder builds a position report, from Station.Position (APRS12c ch. 8 and 9).
type PositionBuilder struct {
	station   Station
	common    common
	messaging bool
	timestamp *Timestamp
}

// Messaging says the station can receive APRS messages (= or @ rather than ! or /).
func (b PositionBuilder) Messaging() PositionBuilder {
	b.messaging = true
	return b
}

// Timestamp sets when the position was valid, for a report that is not current (APRS12c ch. 6).
func (b PositionBuilder) Timestamp(t Timestamp) PositionBuilder {
	b.timestamp = &t
	return b
}

// Data gives the data, without a header. Data the builder was not given everything for (a
// symbol, or an object's or positionless weather report's timestamp) is refused as Build refuses it.
func (b PositionBuilder) Data() (Data, error) {
	d, err := b.data()
	if err != nil {
		return nil, err
	}
	return refuseMissing(d, b.common.symbol == nil)
}

func (b PositionBuilder) data() (Data, error) {
	fields, err := b.common.positioned("a position report", false)
	if err != nil {
		return nil, err
	}
	return PositionReport{Timestamp: b.timestamp, Messaging: b.messaging, Fields: fields}, nil
}

// Build gives the packet: the station's header and this report, encoded.
func (b PositionBuilder) Build() (*Packet, error) { return build(b.station, b.data) }

// ObjectBuilder builds an object, from Station.Object (APRS12c ch. 11).
type ObjectBuilder struct {
	station   Station
	common    common
	name      string
	killed    bool
	timestamp *Timestamp
}

// Timestamp sets when the object report was made; an object must have one. Usually the time
// now, TimestampDHM(day, hour, minute) in UTC.
func (b ObjectBuilder) Timestamp(t Timestamp) ObjectBuilder {
	b.timestamp = &t
	return b
}

// Permanent marks the object permanent, one only this station may change, with the 111111z
// timestamp (APRS12c ch. 18).
func (b ObjectBuilder) Permanent() ObjectBuilder {
	return b.Timestamp(TimestampDHM(11, 11, 11))
}

// Killed kills the object, removing it from maps (_ rather than *).
func (b ObjectBuilder) Killed() ObjectBuilder {
	b.killed = true
	return b
}

// Data gives the data, without a header. Data the builder was not given everything for (a
// symbol, or an object's or positionless weather report's timestamp) is refused as Build refuses it.
func (b ObjectBuilder) Data() (Data, error) {
	d, err := b.data()
	if err != nil {
		return nil, err
	}
	return refuseMissing(d, b.common.symbol == nil || b.timestamp == nil)
}

func (b ObjectBuilder) data() (Data, error) {
	fields, err := b.common.positioned("an object", false)
	if err != nil {
		return nil, err
	}
	// A timestamp not given is left nil, which the encoder refuses as missing-field at its place.
	var t *Timestamp
	if b.timestamp != nil {
		c := *b.timestamp
		t = &c
	}
	return ObjectReport{Name: b.name, Killed: b.killed, Timestamp: t, Fields: fields}, nil
}

// Build gives the packet: the station's header and this object, encoded.
func (b ObjectBuilder) Build() (*Packet, error) { return build(b.station, b.data) }

// ItemBuilder builds an item, from Station.Item (APRS12c ch. 11).
type ItemBuilder struct {
	station Station
	common  common
	name    string
	killed  bool
}

// Killed kills the item, removing it from maps (_ rather than !).
func (b ItemBuilder) Killed() ItemBuilder {
	b.killed = true
	return b
}

// Data gives the data, without a header. Data the builder was not given everything for (a
// symbol, or an object's or positionless weather report's timestamp) is refused as Build refuses it.
func (b ItemBuilder) Data() (Data, error) {
	d, err := b.data()
	if err != nil {
		return nil, err
	}
	return refuseMissing(d, b.common.symbol == nil)
}

func (b ItemBuilder) data() (Data, error) {
	fields, err := b.common.positioned("an item", false)
	if err != nil {
		return nil, err
	}
	return ItemReport{Name: b.name, Killed: b.killed, Fields: fields}, nil
}

// Build gives the packet: the station's header and this item, encoded.
func (b ItemBuilder) Build() (*Packet, error) { return build(b.station, b.data) }

// MicEBuilder builds a Mic-E position report, from Station.MicE (APRS12c ch. 10). The position
// comment is MicEOffDuty unless set; MicEEmergency sets off alarms, so is only sent when asked
// for.
type MicEBuilder struct {
	station     Station
	common      common
	micEMessage MicEMessage
	messaging   bool
}

// MicEMessage sets the position comment, such as MicEEnRoute.
func (b MicEBuilder) MicEMessage(message MicEMessage) MicEBuilder {
	b.micEMessage = message
	return b
}

// Messaging says the station can receive APRS messages: the ` type code, where without this it
// is ' (APRS 1.2).
func (b MicEBuilder) Messaging() MicEBuilder {
	b.messaging = true
	return b
}

// Data gives the data, without a header. Data the builder was not given everything for (a
// symbol, or an object's or positionless weather report's timestamp) is refused as Build refuses it.
func (b MicEBuilder) Data() (Data, error) {
	d, err := b.data()
	if err != nil {
		return nil, err
	}
	return refuseMissing(d, b.common.symbol == nil)
}

func (b MicEBuilder) data() (Data, error) {
	fields, err := b.common.positioned("a Mic-E report", true)
	if err != nil {
		return nil, err
	}
	// An altitude outside the Mic-E altitude bytes' range is refused (not moved to a /A=). The
	// refusal ranks with what else is wrong with the report, so the encoder is asked about the
	// rest first.
	if fields.AltitudeFeet != nil {
		metres := roundSteps(*fields.AltitudeFeet / metresToFeet)
		if !inRange(metres, -10000, 743570) {
			fault := newEncodeError(refusalForValue(*fields.AltitudeFeet), "a Mic-E altitude is -10000 to 743570 metres")
			rest := fields
			rest.AltitudeFeet = nil
			typeCode := '\''
			if _, _, other := encodeMicE(&MicEReport{MicEMessage: b.micEMessage, TypeCode: &typeCode, Fields: rest}); other != nil && other.code.rank() < fault.code.rank() {
				return nil, other
			}
			return nil, fault
		}
	}
	// The type code says whether the station can message (APRS 1.2 Mic-E type codes).
	typeCode := '\''
	if b.messaging {
		typeCode = '`'
	}
	return MicEReport{
		MicEMessage: b.micEMessage,
		TypeCode:    &typeCode,
		Fields:      fields,
	}, nil
}

// Build gives the packet: the station's source and path, the destination this report computes,
// and the report. An altitude, in either unit, goes in the Mic-E altitude bytes, rounded to the
// nearest metre.
func (b MicEBuilder) Build() (*Packet, error) { return build(b.station, b.data) }

// WeatherBuilder builds a weather report, from Station.Weather (APRS12c ch. 12). With a position
// it is a position report with the weather station symbol; without, a positionless weather
// report. Values are in the units APRS sends (mph, Fahrenheit, inches, millibars); the C and Mm
// setters convert.
type WeatherBuilder struct {
	station    Station
	weather    Weather
	position   *Position
	symbol     Symbol
	timestamp  *Timestamp
	messaging  bool
	compressed bool
	dao        *DAO
}

// At sets where the weather station is, in decimal degrees: north and east positive.
func (b WeatherBuilder) At(latitude, longitude float64) WeatherBuilder {
	b.position = &Position{Latitude: latitude, Longitude: longitude}
	return b
}

// Symbol sets the symbol, when not the plain weather station /_; it must still be a weather
// station, such as SymbolWeatherStationWithDigipeater. Only with a position.
func (b WeatherBuilder) Symbol(symbol Symbol) WeatherBuilder {
	b.symbol = symbol
	return b
}

// Timestamp sets when the observations were made. A positionless report must have one, in the
// month-day form (TimestampMDHM); a report with a position without one is current.
func (b WeatherBuilder) Timestamp(t Timestamp) WeatherBuilder {
	b.timestamp = &t
	return b
}

// Messaging says the station can receive APRS messages. Only with a position.
func (b WeatherBuilder) Messaging() WeatherBuilder {
	b.messaging = true
	return b
}

// Compressed sends the position in compressed (base-91) form, with the wind in the course and
// speed bytes (APRS12c ch. 9). Only with a position.
func (b WeatherBuilder) Compressed() WeatherBuilder {
	b.compressed = true
	return b
}

// DAO adds a !DAO! extension in the WGS84 base-91 form. Only with a position.
func (b WeatherBuilder) DAO() WeatherBuilder {
	b.dao = &DAO{Datum: 'W', Precision: DAOBase91}
	return b
}

// WindDirectionDegrees sets the wind direction in degrees clockwise from north. A direction
// between the steps the report holds (whole degrees, or 4 degrees compressed) is rounded to the
// nearest, once, when the packet is built. A compressed report needs the speed too.
func (b WeatherBuilder) WindDirectionDegrees(degrees float64) WeatherBuilder {
	b.weather.WindDirectionDegrees = Ptr(degrees)
	return b
}

// WindSpeedMph sets the sustained one-minute wind speed in mph.
func (b WeatherBuilder) WindSpeedMph(mph float64) WeatherBuilder {
	b.weather.WindSpeedMph = Ptr(mph)
	return b
}

// WindGustMph sets the peak wind gust in the last five minutes, in mph.
func (b WeatherBuilder) WindGustMph(mph float64) WeatherBuilder {
	b.weather.WindGustMph = Ptr(mph)
	return b
}

// TemperatureF sets the temperature in degrees Fahrenheit, sent to the nearest degree.
func (b WeatherBuilder) TemperatureF(fahrenheit float64) WeatherBuilder {
	b.weather.TemperatureF = Ptr(fahrenheit)
	return b
}

// TemperatureC sets the temperature in degrees Celsius, sent to the nearest degree Fahrenheit.
func (b WeatherBuilder) TemperatureC(celsius float64) WeatherBuilder {
	return b.TemperatureF(celsius*9/5 + 32)
}

// HumidityPercent sets the relative humidity, 1-100 percent, rounded to the nearest whole
// percent when the packet is built (99.7 is sent as 100%; a value that rounds to 0 is refused,
// since the report's h00 means 100).
func (b WeatherBuilder) HumidityPercent(percent float64) WeatherBuilder {
	b.weather.HumidityPercent = Ptr(percent)
	return b
}

// PressureMbar sets the barometric pressure in millibars (hPa), sent to a tenth.
func (b WeatherBuilder) PressureMbar(millibars float64) WeatherBuilder {
	b.weather.PressureMbar = Ptr(millibars)
	return b
}

// Rain1hIn sets the rainfall in the last hour, in inches.
func (b WeatherBuilder) Rain1hIn(inches float64) WeatherBuilder {
	b.weather.Rain1hIn = Ptr(inches)
	return b
}

// Rain24hIn sets the rainfall in the last 24 hours, in inches.
func (b WeatherBuilder) Rain24hIn(inches float64) WeatherBuilder {
	b.weather.Rain24hIn = Ptr(inches)
	return b
}

// RainMidnightIn sets the rainfall since local midnight, in inches.
func (b WeatherBuilder) RainMidnightIn(inches float64) WeatherBuilder {
	b.weather.RainMidnightIn = Ptr(inches)
	return b
}

// Rain1hMm sets the rainfall in the last hour, in millimetres, sent in inches.
func (b WeatherBuilder) Rain1hMm(millimetres float64) WeatherBuilder {
	return b.Rain1hIn(millimetres / 25.4)
}

// Rain24hMm sets the rainfall in the last 24 hours, in millimetres, sent in inches.
func (b WeatherBuilder) Rain24hMm(millimetres float64) WeatherBuilder {
	return b.Rain24hIn(millimetres / 25.4)
}

// RainMidnightMm sets the rainfall since local midnight, in millimetres, sent in inches.
func (b WeatherBuilder) RainMidnightMm(millimetres float64) WeatherBuilder {
	return b.RainMidnightIn(millimetres / 25.4)
}

// LuminosityWM2 sets the luminosity in watts per square metre, 0-1999, rounded to the nearest
// whole number when the packet is built.
func (b WeatherBuilder) LuminosityWM2(wattsPerSquareMetre float64) WeatherBuilder {
	b.weather.LuminosityWM2 = Ptr(wattsPerSquareMetre)
	return b
}

// Snow24hIn sets the snowfall in the last 24 hours, in inches.
func (b WeatherBuilder) Snow24hIn(inches float64) WeatherBuilder {
	b.weather.Snow24hIn = Ptr(inches)
	return b
}

// Observations sets every observation at once, replacing any set before.
func (b WeatherBuilder) Observations(weather Weather) WeatherBuilder {
	b.weather = weather
	return b
}

// Data gives the data, without a header. Data the builder was not given everything for (a
// symbol, or an object's or positionless weather report's timestamp) is refused as Build refuses it.
func (b WeatherBuilder) Data() (Data, error) {
	d, err := b.data()
	if err != nil {
		return nil, err
	}
	return refuseMissing(d, b.position == nil && b.timestamp == nil)
}

func (b WeatherBuilder) data() (Data, error) {
	weather := b.weather
	if b.position != nil {
		fields := Positioned{Position: *b.position, Symbol: b.symbol, Compressed: b.compressed, DAO: b.dao, Weather: &weather}
		return PositionReport{Timestamp: b.timestamp, Messaging: b.messaging, Fields: fields}, nil
	}
	if b.messaging || b.compressed || b.dao != nil || b.symbol != SymbolWeatherStation {
		return nil, newEncodeError(RefusalMissingField, "a symbol, messaging, compression and DAO need a position: call At(latitude, longitude)")
	}
	// A timestamp not given is left nil, which the encoder refuses as missing-field at its place.
	var t *Timestamp
	if b.timestamp != nil {
		c := *b.timestamp
		t = &c
	}
	return WeatherReport{Timestamp: t, Weather: weather}, nil
}

// Build gives the packet: the station's header and this report, encoded.
func (b WeatherBuilder) Build() (*Packet, error) { return build(b.station, b.data) }

// MessageBuilder builds a message to another station, from Station.Message (APRS12c ch. 14).
type MessageBuilder struct {
	station Station
	message Message
}

// MessageID sets a message ID (1-5 letters or digits), which asks the other station to
// acknowledge it.
func (b MessageBuilder) MessageID(id string) MessageBuilder {
	b.message.MessageID = Ptr(id)
	return b
}

// ReplyAck uses the reply-ack format (APRS12c ch. 14), which needs MessageID: it acknowledges the
// other station's message theirID in the same packet, or with "" just says this station
// understands reply-acks.
func (b MessageBuilder) ReplyAck(theirID string) MessageBuilder {
	b.message.ReplyAck = Ptr(theirID)
	return b
}

// Data gives the data, without a header.
func (b MessageBuilder) Data() (Data, error) { return b.message, nil }

// Build gives the packet: the station's header and this message, encoded.
func (b MessageBuilder) Build() (*Packet, error) { return build(b.station, b.Data) }

// AckBuilder builds an acknowledgement or rejection, from Station.Ack or Station.Reject (APRS12c
// ch. 14).
type AckBuilder struct {
	station   Station
	addressee string
	id        string
	reject    bool
	replyAck  *string
}

// ReplyAck, for a message that came in reply-ack format ({MM}AA), echoes back its AA part.
func (b AckBuilder) ReplyAck(theirAck string) AckBuilder {
	b.replyAck = Ptr(theirAck)
	return b
}

// Data gives the data, without a header.
func (b AckBuilder) Data() (Data, error) {
	if b.reject {
		return Reject{Addressee: b.addressee, RejectedID: b.id, ReplyAck: b.replyAck}, nil
	}
	return Ack{Addressee: b.addressee, AckedID: b.id, ReplyAck: b.replyAck}, nil
}

// Build gives the packet: the station's header and this acknowledgement or rejection, encoded.
func (b AckBuilder) Build() (*Packet, error) { return build(b.station, b.Data) }

// StatusBuilder builds a status report, from Station.Status (APRS12c ch. 16).
type StatusBuilder struct {
	station Station
	status  Status
}

// Timestamp sets when the status was set, as day, hours and minutes UTC (TimestampDHM). Not with
// a locator.
func (b StatusBuilder) Timestamp(t Timestamp) StatusBuilder {
	b.status.Timestamp = &t
	return b
}

// Locator sets a 4- or 6-character Maidenhead locator at the start, such as IO91SX; lower case is
// written in upper case, as a decoder reads it. It goes with a Symbol: one without the other is
// refused when built.
func (b StatusBuilder) Locator(maidenheadLocator string) StatusBuilder {
	b.status.Locator = Ptr(asciiUpper(maidenheadLocator))
	return b
}

// Symbol sets the symbol after the locator, such as SymbolHouse. Only with a Locator.
func (b StatusBuilder) Symbol(symbol Symbol) StatusBuilder {
	b.status.Symbol = &symbol
	return b
}

// Beam sets a meteor-scatter beam heading and power at the end (^HP).
func (b StatusBuilder) Beam(beam BeamHeading) StatusBuilder {
	b.status.Beam = &beam
	return b
}

// Data gives the data, without a header.
func (b StatusBuilder) Data() (Data, error) { return b.status, nil }

// Build gives the packet: the station's header and this status, encoded.
func (b StatusBuilder) Build() (*Packet, error) { return build(b.station, b.Data) }

// TelemetryBuilder builds a telemetry report, from Station.Telemetry (APRS12c ch. 13): five
// analog values and eight digital bits. The channel names, units and scaling go in separate
// messages: Station.TelemetryNames and the methods after it.
type TelemetryBuilder struct {
	station  Station
	sequence uint16
	analog   []*float64
	bits     string
	comment  string
}

// Analog sets up to 5 analog values, A1 first, each of which may be nil to send that channel
// empty, before a filled one or after; any not given are sent empty.
func (b TelemetryBuilder) Analog(values ...*float64) TelemetryBuilder {
	b.analog = make([]*float64, len(values))
	for i, v := range values {
		if v != nil {
			b.analog[i] = Ptr(*v)
		}
	}
	return b
}

// Bits sets the 8 digital channels as eight 0 or 1 characters, B1 first; all 0 unless set.
func (b TelemetryBuilder) Bits(bits string) TelemetryBuilder {
	b.bits = bits
	return b
}

// Comment sets free text after the values.
func (b TelemetryBuilder) Comment(text string) TelemetryBuilder {
	b.comment = text
	return b
}

// Data gives the data, without a header.
func (b TelemetryBuilder) Data() (Data, error) {
	if b.sequence > 999 {
		return nil, newEncodeError(RefusalValueOutOfRange, "a telemetry sequence is 0-999, three digits (APRS12c ch. 13)")
	}
	// More than 5 values go to the encoder as given, which names that (too-long) in its own order.
	analog := make([]*float64, max(5, len(b.analog)))
	copy(analog, b.analog)
	return Telemetry{Sequence: pad3(uint32(b.sequence)), Analog: analog, Bits: Ptr(b.bits), Comment: b.comment}, nil
}

// Build gives the packet: the station's header and this report, encoded.
func (b TelemetryBuilder) Build() (*Packet, error) { return build(b.station, b.Data) }

// refuseMissing is what Data does for data the builder was not given everything for (no symbol,
// no timestamp): it names the refusal the encoder gives it, at its place in the order, so Data
// refuses what Build refuses. Build itself uses the unchecked data, so the header's faults and
// these rank together in the packet's own check.
func refuseMissing(d Data, missing bool) (Data, error) {
	if !missing {
		return d, nil
	}
	if m, ok := d.(MicEReport); ok {
		if _, _, err := encodeMicE(&m); err != nil {
			return nil, err
		}
		return d, nil
	}
	if _, err := encode(d); err != nil {
		return nil, err
	}
	return d, nil
}

func build(s Station, data func() (Data, error)) (*Packet, error) {
	d, err := data()
	if err != nil {
		return nil, err
	}
	return s.packet(d)
}

// String gives the station's header, such as M0LTE-9>APZ001,WIDE1-1.
func (s Station) String() string {
	var sb strings.Builder
	sb.WriteString(s.source.text)
	sb.WriteByte('>')
	sb.WriteString(s.destination.text)
	for _, e := range s.path {
		sb.WriteByte(',')
		sb.WriteString(e.Address.text)
	}
	return sb.String()
}
