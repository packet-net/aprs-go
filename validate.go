package aprs

import (
	"reflect"
	"unicode/utf8"
)

// prepare readies data given to the encoder: a pointer to one of the data types is the data it
// points to (nil has nothing to encode), and data a program built by hand that no decoder could
// give, an enumeration value outside its type or text that is not UTF-8, is refused before
// anything is written.
func prepare(data Data) (Data, *EncodeError) {
	if data == nil {
		return nil, newEncodeError(RefusalNotEncodable, "there is no data to encode")
	}
	if v := reflect.ValueOf(data); v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, newEncodeError(RefusalNotEncodable, "a nil pointer has no data to encode")
		}
		d, ok := v.Elem().Interface().(Data)
		if !ok {
			return nil, newEncodeError(RefusalNotEncodable, "not one of the data types")
		}
		data = d
	}
	return data, nil
}

// validate refuses data no decoder could give: an enumeration value outside its type
// (value-out-of-range) or text that is not UTF-8 (invalid-character).
func validate(data Data) *EncodeError {
	if !enumsInRange(data) {
		return newEncodeError(RefusalValueOutOfRange, "an enumeration (a timestamp kind, compression type, area shape or colour, storm kind, !DAO! precision, tone, raw weather format or Mic-E message) is not one of its type's values")
	}
	if !textIsUTF8(reflect.ValueOf(data)) {
		return newEncodeError(RefusalInvalidCharacter, "text is not valid UTF-8")
	}
	return nil
}

var packetType = reflect.TypeOf((*Packet)(nil))

// textIsUTF8 says whether every string in v is valid UTF-8. A third-party packet's inner packet
// is written from its information field as received, so it is not looked at.
func textIsUTF8(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return utf8.ValidString(v.String())
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() || v.Type() == packetType {
			return true
		}
		return textIsUTF8(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !textIsUTF8(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return true
		}
		for i := 0; i < v.Len(); i++ {
			if !textIsUTF8(v.Index(i)) {
				return false
			}
		}
	}
	return true
}

func timestampKindOK(t *Timestamp) bool {
	return t == nil || t.Kind <= MonthDayHourMinute
}

func positionedEnumsOK(f *Positioned) bool {
	switch {
	case f.Compression != nil && (f.Compression.Fix > FixCurrent || f.Compression.Source > SourceRMC || f.Compression.Origin > OriginDigipeaterConversion):
		return false
	case f.Area != nil && (f.Area.Shape > AreaFilledBox || f.Area.Color > ColorGrayLow):
		return false
	case f.Storm != nil && f.Storm.Kind > TropicalDepression:
		return false
	case f.DAO != nil && f.DAO.Precision > DAOBase91:
		return false
	case f.Frequency != nil && f.Frequency.Tone != nil && *f.Frequency.Tone > ToneBurst:
		return false
	}
	return true
}

func enumsInRange(data Data) bool {
	switch d := data.(type) {
	case PositionReport:
		return timestampKindOK(d.Timestamp) && positionedEnumsOK(&d.Fields)
	case ObjectReport:
		return timestampKindOK(d.Timestamp) && positionedEnumsOK(&d.Fields)
	case ItemReport:
		return positionedEnumsOK(&d.Fields)
	case MicEReport:
		_, custom := d.MicEMessage.Custom()
		return (custom || (d.MicEMessage >= MicEOffDuty && d.MicEMessage <= MicEUnknown)) && positionedEnumsOK(&d.Fields)
	case Status:
		return timestampKindOK(d.Timestamp)
	case WeatherReport:
		return timestampKindOK(d.Timestamp)
	case RawWeather:
		return d.Format <= UltimeterLogging
	}
	return true
}
