// The methods every positioned builder shares, one set per builder from one template; keep the four sets alike.

package aprs

// At sets where it is, in decimal degrees: north and east positive.
func (b PositionBuilder) At(latitude, longitude float64) PositionBuilder {
	b.common.position = &Position{Latitude: latitude, Longitude: longitude}
	return b
}

// Symbol sets how to draw it, such as SymbolCar.
func (b PositionBuilder) Symbol(symbol Symbol) PositionBuilder {
	b.common.symbol = &symbol
	return b
}

// CourseDegrees sets the course over ground in degrees clockwise from north, 1-360: 360 is north,
// and 0 is the spec's unknown course (APRS12c ch. 7 and 10), which a compressed report cannot
// send. A course between the steps the report holds (whole degrees, or 4 degrees compressed) is
// rounded to the nearest, once, when the packet is built.
func (b PositionBuilder) CourseDegrees(degrees float64) PositionBuilder {
	b.common.fields.CourseDegrees = Ptr(degrees)
	return b
}

// SpeedKnots sets the speed over ground in knots.
func (b PositionBuilder) SpeedKnots(knots float64) PositionBuilder {
	b.common.fields.SpeedKnots = Ptr(knots)
	return b
}

// SpeedKmh sets the speed over ground in kilometres per hour, sent in knots.
func (b PositionBuilder) SpeedKmh(kilometresPerHour float64) PositionBuilder {
	return b.SpeedKnots(kilometresPerHour / 1.852)
}

// AltitudeFeet sets the altitude above mean sea level in feet (/A=), sent to the nearest foot.
func (b PositionBuilder) AltitudeFeet(feet float64) PositionBuilder {
	b.common.fields.AltitudeFeet = Ptr(feet)
	b.common.altitudeM = nil
	return b
}

// AltitudeM sets the altitude above mean sea level in metres: in a Mic-E report, in its altitude
// bytes to the nearest metre; otherwise converted to feet for a /A=, to the nearest foot.
func (b PositionBuilder) AltitudeM(metres float64) PositionBuilder {
	b.common.fields.AltitudeFeet = nil
	b.common.altitudeM = Ptr(metres)
	return b
}

// Comment sets free text after the structured parts. It may contain UTF-8.
func (b PositionBuilder) Comment(text string) PositionBuilder {
	b.common.fields.Comment = text
	return b
}

// PHG sets the power, antenna height, gain and directivity codes (PHGphgd, APRS12c ch. 7).
func (b PositionBuilder) PHG(phg PHG) PositionBuilder {
	if phg.BeaconsPerHour != nil {
		phg.BeaconsPerHour = Ptr(*phg.BeaconsPerHour)
	}
	b.common.fields.PHG = &phg
	return b
}

// RangeMiles sets the omnidirectional radio range in miles (RNGrrrr, APRS12c ch. 7).
func (b PositionBuilder) RangeMiles(miles float64) PositionBuilder {
	b.common.fields.RangeMiles = Ptr(miles)
	return b
}

// Frequency sets a voice frequency at the start of the comment, as radios with a tune button read
// it (145.725MHz, APRS12c ch. 18), rounded to the nearest kHz. Add Tone and OffsetKHz after it.
func (b PositionBuilder) Frequency(mhz float64) PositionBuilder {
	b.common.fields.Frequency = &VoiceFrequency{MHz: mhz}
	return b
}

// Tone sets the tone field for Frequency: a kind, ToneTone, ToneCTCSS or ToneDCS, and its value as
// sent, a whole number of Hz or a DCS code (ToneTone, 118 sends T118; 118.8 Hz is sent as 118).
// ToneOff and ToneBurst have no value, which is ignored.
func (b PositionBuilder) Tone(tone Tone, value uint16) PositionBuilder {
	b.common.tone = &toneSetting{tone: tone, value: value}
	return b
}

// OffsetKHz sets the transmit offset for Frequency, in kHz, such as -600; sent in tens of kHz.
func (b PositionBuilder) OffsetKHz(khz int32) PositionBuilder {
	b.common.offsetKHz = Ptr(khz)
	return b
}

// VoiceFrequency sets a voice frequency with every option (APRS12c ch. 18).
func (b PositionBuilder) VoiceFrequency(frequency VoiceFrequency) PositionBuilder {
	b.common.fields.Frequency = &frequency
	return b
}

// Compressed sends the position in compressed (base-91) form: shorter and more precise, but with
// no room for PHG or other data extensions (APRS12c ch. 9).
func (b PositionBuilder) Compressed() PositionBuilder {
	b.common.fields.Compressed = true
	return b
}

