package aprs_test

import (
	"errors"
	"strings"
	"testing"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
)

// The read-back (vectors README, Rules, Read-back): every public call that writes bytes ends in it.
// These tests show it for every report type, in two ways: a writer that writes a wrong byte is
// caught (the test seam makes every writer's bytes wrong in one place), and data whose naive bytes
// read back as something else gets an equivalent form or a refusal.

var (
	rbSource = aprs.MustAddress("N0CALL")
	rbDest   = aprs.MustAddress("APZ001")
)

func rbBits(s string) *string { return &s }

// rbSamples is one clean sample of every report type this library writes.
func rbSamples() map[string]aprs.Data {
	car := aprs.NewSymbol('/', '>')
	at := aprs.Position{Latitude: 51.5, Longitude: -0.5}
	timestamp := aprs.TimestampDHM(9, 23, 45)
	analog := []*float64{aprs.Ptr(1.0), aprs.Ptr(2.0), nil, aprs.Ptr(4.5), aprs.Ptr(5.0)}
	inner, err := aprs.NewPacket(aprs.MustAddress("G4ABC"), aprs.MustAddress("APRS"), nil, aprs.Status{Text: "hello"})
	if err != nil {
		panic(err)
	}
	locator := "IO91SX"
	return map[string]aprs.Data{
		"position":              aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "hello"}},
		"position with weather": aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: aprs.NewSymbol('/', '_'), Weather: &aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0), TemperatureF: aprs.Ptr(70.0)}}},
		"object":                aprs.ObjectReport{Name: "OBJ", Timestamp: &timestamp, Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "hello"}},
		"item":                  aprs.ItemReport{Name: "ITEM", Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "hello"}},
		"mic-e":                 aprs.MicEReport{MicEMessage: aprs.MicEEnRoute, Fields: aprs.Positioned{Position: at, Symbol: car, SpeedKnots: aprs.Ptr(10.0), Comment: "hello"}},
		"message":               aprs.Message{Addressee: "N0CALL", Text: "hi", MessageID: aprs.Ptr("12")},
		"ack":                   aprs.Ack{Addressee: "N0CALL", AckedID: "12"},
		"reject":                aprs.Reject{Addressee: "N0CALL", RejectedID: "12"},
		"bulletin":              aprs.Bulletin{Addressee: "BLN1", Text: "hello"},
		"nws bulletin":          aprs.NWSBulletin{Addressee: "NWS-WARN", Text: "hello"},
		"telemetry names":       aprs.TelemetryNames{Addressee: "N0CALL", Names: []string{"Battery", "Temp"}},
		"telemetry units":       aprs.TelemetryUnits{Addressee: "N0CALL", Units: []string{"V", "C"}},
		"telemetry coefficient": aprs.TelemetryCoefficients{Addressee: "N0CALL", Coefficients: []float64{0, 0.5, 0}},
		"telemetry bits":        aprs.TelemetryBits{Addressee: "N0CALL", Bits: "10101010", Project: "Garden"},
		"directed query":        aprs.DirectedQuery{Addressee: "N0CALL", QueryType: "APRSP"},
		"status":                aprs.Status{Text: "hello"},
		"status with locator":   aprs.Status{Locator: &locator, Symbol: &car, Text: "hello"},
		"telemetry":             aprs.Telemetry{Sequence: "123", Analog: analog, Bits: rbBits("10101010"), Comment: "x"},
		"positionless weather":  aprs.WeatherReport{Timestamp: aprs.Ptr(aprs.TimestampMDHM(10, 9, 23, 45)), Weather: aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0), TemperatureF: aprs.Ptr(70.0)}},
		"raw weather":           aprs.RawWeather{Format: aprs.PeetBrosHash, Data: "1234"},
		"nmea":                  aprs.NMEA{Sentence: "GPGLL,4916.45,N,12311.12,W,225444,A"},
		"maidenhead":            aprs.MaidenheadBeacon{Locator: "IO91SX", Comment: "hello"},
		"query":                 aprs.Query{QueryType: "APRS", Footprint: &aprs.Footprint{Latitude: 34, Longitude: -117, RadiusMiles: 200}},
		"capabilities":          aprs.Capabilities{Capabilities: []aprs.Capability{{Token: "IGATE"}, {Token: "MSG_CNT", Value: aprs.Ptr("30")}}},
		"third-party":           aprs.ThirdParty{Packet: inner},
		"user-defined":          aprs.UserDefined{UserID: 'A', PacketType: '1', Data: []byte("data")},
		"test":                  aprs.TestData{Data: "hello"},
		"agrelo":                aprs.AgreloDF{BearingDegrees: 123, Quality: 4},
	}
}

