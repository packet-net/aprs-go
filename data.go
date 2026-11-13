package aprs

// Data is the decoded content of an information field: one of the concrete types in this file,
// one per APRS data type. Use a type switch to see which:
//
//	switch d := packet.Data.(type) {
//	case aprs.PositionReport:
//		...
//	case aprs.Message:
//		...
//	}
//
// Each type is also the encoder's input. New types may be added in later 1.x releases, so a
// switch over Data should have a default case.
type Data interface {
	isData()
}

// Unrecognized is the data when nothing could be decoded; the diagnostics say why.
type Unrecognized uint8

const (
	// UnrecognizedEmpty: the information field is empty.
	UnrecognizedEmpty Unrecognized = iota
	// UnrecognizedNotAprs: the first byte is not an APRS data type identifier: plain text, a
	// beacon, another protocol.
	UnrecognizedNotAprs
	// UnrecognizedReservedDataType: a reserved data type identifier with no defined format.
	UnrecognizedReservedDataType
	// UnrecognizedMalformed: it is APRS, but broken; the diagnostics say how.
	UnrecognizedMalformed
)

// Position is a latitude and longitude.
type Position struct {
	// Latitude in degrees, north positive. For an ambiguous position, the centre of the area it covers.
	Latitude float64
	// Longitude in degrees, east positive.
	Longitude float64
	// Ambiguity is how many trailing digits were blanked with spaces (0-4) (APRS12c ch. 6).
	Ambiguity uint8
}

// Symbol is a map symbol: a table (or overlay) character and a code. Every defined symbol also
// has a name, such as SymbolCar.
type Symbol struct {
	// Table is '/' for the primary table, '\\' for the alternate, or an overlay ('0'-'9',
	// 'A'-'Z') on the alternate.
	Table rune
	// Code is the symbol code, '!' to '~'.
	Code rune
}

// NewSymbol makes a symbol from its table (or overlay) character and code, such as
// NewSymbol('/', '>') for a car.
func NewSymbol(table, code rune) Symbol {
	return Symbol{Table: table, Code: code}
}

// WithOverlay gives this alternate-table symbol with an overlay character, '0'-'9' or 'A'-'Z',
// such as SymbolGateway.WithOverlay('I') for an IGate. It fails for a primary-table symbol, which
// takes no overlay, or any other character (APRS12c ch. 21).
func (s Symbol) WithOverlay(overlay rune) (Symbol, bool) {
	if s.Table != '/' && ((overlay >= '0' && overlay <= '9') || (overlay >= 'A' && overlay <= 'Z')) {
		return Symbol{Table: overlay, Code: s.Code}, true
	}
	return Symbol{}, false
}

// String gives the table and code, such as "/>".
func (s Symbol) String() string {
	return string(s.Table) + string(s.Code)
}

// TimestampKind says which of the three APRS timestamp forms a Timestamp is.
type TimestampKind uint8

const (
	// DayHourMinute: day of month, hour and minute, UTC (DDHHMMz) or local time (DDHHMM/).
	DayHourMinute TimestampKind = iota
	// HourMinuteSecond: hour, minute and second, UTC (HHMMSSh).
	HourMinuteSecond
	// MonthDayHourMinute: month, day, hour and minute, UTC (MMDDHHMM), used by positionless
	// weather reports.
	MonthDayHourMinute
)

// Timestamp is a timestamp as sent. The fields are kept even when out of range, so they can be
// reported; IsValid says whether they make sense. Make one with TimestampDHM, TimestampHMS or
// TimestampMDHM; fields the kind does not use are zero.
type Timestamp struct {
	// Kind says which form it is, and so which fields below it uses.
	Kind TimestampKind
	// Month, 1-12 (MonthDayHourMinute only).
	Month uint8
	// Day of the month, 1-31 (DayHourMinute and MonthDayHourMinute).
	Day uint8
	// Hour, 0-23.
	Hour uint8
	// Minute, 0-59.
	Minute uint8
	// Second, 0-59 (HourMinuteSecond only).
	Second uint8
	// UTC is true for a DayHourMinute timestamp in UTC (z) rather than local time (/).
	UTC bool
}

