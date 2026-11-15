package aprs_test

import (
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// BenchmarkEncodeMixed encodes seven common reports in turn: the cost of an encode with its
// read-back (go test -bench EncodeMixed -benchmem).
func BenchmarkEncodeMixed(b *testing.B) {
	at := aprs.Position{Latitude: 51.5, Longitude: -0.5}
	sym := aprs.NewSymbol('/', '>')
	reports := []aprs.Data{
		aprs.PositionReport{Messaging: true, Fields: aprs.Positioned{Position: at, Symbol: sym, CourseDegrees: aprs.Ptr(88.0), SpeedKnots: aprs.Ptr(36.0), Comment: "Mobile"}},
		aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: sym, Compressed: true, CourseDegrees: aprs.Ptr(88.0), SpeedKnots: aprs.Ptr(36.0), Comment: "Compressed"}},
		aprs.ObjectReport{Name: "OBJECT", Timestamp: &aprs.Timestamp{Kind: aprs.DayHourMinute, UTC: true, Day: 1, Hour: 2, Minute: 3}, Fields: aprs.Positioned{Position: at, Symbol: sym}},
		aprs.Message{Addressee: "N0CALL", Text: "Hello there", MessageID: aprs.Ptr("42")},
		aprs.Status{Text: "On air and listening"},
		aprs.Telemetry{Sequence: "001", Analog: []*float64{aprs.Ptr(1.0), aprs.Ptr(2.0), aprs.Ptr(3.0), aprs.Ptr(4.0), aprs.Ptr(5.0)}, Bits: aprs.Ptr("00000000")},
		aprs.PositionReport{Fields: aprs.Positioned{Position: at, Symbol: aprs.NewSymbol('/', '_'), Weather: &aprs.Weather{WindDirectionDegrees: aprs.Ptr(90.0), WindSpeedMph: aprs.Ptr(5.0), TemperatureF: aprs.Ptr(60.0)}}},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, r := range reports {
			if _, err := aprs.Encode(r); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkDecodeMixed decodes seven common lines in turn, leniently.
func BenchmarkDecodeMixed(b *testing.B) {
	lines := [][]byte{
		[]byte("N0CALL-9>APZ001,WIDE1-1,qAR,M0LTE-10:=5130.00N/00007.00W>088/036Mobile"),
		[]byte("N0CALL-9>APZ001:!/5L!!<*e7>7P[Compressed"),
		[]byte("N0CALL-9>APZ001:;OBJECT   *010203z5130.00N/00030.00W>"),
		[]byte("N0CALL-9>APZ001::N0CALL   :Hello there{42"),
		[]byte("N0CALL-9>APZ001:>On air and listening"),
		[]byte("N0CALL-9>APZ001:T#001,001,002,003,004,005,00000000"),
		[]byte("N0CALL-9>S32UVT:`(_fn\"Oj/`\"4e}Mic-E comment"),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, l := range lines {
			if _, err := aprs.DecodeTNC2(l, aprs.Lenient); err != nil {
				b.Fatal(err)
			}
		}
	}
}