func rbWrite(data aprs.Data) ([]byte, error) {
	if _, ok := data.(aprs.MicEReport); ok {
		p, err := aprs.NewPacket(rbSource, rbDest, nil, data)
		if err != nil {
			return nil, err
		}
		return p.Information, nil
	}
	return aprs.Encode(data)
}

// A writer that writes a wrong byte, whatever the report, is refused rather than returned: the
// public call ends in the read-back, for every report type.
func TestEveryReportTypeEndsInTheReadBack(t *testing.T) {
	samples := rbSamples()
	if len(samples) < 27 {
		t.Fatalf("a sample for each report type is needed, have %d", len(samples))
	}
	// Wrong in one place: the last letter or digit written is a different one.
	wrong := func(b []byte) []byte {
		for i := len(b) - 1; i >= 0; i-- {
			if c := b[i]; c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
				b[i] = c ^ 1
				break
			}
		}
		return b
	}
	for name, data := range samples {
		t.Run(name, func(t *testing.T) {
			clean, err := rbWrite(data)
			if err != nil {
				t.Fatalf("the clean sample is refused: %v", err)
			}
			restore := aprs.SetWriterFault(wrong)
			defer restore()
			bytes, err := rbWrite(data)
			if err == nil {
				t.Fatalf("wrong bytes were returned: %q (clean %q)", bytes, clean)
			}
			var e *aprs.EncodeError
			if !errors.As(err, &e) {
				t.Fatalf("expected an *EncodeError, got %v", err)
			}
			switch e.Code() {
			case aprs.RefusalDoesNotReadBack, aprs.RefusalTextReadsAsField, aprs.RefusalInvalidAddressee:
			default:
				t.Errorf("refused with %s (%v), which is not a read-back refusal", e.Code().ID(), err)
			}
			if bytes != nil {
				t.Errorf("bytes came back with the refusal: %q", bytes)
			}
		})
	}
}

// A writer that is wrong in a Mic-E report's position bytes, or a number's digits, is caught too.
func TestAWrongDigitIsCaughtInEveryNumberForm(t *testing.T) {
	car := aprs.NewSymbol('/', '>')
	at := aprs.Position{Latitude: 51.5, Longitude: -0.5}
	forms := map[string]aprs.Data{
		"uncompressed position": aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, CourseDegrees: aprs.Ptr(120.0), SpeedKnots: aprs.Ptr(30.0), AltitudeFeet: aprs.Ptr(1000.0)}},
		"compressed position":   aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, Compressed: true, CourseDegrees: aprs.Ptr(120.0), SpeedKnots: aprs.Ptr(30.0)}},
		"dao":                   aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, DAO: &aprs.DAO{Datum: 'W', Precision: aprs.DAOThousandths}}},
		"ambiguity":             aprs.PositionReport{Fields: aprs.Positioned{Position: aprs.Position{Latitude: 51.5, Longitude: -0.5, Ambiguity: 2}, Symbol: car}},
		"frequency":             aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, Frequency: &aprs.VoiceFrequency{MHz: 146.52}}},
		"range":                 aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, RangeMiles: aprs.Ptr(50.0)}},
		"compressed weather":    aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: aprs.NewSymbol('/', '_'), Compressed: true, Weather: &aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0), TemperatureF: aprs.Ptr(70.0)}}},
		"snowfall":              aprs.WeatherReport{Timestamp: aprs.Ptr(aprs.TimestampMDHM(10, 9, 23, 45)), Weather: aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0), Snow24hIn: aprs.Ptr(1.5)}},
	}
	for name, data := range forms {
		t.Run(name, func(t *testing.T) {
			if _, err := rbWrite(data); err != nil {
				t.Fatalf("the clean sample is refused: %v", err)
			}
			// Every digit in turn, in the bytes written, is wrong once: each is caught.
			clean, _ := rbWrite(data)
			digits := 0
			for i := range clean {
				if clean[i] < '0' || clean[i] > '9' {
					continue
				}
				digits++
				at := i
				restore := aprs.SetWriterFault(func(b []byte) []byte {
					if at < len(b) {
						b[at] = '0' + (b[at]-'0'+1)%10
					}
					return b
				})
				got, err := rbWrite(data)
				restore()
				if err == nil {
					t.Errorf("a wrong digit at %d was written: %q for %q", i, got, clean)
				}
			}
			if digits == 0 {
				t.Fatalf("%q has no digit to be wrong", clean)
			}
		})
	}
}