// CompressionType is a compressed position's type byte (APRS12c ch. 9).
type CompressionType struct {
	// Fix says whether the GPS fix is current.
	Fix CompressionFix
	// Source is the NMEA sentence the position came from.
	Source CompressionSource
	// Origin is what compressed the position.
	Origin CompressionOrigin
}

// CompressionFix is the GPS fix in a compression type byte.
type CompressionFix uint8

const (
	// FixOld is an old (last known) fix.
	FixOld CompressionFix = iota
	// FixCurrent is a current fix.
	FixCurrent
)

// CompressionSource is the NMEA source in a compression type byte.
type CompressionSource uint8

const (
	// SourceOther: other.
	SourceOther CompressionSource = iota
	// SourceGLL: GLL.
	SourceGLL
	// SourceGGA: GGA; the cs bytes then carry altitude.
	SourceGGA
	// SourceRMC: RMC.
	SourceRMC
)

// CompressionOrigin is the compression origin in a compression type byte.
type CompressionOrigin uint8

const (
	// OriginCompressed: compressed.
	OriginCompressed CompressionOrigin = iota
	// OriginTNCBeaconText: TNC BText.
	OriginTNCBeaconText
	// OriginSoftware: software (DOS, Mac, Win+SA).
	OriginSoftware
	// OriginReserved3: reserved (TBD).
	OriginReserved3
	// OriginKPC3: KPC3.
	OriginKPC3
	// OriginPico: Pico.
	OriginPico
	// OriginOtherTracker: other tracker.
	OriginOtherTracker
	// OriginDigipeaterConversion: digipeater conversion.
	OriginDigipeaterConversion
)

// PHG is power, height, gain and directivity (APRS12c ch. 7), as the codes sent.
type PHG struct {
	// Power code 0-9: power = code squared, in watts.
	Power uint8
	// Height code 0-9 (or higher): height = 10 x 2^code feet.
	Height uint8
	// Gain code 0-9: dBi.
	Gain uint8
	// Directivity code 0-9: 0 omni, 1-8 45 x code degrees.
	Directivity uint8
	// BeaconsPerHour is the PHGR rate, or nil.
	BeaconsPerHour *uint8
}

// PowerWatts is the power in watts the code stands for: the code squared.
func (p PHG) PowerWatts() uint32 {
	return uint32(p.Power) * uint32(p.Power)
}

// HeightFeet is the antenna height in feet the code stands for: 10 x 2^code.
func (p PHG) HeightFeet() float64 {
	return ldexp10(p.Height)
}

// GainDB is the antenna gain in dB the code stands for (dBi, as the spec gives it).
func (p PHG) GainDB() uint8 {
	return p.Gain
}

// DirectivityDegrees is the direction of maximum gain in degrees (45 x code), and false for an
// omnidirectional antenna.
func (p PHG) DirectivityDegrees() (uint16, bool) {
	if p.Directivity == 0 {
		return 0, false
	}
	return uint16(p.Directivity) * 45, true
}

// DFS is omni DF signal strength, DFSshgd (APRS12c ch. 7).
type DFS struct {
	// Strength code 0-9 (S-points).
	Strength uint8
	// Height code.
	Height uint8
	// Gain code.
	Gain uint8
	// Directivity code.
	Directivity uint8
}

// AreaShape is an area object's shape (APRS12c ch. 11), codes 0-9.
type AreaShape uint8

const (
	AreaOpenCircle AreaShape = iota
	AreaLineDownRight
	AreaOpenEllipse
	AreaOpenTriangle
	AreaOpenBox
	AreaFilledCircle
	AreaLineDownLeft
	AreaFilledEllipse
	AreaFilledTriangle
	AreaFilledBox
)