// Ambiguity blanks the last 1-4 digits of the position to hide it: to about 0.1, 1, 10 or 60
// nautical miles (APRS12c ch. 6). The digits are cut, so the report gives the box the position is
// in, and a decoder reads its centre. Not with Compressed or DAO.
func (b PositionBuilder) Ambiguity(digits uint8) PositionBuilder {
	b.common.ambiguity = digits
	return b
}

// DAO adds a !DAO! extension carrying extra position precision, in the WGS84 base-91 form (about
// 0.2 m).
func (b PositionBuilder) DAO() PositionBuilder {
	b.common.fields.DAO = &DAO{Datum: 'W', Precision: DAOBase91}
	return b
}

// Telemetry adds base-91 telemetry to the comment (|ss1122..|, APRS12c ch. 13): a sequence
// number, 1-5 analog values, each 0-8280, and optionally the 8 digital bits, which can only follow
// all 5 analog values.
func (b PositionBuilder) Telemetry(telemetry CommentTelemetry) PositionBuilder {
	telemetry.Analog = append([]uint16(nil), telemetry.Analog...)
	b.common.fields.Telemetry = &telemetry
	return b
}

// At sets where it is, in decimal degrees: north and east positive.
func (b ObjectBuilder) At(latitude, longitude float64) ObjectBuilder {
	b.common.position = &Position{Latitude: latitude, Longitude: longitude}
	return b
}

// Symbol sets how to draw it, such as SymbolCar.
func (b ObjectBuilder) Symbol(symbol Symbol) ObjectBuilder {
	b.common.symbol = &symbol
	return b
}

// CourseDegrees sets the course over ground in degrees clockwise from north, 1-360: 360 is north,
// and 0 is the spec's unknown course (APRS12c ch. 7 and 10), which a compressed report cannot
// send. A course between the steps the report holds (whole degrees, or 4 degrees compressed) is
// rounded to the nearest, once, when the packet is built.
func (b ObjectBuilder) CourseDegrees(degrees float64) ObjectBuilder {
	b.common.fields.CourseDegrees = Ptr(degrees)
	return b
}

// SpeedKnots sets the speed over ground in knots.
func (b ObjectBuilder) SpeedKnots(knots float64) ObjectBuilder {
	b.common.fields.SpeedKnots = Ptr(knots)
	return b
}

// SpeedKmh sets the speed over ground in kilometres per hour, sent in knots.
func (b ObjectBuilder) SpeedKmh(kilometresPerHour float64) ObjectBuilder {
	return b.SpeedKnots(kilometresPerHour / 1.852)
}

// AltitudeFeet sets the altitude above mean sea level in feet (/A=), sent to the nearest foot.
func (b ObjectBuilder) AltitudeFeet(feet float64) ObjectBuilder {
	b.common.fields.AltitudeFeet = Ptr(feet)
	b.common.altitudeM = nil
	return b
}

// AltitudeM sets the altitude above mean sea level in metres: in a Mic-E report, in its altitude
// bytes to the nearest metre; otherwise converted to feet for a /A=, to the nearest foot.
func (b ObjectBuilder) AltitudeM(metres float64) ObjectBuilder {
	b.common.fields.AltitudeFeet = nil
	b.common.altitudeM = Ptr(metres)
	return b
}

// Comment sets free text after the structured parts. It may contain UTF-8.
func (b ObjectBuilder) Comment(text string) ObjectBuilder {
	b.common.fields.Comment = text
	return b
}

// PHG sets the power, antenna height, gain and directivity codes (PHGphgd, APRS12c ch. 7).
func (b ObjectBuilder) PHG(phg PHG) ObjectBuilder {
	if phg.BeaconsPerHour != nil {
		phg.BeaconsPerHour = Ptr(*phg.BeaconsPerHour)
	}
	b.common.fields.PHG = &phg
	return b
}

// RangeMiles sets the omnidirectional radio range in miles (RNGrrrr, APRS12c ch. 7).
func (b ObjectBuilder) RangeMiles(miles float64) ObjectBuilder {
	b.common.fields.RangeMiles = Ptr(miles)
	return b
}

// Frequency sets a voice frequency at the start of the comment, as radios with a tune button read
// it (145.725MHz, APRS12c ch. 18), rounded to the nearest kHz. Add Tone and OffsetKHz after it.
func (b ObjectBuilder) Frequency(mhz float64) ObjectBuilder {
	b.common.fields.Frequency = &VoiceFrequency{MHz: mhz}
	return b
}