// Data whose naive bytes read back as something else is written in an equivalent form, or
// refused, never written as it would misread.
func TestNaiveBytesThatReadBackAsSomethingElseAreNotWritten(t *testing.T) {
	car := aprs.NewSymbol('/', '>')
	at := aprs.Position{Latitude: 51.5, Longitude: -0.5}
	timestamp := aprs.TimestampDHM(9, 23, 45)
	locator := "IO91SX"
	tests := []struct {
		name string
		data aprs.Data
	}{
		{"position comment that is a !DAO!", aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "!W12!"}}},
		{"position comment that is an altitude", aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "/A=000100"}}},
		{"object comment that is a !DAO!", aprs.ObjectReport{Name: "OBJ", Timestamp: &timestamp, Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "!W12!"}}},
		{"item comment that is a !DAO!", aprs.ItemReport{Name: "ITEM", Fields: aprs.Positioned{Position: at, Symbol: car, Comment: "!W12!"}}},
		{"mic-e comment that is an altitude", aprs.MicEReport{TypeCode: aprs.Ptr('`'), Fields: aprs.Positioned{Position: at, Symbol: car, SpeedKnots: aprs.Ptr(10.0), Comment: "ack1}AB"}}},
		{"message that is an ack", aprs.Message{Addressee: "N0CALL", Text: "ack1"}},
		{"bulletin that is an ack", aprs.Bulletin{Addressee: "BLN1", Text: "ack1"}},
		{"nws bulletin that is an ack", aprs.NWSBulletin{Addressee: "NWS-WARN", Text: "ack1"}},
		{"status that is a timestamp", aprs.Status{Text: "092345z"}},
		{"status that is a locator", aprs.Status{Text: "IO91SX/> hello"}},
		{"status after a locator", aprs.Status{Locator: &locator, Symbol: &car, Text: "092345z"}},
		{"third-party status that is a timestamp", aprs.ThirdParty{Packet: innerAsReceived(">092345z", aprs.Status{Text: "092345z"})}},
		{"positionless weather with a software type that is a field", aprs.WeatherReport{Timestamp: aprs.Ptr(aprs.TimestampMDHM(10, 9, 23, 45)), Weather: aprs.Weather{TemperatureF: aprs.Ptr(70.0), Software: aprs.Ptr('r'), Unit: aprs.Ptr("001p")}}},
		{"positioned weather with a software type that is a field", aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: aprs.NewSymbol('/', '_'), Weather: &aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0), Software: aprs.Ptr('A'), Unit: aprs.Ptr("A00")}}}},
		{"ack to a bulletin address", aprs.Ack{Addressee: "BLN1", AckedID: "5"}},
		{"reject to a bulletin address", aprs.Reject{Addressee: "BLN1", RejectedID: "5"}},
		{"names to a bulletin address", aprs.TelemetryNames{Addressee: "BLN1", Names: []string{"a"}}},
		{"units to a bulletin address", aprs.TelemetryUnits{Addressee: "BLN1", Units: []string{"a"}}},
		{"coefficients to a bulletin address", aprs.TelemetryCoefficients{Addressee: "BLN1", Coefficients: []float64{1, 2, 3}}},
		{"bits to a bulletin address", aprs.TelemetryBits{Addressee: "BLN1", Bits: "10101010", Project: "x"}},
		{"query to a bulletin address", aprs.DirectedQuery{Addressee: "BLN1", QueryType: "APRSP"}},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			info, err := rbWrite(c.data)
			if err != nil {
				var e *aprs.EncodeError
				if !errors.As(err, &e) {
					t.Fatal(err)
				}
				switch e.Code() {
				case aprs.RefusalTextReadsAsField, aprs.RefusalInvalidAddressee, aprs.RefusalDoesNotReadBack:
					return
				}
				t.Fatalf("refused for another reason: %v (%s)", err, e.Code().ID())
			}
			// Written: the bytes decode, leniently and cleanly, to the data given.
			destination := rbDest
			if _, ok := c.data.(aprs.MicEReport); ok {
				p, err := aprs.NewPacket(rbSource, rbDest, nil, c.data)
				if err != nil {
					t.Fatal(err)
				}
				destination = p.Destination
			}
			back := aprs.Decode(rbSource, destination, nil, info, aprs.Lenient)
			if back.HasWarnings() || back.HasErrors() {
				t.Errorf("%q decodes with %v", info, back.Diagnostics)
			}
			var diff []string
			neutral.Differences(any(neutral.Data(c.data)), any(neutral.Data(back.Data)), "", &diff)
			if len(diff) > 0 {
				t.Errorf("%q reads back as something else: %v", info, diff)
			}
		})
	}
}