// AreaColor is an area object's colour (APRS12c ch. 11): eight colours, each high or low
// intensity, codes /0 to /9 and 10 to 15.
type AreaColor uint8

const (
	ColorBlack AreaColor = iota
	ColorBlue
	ColorGreen
	ColorCyan
	ColorRed
	ColorViolet
	ColorYellow
	ColorGray
	ColorBlackLow
	ColorBlueLow
	ColorGreenLow
	ColorCyanLow
	ColorRedLow
	ColorVioletLow
	ColorYellowLow
	ColorGrayLow
)

// AreaObject is an area object, Tyy/Cxx and optionally {www} (APRS12c ch. 11).
type AreaObject struct {
	Shape AreaShape
	// LatOffset is the latitude offset code yy: the square root of the offset in 1/100 degree.
	LatOffset uint8
	Color     AreaColor
	// LonOffset is the longitude offset code xx.
	LonOffset uint8
	// CorridorWidthMiles is a line object's corridor width, {www}, or nil.
	CorridorWidthMiles *uint16
}

// DFBearing is a DF bearing and quality, /BRG/NRQ (APRS12c ch. 7).
type DFBearing struct {
	// BearingDegrees is the bearing in degrees.
	BearingDegrees uint16
	// Number is N, the number of hits, 0-9 (0: the report is meaningless).
	Number uint8
	// Range is the R range code.
	Range uint8
	// Quality is the Q quality code (beamwidth).
	Quality uint8
}

// Storm is a storm or hurricane report in an object (APRS12c ch. 12). Each value is nil when
// sent as unknown.
type Storm struct {
	// Kind is what it is: tropical storm, hurricane or tropical depression.
	Kind StormKind
	// SustainedWindKnots is the sustained wind speed, knots.
	SustainedWindKnots *uint16
	// GustKnots is the peak gusts, knots.
	GustKnots *uint16
	// CentralPressureMbar is the central pressure, millibars.
	CentralPressureMbar *uint16
	// HurricaneRadiusNM is the radius of hurricane-force winds, nautical miles.
	HurricaneRadiusNM *uint16
	// TropicalStormRadiusNM is the radius of tropical-storm winds, nautical miles.
	TropicalStormRadiusNM *uint16
	// WholeGaleRadiusNM is the radius of whole gale winds, nautical miles, sent only in the
	// optional %ggg field.
	WholeGaleRadiusNM *uint16
}

// StormKind is the kind of storm.
type StormKind uint8

const (
	// TropicalStorm: TS.
	TropicalStorm StormKind = iota
	// Hurricane: HC.
	Hurricane
	// TropicalDepression: TD.
	TropicalDepression
)

// DAO is !DAO! extra precision and datum (APRS12c ch. 9 addendum).
type DAO struct {
	// Datum letter: 'W' for WGS84 (upper case), others as sent.
	Datum rune
	// Precision says how the extra digits are written.
	Precision DAOPrecision
}

// DAOPrecision says how a !DAO! writes its extra digits.
type DAOPrecision uint8

const (
	// DAONone: no extra digits (spaces).
	DAONone DAOPrecision = iota
	// DAOThousandths: upper-case datum, one decimal digit each, thousandths of a minute.
	DAOThousandths
	// DAOBase91: lower-case datum, one base-91 character each, 1/91 of a hundredth of a minute.
	DAOBase91
)

// CommentTelemetry is base-91 comment telemetry, |ss11223344556677| (APRS12c ch. 13 addendum).
type CommentTelemetry struct {
	// Sequence number, 0-8280.
	Sequence uint16
	// One to five analog values, 0-8280.
	Analog []uint16
	// Digital is the digital bits, if sent (B1 is the least significant bit).
	Digital *uint8
}