// Tone sets the tone field for Frequency: a kind, ToneTone, ToneCTCSS or ToneDCS, and its value as
// sent, a whole number of Hz or a DCS code (ToneTone, 118 sends T118; 118.8 Hz is sent as 118).
// ToneOff and ToneBurst have no value, which is ignored.
func (b ObjectBuilder) Tone(tone Tone, value uint16) ObjectBuilder {
	b.common.tone = &toneSetting{tone: tone, value: value}
	return b
}

// OffsetKHz sets the transmit offset for Frequency, in kHz, such as -600; sent in tens of kHz.
func (b ObjectBuilder) OffsetKHz(khz int32) ObjectBuilder {
	b.common.offsetKHz = Ptr(khz)
	return b
}

// VoiceFrequency sets a voice frequency with every option (APRS12c ch. 18).
func (b ObjectBuilder) VoiceFrequency(frequency VoiceFrequency) ObjectBuilder {
	b.common.fields.Frequency = &frequency
	return b
}

// Compressed sends the position in compressed (base-91) form: shorter and more precise, but with
// no room for PHG or other data extensions (APRS12c ch. 9).
func (b ObjectBuilder) Compressed() ObjectBuilder {
	b.common.fields.Compressed = true
	return b
}

// Ambiguity blanks the last 1-4 digits of the position to hide it: to about 0.1, 1, 10 or 60
// nautical miles (APRS12c ch. 6). The digits are cut, so the report gives the box the position is
// in, and a decoder reads its centre. Not with Compressed or DAO.
func (b ObjectBuilder) Ambiguity(digits uint8) ObjectBuilder {
	b.common.ambiguity = digits
	return b
}

// DAO adds a !DAO! extension carrying extra position precision, in the WGS84 base-91 form (about
// 0.2 m).
func (b ObjectBuilder) DAO() ObjectBuilder {
	b.common.fields.DAO = &DAO{Datum: 'W', Precision: DAOBase91}
	return b
}

// Telemetry adds base-91 telemetry to the comment (|ss1122..|, APRS12c ch. 13): a sequence
// number, 1-5 analog values, each 0-8280, and optionally the 8 digital bits, which can only follow
// all 5 analog values.
func (b ObjectBuilder) Telemetry(telemetry CommentTelemetry) ObjectBuilder {
	telemetry.Analog = append([]uint16(nil), telemetry.Analog...)
	b.common.fields.Telemetry = &telemetry
	return b
}

// At sets where it is, in decimal degrees: north and east positive.
func (b ItemBuilder) At(latitude, longitude float64) ItemBuilder {
	b.common.position = &Position{Latitude: latitude, Longitude: longitude}
	return b
}

// Symbol sets how to draw it, such as SymbolCar.
func (b ItemBuilder) Symbol(symbol Symbol) ItemBuilder {
	b.common.symbol = &symbol
	return b
}

// CourseDegrees sets the course over ground in degrees clockwise from north, 1-360: 360 is north,
// and 0 is the spec's unknown course (APRS12c ch. 7 and 10), which a compressed report cannot
// send. A course between the steps the report holds (whole degrees, or 4 degrees compressed) is
// rounded to the nearest, once, when the packet is built.
func (b ItemBuilder) CourseDegrees(degrees float64) ItemBuilder {
	b.common.fields.CourseDegrees = Ptr(degrees)
	return b
}

// SpeedKnots sets the speed over ground in knots.
func (b ItemBuilder) SpeedKnots(knots float64) ItemBuilder {
	b.common.fields.SpeedKnots = Ptr(knots)
	return b
}

// SpeedKmh sets the speed over ground in kilometres per hour, sent in knots.
func (b ItemBuilder) SpeedKmh(kilometresPerHour float64) ItemBuilder {
	return b.SpeedKnots(kilometresPerHour / 1.852)
}

// AltitudeFeet sets the altitude above mean sea level in feet (/A=), sent to the nearest foot.
func (b ItemBuilder) AltitudeFeet(feet float64) ItemBuilder {
	b.common.fields.AltitudeFeet = Ptr(feet)
	b.common.altitudeM = nil
	return b
}

// AltitudeM sets the altitude above mean sea level in metres: in a Mic-E report, in its altitude
// bytes to the nearest metre; otherwise converted to feet for a /A=, to the nearest foot.
func (b ItemBuilder) AltitudeM(metres float64) ItemBuilder {
	b.common.fields.AltitudeFeet = nil
	b.common.altitudeM = Ptr(metres)
	return b
}

// Comment sets free text after the structured parts. It may contain UTF-8.
func (b ItemBuilder) Comment(text string) ItemBuilder {
	b.common.fields.Comment = text
	return b
}