// innerAsReceived is a third-party packet's inner packet whose information field is as received and
// whose data is what a program says it is.
func innerAsReceived(information string, data aprs.Data) *aprs.Packet {
	p := aprs.Decode(aprs.MustAddress("G4ABC"), aprs.MustAddress("APRS"), nil, []byte(information), aprs.Lenient)
	p.Data = data
	p.ThirdParty = true
	return p
}

func mustInner(data aprs.Data) *aprs.Packet {
	p, err := aprs.NewPacket(aprs.MustAddress("G4ABC"), aprs.MustAddress("APRS"), nil, data)
	if err != nil {
		panic(err)
	}
	return p
}

// The refusals the read-back names: free text that reads back changed is text-reads-as-field,
// where a rule names the cause it is that rule's code, and a software type without its unit (or a
// unit without its type) is missing-field.
func TestReadBackRefusalCodes(t *testing.T) {
	at := aprs.Position{Latitude: 51.5, Longitude: -0.5}
	wx := func(f func(w *aprs.Weather)) aprs.Data {
		w := aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0)}
		f(&w)
		return aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: aprs.NewSymbol('/', '_'), Weather: &w}}
	}
	storm := aprs.ObjectReport{Name: "STORM", Timestamp: aprs.Ptr(aprs.TimestampDHM(9, 23, 45)), Fields: aprs.Positioned{
		Position: at, Symbol: aprs.NewSymbol('\\', '@'), Comment: "%123/4",
		Storm: &aprs.Storm{Kind: aprs.Hurricane, SustainedWindKnots: aprs.Ptr(uint16(30)), GustKnots: aprs.Ptr(uint16(40)), CentralPressureMbar: aprs.Ptr(uint16(990)), HurricaneRadiusNM: aprs.Ptr(uint16(1)), TropicalStormRadiusNM: aprs.Ptr(uint16(2))},
	}}
	tests := []struct {
		name string
		data aprs.Data
		want aprs.RefusalCode
	}{
		{"extra that reads as a rain counter", wx(func(w *aprs.Weather) { w.Extra = []aprs.WeatherField{{Letter: '#', Value: "123"}} }), aprs.RefusalTextReadsAsField},
		{"extra that reads as software and unit", wx(func(w *aprs.Weather) { w.Extra = []aprs.WeatherField{{Letter: 'A', Value: "A00"}} }), aprs.RefusalTextReadsAsField},
		{"software that is not a letter", wx(func(w *aprs.Weather) { w.Software, w.Unit = aprs.Ptr('1'), aprs.Ptr("Dvs") }), aprs.RefusalTextReadsAsField},
		{"unit of one character", wx(func(w *aprs.Weather) { w.Software, w.Unit = aprs.Ptr('e'), aprs.Ptr("D") }), aprs.RefusalTextReadsAsField},
		{"software without a unit", wx(func(w *aprs.Weather) { w.Software = aprs.Ptr('e') }), aprs.RefusalMissingField},
		{"unit without a software type", wx(func(w *aprs.Weather) { w.Unit = aprs.Ptr("Dvs") }), aprs.RefusalMissingField},
		{"message that is an ack", aprs.Message{Addressee: "N0CALL", Text: "ack1"}, aprs.RefusalTextReadsAsField},
		{"empty message ID is no message ID", aprs.Message{Addressee: "N0CALL", Text: "hi", MessageID: aprs.Ptr("")}, 0},
		{"comment after storm data that reads as a whole gale radius", storm, 0},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			info, err := rbWrite(c.data)
			if c.want == 0 {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				back := aprs.Decode(rbSource, rbDest, nil, info, aprs.Lenient)
				var diff []string
				neutral.Differences(any(neutral.Data(c.data)), any(neutral.Data(back.Data)), "", &diff)
				if len(diff) > 0 && !strings.Contains(c.name, "empty") {
					t.Errorf("%q reads back as something else: %v", info, diff)
				}
				return
			}
			if err == nil {
				t.Fatalf("written: %q", info)
			}
			eq(t, refusal(t, err), c.want)
		})
	}
}