// VoiceFrequency is an APRS 1.2 voice frequency and its settings (APRS12c ch. 18 addendum).
type VoiceFrequency struct {
	// MHz is the frequency.
	MHz float64
	// Tone is the tone field, if one was sent.
	Tone *Tone
	// ToneValue is the tone frequency (whole Hz) or DCS code, such as 100 or 23.
	ToneValue *uint16
	// OffsetKHz is the transmit offset in kHz, signed.
	OffsetKHz *int32
	// Range in miles or (with RangeKm) kilometres.
	Range *uint16
	// RangeKm says the range is in kilometres.
	RangeKm bool
	// Narrow band (a lower-case tone letter).
	Narrow bool
	// TenKHzResolution: written with 10 kHz resolution (the MHz form with two decimals).
	TenKHzResolution bool
}

// Tone is tone squelch on a voice frequency.
type Tone uint8

const (
	// ToneOff: Toff, no tone, no DCS.
	ToneOff Tone = iota
	// ToneTone: Tnnn, a tone is sent on transmit.
	ToneTone
	// ToneCTCSS: Cnnn, CTCSS tone squelch.
	ToneCTCSS
	// ToneDCS: Dnnn, digital-coded squelch.
	ToneDCS
	// ToneBurst: 1750, a 1750 Hz tone burst opens the repeater (APRS12c ch. 18).
	ToneBurst
)

// Weather is weather data (APRS12c ch. 12), in the units APRS sends. Each value is nil when not
// sent or sent as unknown.
type Weather struct {
	// WindDirectionDegrees: whole when decoded; rounded once when encoded (to 4 degrees in a
	// compressed report).
	WindDirectionDegrees *float64
	// WindSpeedMph is the sustained wind speed.
	WindSpeedMph *float64
	// WindGustMph is the peak gust in the last 5 minutes.
	WindGustMph *float64
	// TemperatureF is the temperature in degrees Fahrenheit.
	TemperatureF *float64
	// Rain1hIn is rain in the last hour, inches.
	Rain1hIn *float64
	// Rain24hIn is rain in the last 24 hours, inches.
	Rain24hIn *float64
	// RainMidnightIn is rain since local midnight, inches.
	RainMidnightIn *float64
	// RainRaw is the raw rain counter (#).
	RainRaw *uint32
	// HumidityPercent: 1-100 once rounded. Whole when decoded; rounded once when encoded.
	HumidityPercent *float64
	// PressureMbar is barometric pressure in millibars (tenths as sent).
	PressureMbar *float64
	// LuminosityWM2 is luminosity in W/m2 (0-1999 once rounded). Whole when decoded; rounded
	// once when encoded.
	LuminosityWM2 *float64
	// Snow24hIn is snowfall in the last 24 hours, inches.
	Snow24hIn *float64
	// Software is the software type character, such as 'e' or 'W'.
	Software *rune
	// Unit is the weather unit type, such as "Dvs".
	Unit *string
	// Extra holds fields in the weather run this decoder does not name (letter and value as sent).
	Extra []WeatherField
}

// WeatherField is a weather field with a letter the spec does not define.
type WeatherField struct {
	Letter rune
	// Value as sent.
	Value string
}

// Positioned holds the fields shared by positions, Mic-E reports, objects and items.
type Positioned struct {
	// Position is where.
	Position Position
	// Symbol is how to draw it on a map.
	Symbol Symbol
	// Compressed: sent in compressed form (APRS12c ch. 9).
	Compressed bool
	// Compression is the compressed type byte, when it says anything (course/speed, range or
	// altitude present).
	Compression *CompressionType
	// CourseDegrees: 0 is the spec's unknown course (a decoder reports it for 000) and 360 is
	// north; 1-360 is a known course. Whole when decoded; rounded once when encoded (to 4 degrees
	// in a compressed report).
	CourseDegrees *float64
	// SpeedKnots is the speed over ground, knots.
	SpeedKnots *float64
	// AltitudeFeet is the altitude, feet: from a /A=, the compressed cs bytes or Mic-E.
	AltitudeFeet *float64
	// PHG is power, antenna height, gain and directivity.
	PHG *PHG
	// RangeMiles is the radio range (RNGrrrr or compressed).
	RangeMiles *float64
	// DFS is omni DF signal strength.
	DFS *DFS
	// Area is an area object's shape and size, on the area symbol.
	Area *AreaObject
	// DFBearing is a DF bearing and quality, on the DF symbol.
	DFBearing *DFBearing
	// Storm is storm data, on the hurricane symbol.
	Storm *Storm
	// DAO is a !DAO! datum and its precision; any extra digits are already in Position.
	DAO *DAO
	// Telemetry is base-91 comment telemetry.
	Telemetry *CommentTelemetry
	// Frequency is an APRS 1.2 voice frequency at the start of the comment.
	Frequency *VoiceFrequency
	// Weather, for a weather station symbol.
	Weather *Weather
	// Signpost is the text of {...} on the signpost symbol.
	Signpost *string
	// Comment is what is left of the comment once everything above is lifted out.
	Comment string
}

