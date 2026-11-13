package aprs

// TimestampDHM is day of month, hour and minute, UTC: DDHHMMz, the form most reports use.
func TimestampDHM(day, hour, minute uint8) Timestamp {
	return Timestamp{Kind: DayHourMinute, Day: day, Hour: hour, Minute: minute, UTC: true}
}

// TimestampDHMLocal is day of month, hour and minute in local time: DDHHMM/.
func TimestampDHMLocal(day, hour, minute uint8) Timestamp {
	return Timestamp{Kind: DayHourMinute, Day: day, Hour: hour, Minute: minute}
}

// TimestampHMS is hour, minute and second, UTC: HHMMSSh.
func TimestampHMS(hour, minute, second uint8) Timestamp {
	return Timestamp{Kind: HourMinuteSecond, Hour: hour, Minute: minute, Second: second}
}

// TimestampMDHM is month, day, hour and minute, UTC: MMDDHHMM, the form positionless weather
// reports use.
func TimestampMDHM(month, day, hour, minute uint8) Timestamp {
	return Timestamp{Kind: MonthDayHourMinute, Month: month, Day: day, Hour: hour, Minute: minute}
}

// ParseTimestamp reads a 7-character DHM (z or /) or HMS (h) timestamp, and fails if it is not
// that shape.
func ParseTimestamp(b []byte) (Timestamp, bool) {
	if len(b) != 7 || !allDigits(b[:6]) {
		return Timestamp{}, false
	}
	a, c, d := pair(b[0:2]), pair(b[2:4]), pair(b[4:6])
	switch b[6] {
	case 'z':
		return TimestampDHM(a, c, d), true
	case '/':
		return TimestampDHMLocal(a, c, d), true
	case 'h':
		return TimestampHMS(a, c, d), true
	}
	return Timestamp{}, false
}

// ParseTimestampMDHM reads an 8-digit month-day-hour-minute timestamp.
func ParseTimestampMDHM(b []byte) (Timestamp, bool) {
	if len(b) != 8 || !allDigits(b) {
		return Timestamp{}, false
	}
	return TimestampMDHM(pair(b[0:2]), pair(b[2:4]), pair(b[4:6]), pair(b[6:8])), true
}

// IsValid says whether the fields make sense: a day 1-31, an hour 0-23, a minute and second
// 0-59, a month 1-12.
func (t Timestamp) IsValid() bool {
	switch t.Kind {
	case DayHourMinute:
		return t.Day >= 1 && t.Day <= 31 && t.Hour < 24 && t.Minute < 60
	case HourMinuteSecond:
		return t.Hour < 24 && t.Minute < 60 && t.Second < 60
	default:
		return t.Month >= 1 && t.Month <= 12 && t.Day >= 1 && t.Day <= 31 && t.Hour < 24 && t.Minute < 60
	}
}

// IsPermanentObjectMarker says whether this is 111111z, which APRS12c ch. 18 uses to mark a
// permanent object.
func (t Timestamp) IsPermanentObjectMarker() bool {
	return t.Kind == DayHourMinute && t.UTC && t.Day == 11 && t.Hour == 11 && t.Minute == 11
}

// String gives the timestamp as sent on air: 092345z, 092345/, 234517h or 10090556.
func (t Timestamp) String() string {
	return string(t.bytes())
}

func (t Timestamp) bytes() []byte {
	out := make([]byte, 0, 8)
	two := func(n uint8) {
		out = append(out, '0'+n/10%10, '0'+n%10)
	}
	switch t.Kind {
	case DayHourMinute:
		two(t.Day)
		two(t.Hour)
		two(t.Minute)
		if t.UTC {
			out = append(out, 'z')
		} else {
			out = append(out, '/')
		}
	case HourMinuteSecond:
		two(t.Hour)
		two(t.Minute)
		two(t.Second)
		out = append(out, 'h')
	default:
		two(t.Month)
		two(t.Day)
		two(t.Hour)
		two(t.Minute)
	}
	return out
}

func pair(b []byte) uint8 {
	return (b[0]-'0')*10 + (b[1] - '0')
}

// sameTimestamp compares two optional timestamps by the fields their kind uses.
func sameTimestamp(a, b *Timestamp) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case DayHourMinute:
		return a.Day == b.Day && a.Hour == b.Hour && a.Minute == b.Minute && a.UTC == b.UTC
	case HourMinuteSecond:
		return a.Hour == b.Hour && a.Minute == b.Minute && a.Second == b.Second
	default:
		return a.Month == b.Month && a.Day == b.Day && a.Hour == b.Hour && a.Minute == b.Minute
	}
}