// An empty message ID or directed query target is none: it is written as none.
func TestEmptyMessageIDAndTargetAreNone(t *testing.T) {
	b, err := aprs.Encode(aprs.Message{Addressee: "N0CALL", Text: "hi", MessageID: aprs.Ptr("")})
	if err != nil || string(b) != ":N0CALL   :hi" {
		t.Errorf("message: %q, %v", b, err)
	}
	b, err = aprs.Encode(aprs.DirectedQuery{Addressee: "N0CALL", QueryType: "APRSP", Target: aprs.Ptr("")})
	if err != nil || string(b) != ":N0CALL   :?APRSP" {
		t.Errorf("query: %q, %v", b, err)
	}
	// A reply-ack still needs a message ID, and its empty value is a value.
	_, err = aprs.Encode(aprs.Message{Addressee: "N0CALL", Text: "hi", MessageID: aprs.Ptr(""), ReplyAck: aprs.Ptr("")})
	eq(t, refusal(t, err), aprs.RefusalMissingField)
	b, err = aprs.Encode(aprs.Message{Addressee: "N0CALL", Text: "hi", MessageID: aprs.Ptr("1"), ReplyAck: aprs.Ptr("")})
	if err != nil || string(b) != ":N0CALL   :hi{1}" {
		t.Errorf("reply-ack: %q, %v", b, err)
	}
	// A builder refuses an empty message ID, as it refuses any out of range.
	st := me(t)
	_, err = st.Message("N0CALL", "hi").MessageID("").Build()
	eq(t, refusal(t, err), aprs.RefusalInvalidMessageID)
}