// PositionReport is a position report: !, =, / or @ (APRS12c ch. 8 and 9).
type PositionReport struct {
	// Timestamp, if timestamped (/ or @).
	Timestamp *Timestamp
	// Messaging: the sender can do APRS messaging (= or @).
	Messaging bool
	Fields    Positioned
}

// MicEReport is a Mic-E position report: ` or ' (APRS12c ch. 10).
type MicEReport struct {
	MicEMessage MicEMessage
	// OldData: sent with ' (old or non-current data) rather than `.
	OldData bool
	// TypeCode is the device type code after the symbol: `, ', >, ] or a space.
	TypeCode *rune
	// DeviceSuffix is the device suffix after the status text, such as "=" or "_3".
	DeviceSuffix string
	// Locator is a grid locator in the status text.
	Locator *string
	// LegacyTelemetry is obsolete Mic-E telemetry.
	LegacyTelemetry []byte
	// DestinationSSID is the destination address's SSID, which selects a digipeat path.
	DestinationSSID uint8
	// AltitudeInComment: the altitude is written in the status text as /A= (feet) rather than in
	// the Mic-E altitude bytes (whole metres). Set by the decoder when the altitude came from a
	// /A=, and honoured by the encoder, so that encoding a copy of a report writes the bytes the
	// report was built from. An altitude the Mic-E bytes cannot hold exactly is written as /A=
	// whatever this says.
	AltitudeInComment bool
	Fields            Positioned
}

// MicEMessage is a Mic-E message code (APRS12c ch. 10). Custom codes are MicECustom(0) to
// MicECustom(6).
type MicEMessage int

const (
	// MicEOffDuty is M0.
	MicEOffDuty MicEMessage = iota
	// MicEEnRoute is M1.
	MicEEnRoute
	// MicEInService is M2.
	MicEInService
	// MicEReturning is M3.
	MicEReturning
	// MicECommitted is M4.
	MicECommitted
	// MicESpecial is M5.
	MicESpecial
	// MicEPriority is M6.
	MicEPriority
	// MicEEmergency: all message bits zero.
	MicEEmergency
	// MicEUnknown: a mix of standard and custom bits, which has no meaning.
	MicEUnknown
)

const micECustomBase = 100

// MicECustom is the custom message code Cn (0-6).
func MicECustom(n uint8) MicEMessage {
	return MicEMessage(micECustomBase + int(n))
}

// Custom gives n for a custom message code Cn.
func (m MicEMessage) Custom() (uint8, bool) {
	if m >= micECustomBase && m <= micECustomBase+255 {
		return uint8(m - micECustomBase), true
	}
	return 0, false
}

// ObjectReport is an object report: ; (APRS12c ch. 11).
type ObjectReport struct {
	// Name is the object's name, 1-9 characters.
	Name string
	// Killed (_) rather than alive (*).
	Killed    bool
	Timestamp *Timestamp
	Fields    Positioned
}