// PHG sets the power, antenna height, gain and directivity codes (PHGphgd, APRS12c ch. 7).
func (b ItemBuilder) PHG(phg PHG) ItemBuilder {
	if phg.BeaconsPerHour != nil {
		phg.BeaconsPerHour = Ptr(*phg.BeaconsPerHour)
	}
	b.common.fields.PHG = &phg
	return b
}

// RangeMiles sets the omnidirectional radio range in miles (RNGrrrr, APRS12c ch. 7).
func (b ItemBuilder) RangeMiles(miles float64) ItemBuilder {
	b.common.fields.RangeMiles = Ptr(miles)
	return b
}

// Frequency sets a voice frequency at the start of the comment, as radios with a tune button read
// it (145.725MHz, APRS12c ch. 18), rounded to the nearest kHz. Add Tone and OffsetKHz after it.
func (b ItemBuilder) Frequency(mhz float64) ItemBuilder {
	b.common.fields.Frequency = &VoiceFrequency{MHz: mhz}
	return b
}

// Tone sets the tone field for Frequency: a kind, ToneTone, ToneCTCSS or ToneDCS, and its value as
// sent, a whole number of Hz or a DCS code (ToneTone, 118 sends T118; 118.8 Hz is sent as 118).
// ToneOff and ToneBurst have no value, which is ignored.
func (b ItemBuilder) Tone(tone Tone, value uint16) ItemBuilder {
	b.common.tone = &toneSetting{tone: tone, value: value}
	return b
}

// OffsetKHz sets the transmit offset for Frequency, in kHz, such as -600; sent in tens of kHz.
func (b ItemBuilder) OffsetKHz(khz int32) ItemBuilder {
	b.common.offsetKHz = Ptr(khz)
	return b
}

// VoiceFrequency sets a voice frequency with every option (APRS12c ch. 18).
func (b ItemBuilder) VoiceFrequency(frequency VoiceFrequency) ItemBuilder {
	b.common.fields.Frequency = &frequency
	return b
}

// Compressed sends the position in compressed (base-91) form: shorter and more precise, but with
// no room for PHG or other data extensions (APRS12c ch. 9).
func (b ItemBuilder) Compressed() ItemBuilder {
	b.common.fields.Compressed = true
	return b
}

// Ambiguity blanks the last 1-4 digits of the position to hide it: to about 0.1, 1, 10 or 60
// nautical miles (APRS12c ch. 6). The digits are cut, so the report gives the box the position is
// in, and a decoder reads its centre. Not with Compressed or DAO.
func (b ItemBuilder) Ambiguity(digits uint8) ItemBuilder {
	b.common.ambiguity = digits
	return b
}

// DAO adds a !DAO! extension carrying extra position precision, in the WGS84 base-91 form (about
// 0.2 m).
func (b ItemBuilder) DAO() ItemBuilder {
	b.common.fields.DAO = &DAO{Datum: 'W', Precision: DAOBase91}
	return b
}

// Telemetry adds base-91 telemetry to the comment (|ss1122..|, APRS12c ch. 13): a sequence
// number, 1-5 analog values, each 0-8280, and optionally the 8 digital bits, which can only follow
// all 5 analog values.
func (b ItemBuilder) Telemetry(telemetry CommentTelemetry) ItemBuilder {
	telemetry.Analog = append([]uint16(nil), telemetry.Analog...)
	b.common.fields.Telemetry = &telemetry
	return b
}

// At sets where it is, in decimal degrees: north and east positive.
func (b MicEBuilder) At(latitude, longitude float64) MicEBuilder {
	b.common.position = &Position{Latitude: latitude, Longitude: longitude}
	return b
}

// Symbol sets how to draw it, such as SymbolCar.
func (b MicEBuilder) Symbol(symbol Symbol) MicEBuilder {
	b.common.symbol = &symbol
	return b
}

// CourseDegrees sets the course over ground in degrees clockwise from north, 1-360: 360 is north,
// and 0 is the spec's unknown course (APRS12c ch. 7 and 10), which a compressed report cannot
// send. A course between the steps the report holds (whole degrees, or 4 degrees compressed) is
// rounded to the nearest, once, when the packet is built.
func (b MicEBuilder) CourseDegrees(degrees float64) MicEBuilder {
	b.common.fields.CourseDegrees = Ptr(degrees)
	return b
}

// SpeedKnots sets the speed over ground in knots.
func (b MicEBuilder) SpeedKnots(knots float64) MicEBuilder {
	b.common.fields.SpeedKnots = Ptr(knots)
	return b
}