// The header written is read back too: a TNC2 line writes * on the last used entry only, an
// AX.25 frame sets the H bit of every used entry, and a path with an entry not marked used before
// one that is cannot be carried by either, as it would read back used.
func TestTheHeaderWrittenIsReadBack(t *testing.T) {
	st := func(path ...string) aprs.Station {
		s, err := me(t).Via(path...)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	build := func(s aprs.Station) *aprs.Packet {
		p, err := s.Status("hi").Build()
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	two := build(st("WIDE1-1*", "N0CALL-1*", "WIDE2-1"))
	eq(t, string(two.TNC2()), "M0LTE-11>APZ001,WIDE1-1,N0CALL-1*,WIDE2-1:>hi")
	frame, err := two.AX25()
	if err != nil {
		t.Fatal(err)
	}
	heard, err := aprs.DecodeAX25(frame, aprs.Lenient)
	if err != nil || len(heard.Diagnostics) != 0 || !heard.Path[0].Used || !heard.Path[1].Used || heard.Path[2].Used {
		t.Errorf("frame: %v %v %v", heard, err, heard.Diagnostics)
	}
	for _, path := range [][]string{{"WIDE1-1", "N0CALL-1*"}, {"N0CALL-1", "WIDE1-1", "WIDE2-1*"}} {
		bad := build(st(path...))
		if line, err := bad.TNC2Line(); err == nil || refusal(t, err) != aprs.RefusalTNC2CannotCarry {
			t.Errorf("%v as a line: %q, %v", path, line, err)
		}
		if bad.TNC2() != nil {
			t.Errorf("%v: TNC2 gave bytes", path)
		}
		if f, err := bad.AX25(); err == nil || refusal(t, err) != aprs.RefusalAX25CannotCarry {
			t.Errorf("%v as a frame: %x, %v", path, f, err)
		}
		if _, err := bad.KISS(0); err == nil || refusal(t, err) != aprs.RefusalAX25CannotCarry {
			t.Errorf("%v as KISS: %v", path, err)
		}
	}
	eq(t, string(build(st("N0CALL-1*", "WIDE2-1")).TNC2()), "M0LTE-11>APZ001,N0CALL-1*,WIDE2-1:>hi")
	eq(t, string(build(st("WIDE1-1", "WIDE2-1")).TNC2()), "M0LTE-11>APZ001,WIDE1-1,WIDE2-1:>hi")
	// A decoded packet's header is read back as well, with no diagnostic the packet does not carry:
	// an empty destination the packet holds is written empty, and a hand-built one is refused.
	p, err := aprs.DecodeTNC2([]byte("N0CALL>:>hi"), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, string(p.TNC2()), "N0CALL>:>hi")
	held := *p
	held.Diagnostics = nil
	if _, err := held.TNC2Line(); err == nil || refusal(t, err) != aprs.RefusalInvalidAddress {
		t.Errorf("an empty destination the packet does not carry: %v", err)
	}
	// A path entry marked used before the one the line marks is used too, so a decoded packet
	// whose line marked two entries writes one mark and reads back the same.
	p, err = aprs.DecodeTNC2([]byte("N0CALL>APZ001,WIDE1-1*,WIDE2-1*:>hi"), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, string(p.TNC2()), "N0CALL>APZ001,WIDE1-1,WIDE2-1*:>hi")
}

// A third-party packet is read back whole: its inner packet's data and its own warnings.
func TestThirdPartyIsReadBackWhole(t *testing.T) {
	inner := mustInner(aprs.Status{Text: "hello"})
	b, err := aprs.Encode(aprs.ThirdParty{Packet: inner})
	if err != nil || string(b) != "}G4ABC>APRS:>hello" {
		t.Fatalf("%q, %v", b, err)
	}
	// An inner packet whose data is not what its information field says does not read back.
	wrong := *inner
	wrong.Data = aprs.Status{Text: "goodbye"}
	_, err = aprs.Encode(aprs.ThirdParty{Packet: &wrong})
	if c := refusal(t, err); c != aprs.RefusalTextReadsAsField && c != aprs.RefusalDoesNotReadBack {
		t.Errorf("refused with %s", c.ID())
	}
	// Nor one whose information field decodes with a warning the data does not give.
	warned := *inner
	warned.Information = []byte(">hello\x01")
	_, err = aprs.Encode(aprs.ThirdParty{Packet: &warned})
	if err == nil {
		t.Errorf("an inner packet that decodes with a warning the data lacks was written")
	}
}