// ItemReport is an item report: ) (APRS12c ch. 11).
type ItemReport struct {
	// Name is the item's name, 3-9 characters.
	Name string
	// Killed (_) rather than alive (!).
	Killed bool
	Fields Positioned
}

// Message is a message to a station (APRS12c ch. 14).
type Message struct {
	// Addressee is the station it is for, without the padding to 9 characters.
	Addressee string
	// Text is the message text.
	Text string
	// MessageID is the message number, if an acknowledgement is wanted.
	MessageID *string
	// ReplyAck: a pointer to "" says the sender supports reply-acks; otherwise the ID being acked.
	ReplyAck *string
}

// Ack is a message acknowledgement.
type Ack struct {
	Addressee string
	// AckedID is the message ID acknowledged.
	AckedID string
	// MessageID is a message ID of its own (not allowed, UAP 5.32).
	MessageID *string
	ReplyAck  *string
}

// Reject is a message rejection.
type Reject struct {
	Addressee string
	// RejectedID is the message ID rejected.
	RejectedID string
	// MessageID is a message ID of its own (not allowed, UAP 5.32).
	MessageID *string
	ReplyAck  *string
}

// Bulletin is a bulletin or announcement (BLNn, BLNx, BLNnGROUP).
type Bulletin struct {
	// Addressee, such as BLN3.
	Addressee string
	// Text is the bulletin text.
	Text string
	// MessageID, if sent.
	MessageID *string
}

// NWSBulletin is a National Weather Service bulletin, addressed NWS- or NWS_.
type NWSBulletin struct {
	// Addressee, such as NWS-WARN.
	Addressee string
	// Text is the bulletin text.
	Text string
	// MessageID, if sent.
	MessageID *string
}

// TelemetryNames is telemetry channel names, PARM. (APRS12c ch. 13).
type TelemetryNames struct {
	// Addressee is the station the metadata describes.
	Addressee string
	// Names: up to 13, in channel order (A1-A5, then B1-B8); empty for a channel not named.
	Names []string
	// MessageID, when the metadata was sent as a numbered message.
	MessageID *string
}

// TelemetryUnits is telemetry channel units, UNIT.
type TelemetryUnits struct {
	Addressee string
	// Units: up to 13, in channel order (A1-A5, then B1-B8); empty for a channel without one.
	Units     []string
	MessageID *string
}

// TelemetryCoefficients is telemetry equation coefficients, EQNS. CoefficientsText keeps the
// numbers as sent (".53"), so that a received packet writes back byte for byte; it is optional
// when encoding.
type TelemetryCoefficients struct {
	Addressee string
	// Coefficients: up to 15 values, a, b, c for each analog channel in turn.
	Coefficients []float64
	// CoefficientsText: the values as sent, one for each of Coefficients (or empty). The encoder
	// writes a text that is a coefficient as the spec allows and still says its number, and
	// otherwise writes the number as the shortest decimal that reads back the same, with no
	// exponent.
	CoefficientsText []string
	MessageID        *string
}

// TelemetryBits is telemetry bit sense and project name, BITS.
type TelemetryBits struct {
	Addressee string
	// Bits: the eight bit-sense characters, B1 first.
	Bits string
	// Project is the project title.
	Project   string
	MessageID *string
}

// DirectedQuery is a query sent as a message: ?APRSx, ?WX, ... (APRS12c ch. 15).
type DirectedQuery struct {
	// Addressee is the station asked.
	Addressee string
	// QueryType, such as APRSD, PING? or WX.
	QueryType string
	// Target is the station the query is about, for the types that name one.
	Target *string
}

// Status is a status report: > (APRS12c ch. 16).
type Status struct {
	// Timestamp (DDHHMMz only).
	Timestamp *Timestamp
	// Locator is a Maidenhead locator, for the grid format.
	Locator *string
	// Symbol, for the grid format.
	Symbol *Symbol
	// Beam is a meteor-scatter beam heading and power, ^HP (APRS12c ch. 16).
	Beam *BeamHeading
	Text string
}