// SpeedKmh sets the speed over ground in kilometres per hour, sent in knots.
func (b MicEBuilder) SpeedKmh(kilometresPerHour float64) MicEBuilder {
	return b.SpeedKnots(kilometresPerHour / 1.852)
}

// AltitudeFeet sets the altitude above mean sea level in feet (/A=), sent to the nearest foot.
func (b MicEBuilder) AltitudeFeet(feet float64) MicEBuilder {
	b.common.fields.AltitudeFeet = Ptr(feet)
	b.common.altitudeM = nil
	return b
}

// AltitudeM sets the altitude above mean sea level in metres: in a Mic-E report, in its altitude
// bytes to the nearest metre; otherwise converted to feet for a /A=, to the nearest foot.
func (b MicEBuilder) AltitudeM(metres float64) MicEBuilder {
	b.common.fields.AltitudeFeet = nil
	b.common.altitudeM = Ptr(metres)
	return b
}

// Comment sets free text after the structured parts. It may contain UTF-8.
func (b MicEBuilder) Comment(text string) MicEBuilder {
	b.common.fields.Comment = text
	return b
}

// PHG sets the power, antenna height, gain and directivity codes (PHGphgd, APRS12c ch. 7).
func (b MicEBuilder) PHG(phg PHG) MicEBuilder {
	if phg.BeaconsPerHour != nil {
		phg.BeaconsPerHour = Ptr(*phg.BeaconsPerHour)
	}
	b.common.fields.PHG = &phg
	return b
}

// RangeMiles sets the omnidirectional radio range in miles (RNGrrrr, APRS12c ch. 7).
func (b MicEBuilder) RangeMiles(miles float64) MicEBuilder {
	b.common.fields.RangeMiles = Ptr(miles)
	return b
}

// Frequency sets a voice frequency at the start of the comment, as radios with a tune button read
// it (145.725MHz, APRS12c ch. 18), rounded to the nearest kHz. Add Tone and OffsetKHz after it.
func (b MicEBuilder) Frequency(mhz float64) MicEBuilder {
	b.common.fields.Frequency = &VoiceFrequency{MHz: mhz}
	return b
}

// Tone sets the tone field for Frequency: a kind, ToneTone, ToneCTCSS or ToneDCS, and its value as
// sent, a whole number of Hz or a DCS code (ToneTone, 118 sends T118; 118.8 Hz is sent as 118).
// ToneOff and ToneBurst have no value, which is ignored.
func (b MicEBuilder) Tone(tone Tone, value uint16) MicEBuilder {
	b.common.tone = &toneSetting{tone: tone, value: value}
	return b
}

// OffsetKHz sets the transmit offset for Frequency, in kHz, such as -600; sent in tens of kHz.
func (b MicEBuilder) OffsetKHz(khz int32) MicEBuilder {
	b.common.offsetKHz = Ptr(khz)
	return b
}

// VoiceFrequency sets a voice frequency with every option (APRS12c ch. 18).
func (b MicEBuilder) VoiceFrequency(frequency VoiceFrequency) MicEBuilder {
	b.common.fields.Frequency = &frequency
	return b
}

// Compressed sends the position in compressed (base-91) form: shorter and more precise, but with
// no room for PHG or other data extensions (APRS12c ch. 9).
func (b MicEBuilder) Compressed() MicEBuilder {
	b.common.fields.Compressed = true
	return b
}

// Ambiguity blanks the last 1-4 digits of the position to hide it: to about 0.1, 1, 10 or 60
// nautical miles (APRS12c ch. 6). The digits are cut, so the report gives the box the position is
// in, and a decoder reads its centre. Not with Compressed or DAO.
func (b MicEBuilder) Ambiguity(digits uint8) MicEBuilder {
	b.common.ambiguity = digits
	return b
}

// DAO adds a !DAO! extension carrying extra position precision, in the WGS84 base-91 form (about
// 0.2 m).
func (b MicEBuilder) DAO() MicEBuilder {
	b.common.fields.DAO = &DAO{Datum: 'W', Precision: DAOBase91}
	return b
}

// Telemetry adds base-91 telemetry to the comment (|ss1122..|, APRS12c ch. 13): a sequence
// number, 1-5 analog values, each 0-8280, and optionally the 8 digital bits, which can only follow
// all 5 analog values.
func (b MicEBuilder) Telemetry(telemetry CommentTelemetry) MicEBuilder {
	telemetry.Analog = append([]uint16(nil), telemetry.Analog...)
	b.common.fields.Telemetry = &telemetry
	return b
}