// BeamHeading is a beam heading and power in a status report, as the codes sent.
type BeamHeading struct {
	// HeadingCode: '0'-'9' or 'A'-'Z', 10 degrees each.
	HeadingCode rune
	// PowerCode: the PHG power code.
	PowerCode rune
}

// Telemetry is a telemetry report: T# (APRS12c ch. 13). AnalogText keeps the values as sent
// ("073", "190.0"), so that a received packet writes back byte for byte; it is optional when
// encoding.
type Telemetry struct {
	// Sequence as sent: usually three digits, or MIC.
	Sequence string
	// Analog: five values; nil for a channel sent empty.
	Analog []*float64
	// AnalogText: the analog values as sent, one for each of Analog (or empty); an empty string
	// for a channel sent empty. The encoder writes a text that is a telemetry value and still says
	// its number, and otherwise writes the number: three digits for a whole number from 0 to 999,
	// else the shortest decimal that reads back the same, with no exponent.
	AnalogText []string
	// Bits: the eight digital bits as on air, B1 first.
	Bits *string
	// Comment is the text after the bits.
	Comment string
}

// WeatherReport is a positionless weather report: _ (APRS12c ch. 12).
type WeatherReport struct {
	// Timestamp (MDHM).
	Timestamp *Timestamp
	Weather   Weather
	// Comment is text after the weather data (not allowed; see CodeWeatherComment).
	Comment string
}

// RawWeather is raw weather station output: #, *, $ULTW, !! (APRS12c ch. 12).
type RawWeather struct {
	Format RawWeatherFormat
	// Data after the identifier.
	Data string
}

// RawWeatherFormat is a raw weather format.
type RawWeatherFormat uint8

const (
	// PeetBrosHash: #, Peet Bros U-II, hash format.
	PeetBrosHash RawWeatherFormat = iota
	// PeetBrosStar: *, Peet Bros U-II, star format.
	PeetBrosStar
	// UltimeterPacket: $ULTW, Ultimeter 2000 packet mode.
	UltimeterPacket
	// UltimeterLogging: !!, Ultimeter logging mode.
	UltimeterLogging
)

// NMEA is a raw NMEA 0183 sentence: $GPxxx (APRS12c ch. 5 and 7).
type NMEA struct {
	// Sentence as sent, without the $, up to and including any *hh checksum.
	Sentence string
	// HasChecksum: the sentence ends in a *hh checksum (which matched).
	HasChecksum bool
	// Latitude, if the sentence type carries one. The fields below are read from GGA, GLL, RMC,
	// VTG and WPL sentences only, and each is nil when it is missing or does not parse; a
	// position needs both coordinates.
	Latitude  *float64
	Longitude *float64
	// FixValid: RMC's or GLL's status (A or V), or GGA's quality digit.
	FixValid *bool
	// CourseDegrees is course over ground.
	CourseDegrees *float64
	// SpeedKnots is speed over ground.
	SpeedKnots *float64
	// AltitudeM is altitude in metres.
	AltitudeM *float64
	// Time: UTC time, HH:MM:SS, with any fraction of a second as sent (less trailing zeros).
	Time *string
	// Waypoint is a waypoint name.
	Waypoint *string
	// Comment is text after the checksum, kept as sent. Only a sentence with a checksum can have
	// one, since the checksum is what ends the sentence.
	Comment string
}

// MaidenheadBeacon is a Maidenhead locator beacon: [ (obsolete, APRS12c ch. 7).
type MaidenheadBeacon struct {
	Locator string
	Comment string
}

// Query is a general query: ?APRS?, ?IGATE?, ... (APRS12c ch. 15).
type Query struct {
	// QueryType, such as APRS or IGATE.
	QueryType string
	// Footprint is the area it is limited to.
	Footprint *Footprint
}

// Footprint is a query's target area. LatitudeText and LongitudeText keep the numbers as sent, a
// leading space included, so that " 34.0" and "-.1715" write back unchanged; they are optional
// when encoding. NewFootprint makes one without them.
type Footprint struct {
	// Latitude in decimal degrees, north positive, within 90 degrees.
	Latitude float64
	// Longitude in decimal degrees, east positive, within 180 degrees.
	Longitude float64
	// RadiusMiles: whole when decoded; rounded once when encoded, to 0-9999.
	RadiusMiles float64
	// LatitudeText: the latitude as sent, or empty. The encoder writes a text of the form
	// APRS12c ch. 15 gives that still says the latitude, and otherwise the number as the shortest
	// decimal that reads back the same, after a space unless negative.
	LatitudeText string
	// LongitudeText: the longitude as sent, written as LatitudeText is.
	LongitudeText string
}

// NewFootprint makes a footprint from degrees (north and east positive) and a radius in miles,
// with no text as sent: the encoder writes the numbers as APRS12c ch. 15 writes them, a positive
// value after a space.
func NewFootprint(latitude, longitude, radiusMiles float64) Footprint {
	return Footprint{Latitude: latitude, Longitude: longitude, RadiusMiles: radiusMiles}
}

// Capabilities is station capabilities: < (APRS12c ch. 15).
type Capabilities struct {
	// Capabilities: TOKEN or TOKEN=VALUE items, in order.
	Capabilities []Capability
}

// Capability is one station capability: a token and, for TOKEN=VALUE, its value.
type Capability struct {
	// Token is the capability's name, such as IGATE.
	Token string
	// Value is the text after =, or nil for a bare TOKEN.
	Value *string
}

// ThirdParty is third-party traffic: } and a whole packet inside (APRS12c ch. 17).
type ThirdParty struct {
	// Packet is the packet carried inside, decoded with the same options. Its Information is
	// part of the outer packet's, and its ThirdParty field is true.
	Packet *Packet
}

// UserDefined is user-defined data: { (APRS12c ch. 19).
type UserDefined struct {
	UserID     rune
	PacketType rune
	// Data is the rest of the information field, as sent.
	Data []byte
}

// TestData is invalid or test data: , (APRS12c ch. 20).
type TestData struct {
	// Data is the text after the ,.
	Data string
}

// AgreloDF is an Agrelo DFJr bearing report: %.
type AgreloDF struct {
	BearingDegrees uint16
	// Quality, 0-9.
	Quality uint8
}

func (PositionReport) isData()        {}
func (MicEReport) isData()            {}
func (ObjectReport) isData()          {}
func (ItemReport) isData()            {}
func (Message) isData()               {}
func (Ack) isData()                   {}
func (Reject) isData()                {}
func (Bulletin) isData()              {}
func (NWSBulletin) isData()           {}
func (TelemetryNames) isData()        {}
func (TelemetryUnits) isData()        {}
func (TelemetryCoefficients) isData() {}
func (TelemetryBits) isData()         {}
func (DirectedQuery) isData()         {}
func (Status) isData()                {}
func (Telemetry) isData()             {}
func (WeatherReport) isData()         {}
func (RawWeather) isData()            {}
func (NMEA) isData()                  {}
func (MaidenheadBeacon) isData()      {}
func (Query) isData()                 {}
func (Capabilities) isData()          {}
func (ThirdParty) isData()            {}
func (UserDefined) isData()           {}
func (TestData) isData()              {}
func (AgreloDF) isData()              {}
func (Unrecognized) isData()          {}

// Ptr gives a pointer to a copy of v, for the optional fields: Ptr(88.0), Ptr("01").
func Ptr[T any](v T) *T {
	return &v
}

// Description says what the symbol stands for, from the symbol tables ("Car", "Police,
// Sheriff"), ignoring any overlay; false for a table and code the tables do not define.
func (s Symbol) Description() (string, bool) {
	table := '\\'
	if s.Table == '/' {
		table = '/'
	}
	d, ok := symbolDescriptions[Symbol{Table: table, Code: s.Code}]
	return d, ok
}
