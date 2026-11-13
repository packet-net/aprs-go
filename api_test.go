package aprs_test

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
)

func me(t *testing.T) aprs.Station {
	t.Helper()
	s, err := aprs.NewStation("M0LTE-11")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// line gives a built packet's TNC2 line, failing the test on a refusal: line(t)(b.Build()).
func line(t *testing.T) func(*aprs.Packet, error) string {
	return func(p *aprs.Packet, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		return p.String()
	}
}

func info(t *testing.T, d aprs.Data) string {
	t.Helper()
	b, err := aprs.Encode(d)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	return string(b)
}

func refusal(t *testing.T, err error) aprs.RefusalCode {
	t.Helper()
	var e *aprs.EncodeError
	if !errors.As(err, &e) {
		t.Fatalf("expected an *EncodeError, got %v", err)
	}
	return e.Code()
}

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMetadataCanDescribeAnotherStation(t *testing.T) {
	eq(t, line(t)(me(t).TelemetryNames("Battery").Addressee("G3NRW-5").Build()), "M0LTE-11>APZ001::G3NRW-5  :PARM.Battery")
	eq(t, line(t)(me(t).TelemetryUnits("V").Addressee("G3NRW-5").Build()), "M0LTE-11>APZ001::G3NRW-5  :UNIT.V")
	eq(t, line(t)(me(t).TelemetryCoefficients(0, 0.5, 0).Addressee("G3NRW-5").Build()), "M0LTE-11>APZ001::G3NRW-5  :EQNS.0,0.5,0")
	eq(t, line(t)(me(t).TelemetryBits("10101010", "Garden").Addressee("G3NRW-5").Build()), "M0LTE-11>APZ001::G3NRW-5  :BITS.10101010,Garden")
	// By default it is the station's own.
	eq(t, line(t)(me(t).TelemetryNames("Battery").Build()), "M0LTE-11>APZ001::M0LTE-11 :PARM.Battery")
	// The digital bits are text, and refused when they are not eight 0 or 1 characters.
	_, err := me(t).TelemetryBits("1010", "Garden").Build()
	eq(t, refusal(t, err), aprs.RefusalMalformed)
	_, err = me(t).Telemetry(1).Bits("2").Build()
	eq(t, refusal(t, err), aprs.RefusalMalformed)
}

func TestAToneHasAKindAndAValueAsSent(t *testing.T) {
	tail := func(tone aprs.Tone, value uint16) string {
		l := line(t)(me(t).Position(51.45, -0.98).Symbol(aprs.SymbolRepeater).Frequency(145.725).Tone(tone, value).Build())
		_, after, _ := strings.Cut(l, "MHz")
		return after
	}
	eq(t, tail(aprs.ToneTone, 118), " T118")
	eq(t, tail(aprs.ToneCTCSS, 100), " C100")
	eq(t, tail(aprs.ToneDCS, 23), " D023")
	eq(t, tail(aprs.ToneOff, 0), " Toff")
	// A frequency is rounded to its step, not refused for extra decimals.
	l := line(t)(me(t).Position(51.45, -0.98).Symbol(aprs.SymbolRepeater).Frequency(145.7251).Build())
	if !strings.HasSuffix(l, "145.725MHz") {
		t.Error(l)
	}
}

func TestMicEIsMadeByNewPacketAndNeverLosesItsDestination(t *testing.T) {
	micE, err := me(t).MicE(42.179, -71.1985).Symbol(aprs.SymbolCar).CourseDegrees(215).SpeedKnots(9).Data()
	if err != nil {
		t.Fatal(err)
	}
	// The destination passed in is not used; the one the report computes is.
	p, err := aprs.NewPacket(aprs.MustAddress("M0LTE"), aprs.MustAddress("APZ001"), nil, micE)
	if err != nil {
		t.Fatal(err)
	}
	if p.Destination.String() == "APZ001" {
		t.Error("the destination was not computed")
	}
	// Encoding the data alone would drop half the position, so it is refused.
	_, err = aprs.Encode(micE)
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
}

func TestAMicEAltitudeInTheCommentReEncodesWhereItWas(t *testing.T) {
	built := func(altitude func(aprs.MicEBuilder) aprs.MicEBuilder) *aprs.Packet {
		p, err := altitude(me(t).MicE(42.179, -71.1985).Symbol(aprs.SymbolCar)).Build()
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// The Mic-E bytes: 9176 metres. The /A= : 100 feet, which no whole metre says.
	for _, c := range []struct {
		packet    *aprs.Packet
		inComment bool
	}{
		{built(func(b aprs.MicEBuilder) aprs.MicEBuilder { return b.AltitudeM(9176) }), false},
		{built(func(b aprs.MicEBuilder) aprs.MicEBuilder { return b.AltitudeFeet(100) }), true},
	} {
		heard, err := aprs.DecodeTNC2(c.packet.TNC2(), aprs.Lenient)
		if err != nil {
			t.Fatal(err)
		}
		report := heard.Data.(aprs.MicEReport)
		eq(t, report.AltitudeInComment, c.inComment)
		again, err := aprs.NewPacket(heard.Source, heard.Destination, heard.Path, heard.Data)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, string(again.Information), string(c.packet.Information))
		eq(t, again.Destination, c.packet.Destination)
	}
	// Feet that are exactly whole metres still go where the data says.
	inText := built(func(b aprs.MicEBuilder) aprs.MicEBuilder { return b.AltitudeFeet(30 / 0.3048) })
	if !strings.Contains(string(inText.Information), "/A=") {
		t.Error(string(inText.Information))
	}
	inBytes := built(func(b aprs.MicEBuilder) aprs.MicEBuilder { return b.AltitudeM(30) })
	if strings.Contains(string(inBytes.Information), "/A=") {
		t.Error(string(inBytes.Information))
	}
}

func TestNumbersTheEncoderRoundsAreFractionalAndRoundedOnce(t *testing.T) {
	position := func(course float64) string {
		return info(t, aprs.PositionReport{Fields: aprs.Positioned{
			Position: aprs.Position{Latitude: 51.5, Longitude: -0.1}, Symbol: aprs.SymbolCar,
			CourseDegrees: aprs.Ptr(course), SpeedKnots: aprs.Ptr(1.0),
		}})
	}
	if s := position(88.6); !strings.HasSuffix(s, ">089/001") {
		t.Error(s)
	}
	if s := position(88.4); !strings.HasSuffix(s, ">088/001") {
		t.Error(s)
	}
	w := aprs.Weather{
		WindDirectionDegrees: aprs.Ptr(89.5), WindSpeedMph: aprs.Ptr(1.0), WindGustMph: aprs.Ptr(2.0),
		TemperatureF: aprs.Ptr(70.0), HumidityPercent: aprs.Ptr(99.7), LuminosityWM2: aprs.Ptr(1412.4),
	}
	ts := aprs.TimestampMDHM(1, 2, 3, 4)
	eq(t, info(t, aprs.WeatherReport{Timestamp: &ts, Weather: w}), "_01020304c090s001g002t070h00l412")
	w.HumidityPercent = aprs.Ptr(0.4)
	_, err := aprs.Encode(aprs.WeatherReport{Timestamp: &ts, Weather: w})
	eq(t, refusal(t, err), aprs.RefusalValueHasNoForm)
	// A footprint's radius.
	fp := aprs.NewFootprint(34.02, -117.15, 200.4)
	eq(t, info(t, aprs.Query{QueryType: "APRS", Footprint: &fp}), "?APRS? 34.02,-117.15,0200")
	fp = aprs.NewFootprint(34, 1, 10000)
	_, err = aprs.Encode(aprs.Query{QueryType: "APRS", Footprint: &fp})
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
}

func TestNumbersKeptAsSentAreWrittenBackWhileTheyStillSayTheNumber(t *testing.T) {
	heard, _ := aprs.DecodeTNC2([]byte("M0LTE>APZ001:T#005,073,190.0,.5,,-0.50,00000000"), aprs.Lenient)
	tel := heard.Data.(aprs.Telemetry)
	if !reflect.DeepEqual(tel.AnalogText, []string{"073", "190.0", ".5", "", "-0.50"}) || tel.Analog[3] != nil || *tel.Analog[4] != -0.5 {
		t.Errorf("%q %v", tel.AnalogText, tel.Analog)
	}
	eq(t, info(t, heard.Data), "T#005,073,190.0,.5,,-0.50,00000000")
	// Without text the number is written as APRS12c does: three digits for 0-999, else the
	// shortest decimal.
	numbers := aprs.Telemetry{Sequence: "005", Analog: []*float64{aprs.Ptr(5.0), aprs.Ptr(190.0), aprs.Ptr(0.5), nil, aprs.Ptr(-94.0)}, Bits: aprs.Ptr("00000000")}
	eq(t, info(t, numbers), "T#005,005,190,0.5,,-94,00000000")
	// Text that no longer says the number is ignored.
	numbers.AnalogText = []string{"073", "190.0", "9", "", "-94"}
	eq(t, info(t, numbers), "T#005,005,190.0,0.5,,-94,00000000")

	coefficients := aprs.TelemetryCoefficients{Addressee: "M0LTE", Coefficients: []float64{0, 0.53, 1e-7}, CoefficientsText: []string{"0", ".53", "1e-7"}}
	eq(t, info(t, coefficients), ":M0LTE    :EQNS.0,.53,1e-7")
	coefficients.CoefficientsText = nil
	eq(t, info(t, coefficients), ":M0LTE    :EQNS.0,0.53,0.0000001")

	heard, _ = aprs.DecodeTNC2([]byte("M0LTE>APZ001:?APRS? 34.0,-.1715,0200"), aprs.Lenient)
	f := heard.Data.(aprs.Query).Footprint
	eq(t, f.Latitude, 34.0)
	eq(t, f.Longitude, -0.1715)
	eq(t, f.RadiusMiles, 200.0)
	eq(t, info(t, heard.Data), "?APRS? 34.0,-.1715,0200")
}

func TestTextAsSentMustSayExactlyTheNumber(t *testing.T) {
	// 0.1 + 0.2 is not 0.3, so the text does not say it: the number is written.
	sum := 0.1
	sum += 0.2
	c := aprs.TelemetryCoefficients{Addressee: "M0LTE", Coefficients: []float64{sum}, CoefficientsText: []string{"0.3"}}
	eq(t, info(t, c), ":M0LTE    :EQNS.0.30000000000000004")
	tel := aprs.Telemetry{Sequence: "001", Analog: []*float64{aprs.Ptr(1.0), nil, nil, nil, nil}, AnalogText: []string{"1.0000000001"}, Bits: aprs.Ptr("00000000")}
	eq(t, info(t, tel), "T#001,001,,,,,00000000")
	eq(t, aprs.PHG{Power: 1, Height: 61}.HeightFeet(), 10*math.Pow(2, 61))
}

func TestARefusalHasAReasonToRead(t *testing.T) {
	_, err := aprs.Encode(aprs.Message{Addressee: "N0CALL", Text: strings.Repeat("x", 68)})
	if !strings.Contains(err.Error(), "67 characters") {
		t.Error(err)
	}
	_, err = aprs.NewAddress("not an address!")
	var invalid *aprs.InvalidAddressError
	if !errors.As(err, &invalid) || invalid.Text != "not an address!" {
		t.Error(err)
	}
}

func TestSmallThingsAProgramReads(t *testing.T) {
	eq(t, aprs.TimestampDHM(9, 23, 45).String(), "092345z")
	eq(t, aprs.TimestampMDHM(10, 9, 5, 56).String(), "10090556")
	eq(t, aprs.Warning.String(), "warning")
	if len(aprs.AllCodes()) < 64 {
		t.Error("codes missing")
	}
	code, ok := aprs.CodeFromID("invalid-header")
	eq(t, ok && code == aprs.CodeInvalidHeader, true)
	eq(t, len(aprs.DeviceDatabaseVersion) > 8, true)
	_, ok = aprs.DeviceFromTocall("APRS")
	eq(t, ok, true)
	d, _ := aprs.SymbolCar.Description()
	eq(t, d, "Car")
	igate, ok := aprs.SymbolGateway.WithOverlay('I')
	eq(t, ok, true)
	d1, _ := igate.Description()
	d2, _ := aprs.SymbolGateway.Description()
	eq(t, d1, d2)
	_, ok = aprs.NewSymbol('/', ' ').Description()
	eq(t, ok, false)
	_, ok = aprs.SymbolCar.WithOverlay('I')
	eq(t, ok, false)
	phg := aprs.PHG{Power: 5, Height: 3, Gain: 6, Directivity: 2}
	directivity, ok := phg.DirectivityDegrees()
	eq(t, ok, true)
	eq(t, phg.PowerWatts(), uint32(25))
	eq(t, phg.HeightFeet(), 80.0)
	eq(t, phg.GainDB(), uint8(6))
	eq(t, directivity, uint16(90))
	phg.Directivity = 0
	_, ok = phg.DirectivityDegrees()
	eq(t, ok, false)
}

func TestHandBuiltMicEDataNeedsOnlyItsFields(t *testing.T) {
	report := aprs.MicEReport{
		Fields:   aprs.Positioned{Position: aprs.Position{Latitude: 42.179, Longitude: -71.1985}, Symbol: aprs.SymbolCar},
		TypeCode: aprs.Ptr('`'),
	}
	p, err := aprs.NewPacket(aprs.MustAddress("M0LTE"), aprs.MustAddress("APZ001"), nil, report)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, p.Information[0], byte('`'))
}

func TestCommentTelemetryIsOneValue(t *testing.T) {
	l := line(t)(me(t).Position(51.4543, -0.9781).Symbol(aprs.SymbolHouse).
		Telemetry(aprs.CommentTelemetry{Sequence: 1, Analog: []uint16{100, 200, 300, 400, 500}, Digital: aprs.Ptr(uint8(5))}).Build())
	if !strings.Contains(l, "|") {
		t.Error(l)
	}
}

func TestRefusalCodeGroups(t *testing.T) {
	message := func(addressee, text string, id *string) error {
		_, err := aprs.Encode(aprs.Message{Addressee: addressee, Text: text, MessageID: id})
		return err
	}
	eq(t, refusal(t, message("", "hi", nil)), aprs.RefusalInvalidAddressee)
	eq(t, refusal(t, message("N0CALL", strings.Repeat("x", 68), nil)), aprs.RefusalTooLong)
	eq(t, refusal(t, message("N0CALL", "a{b", nil)), aprs.RefusalInvalidCharacter)
	eq(t, refusal(t, message("N0CALL", "hi", aprs.Ptr("toolong"))), aprs.RefusalInvalidMessageID)
	// Several faults name the first code in the order: out of range before a line break.
	at := func(latitude float64) error {
		_, err := aprs.Encode(aprs.PositionReport{Fields: aprs.Positioned{Position: aprs.Position{Latitude: latitude}, Symbol: aprs.SymbolCar, Comment: "a\nb"}})
		return err
	}
	eq(t, refusal(t, at(95)), aprs.RefusalValueOutOfRange)
	eq(t, refusal(t, at(math.NaN())), aprs.RefusalNotANumber)
	eq(t, refusal(t, message("N0CALL", "{"+strings.Repeat("x", 70), nil)), aprs.RefusalInvalidCharacter)
	// A message to a bulletin addressee reads back as a bulletin: the addressee is the fault.
	eq(t, refusal(t, message("BLN1", "hi", nil)), aprs.RefusalInvalidAddressee)
}

func TestAX25HeaderRefusals(t *testing.T) {
	msg := aprs.Message{Addressee: "N0CALL", Text: "hi"}
	p, err := aprs.NewPacket(aprs.MustAddress("N0CALLXYZ"), aprs.MustAddress("APZ001"), nil, msg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AX25()
	eq(t, refusal(t, err), aprs.RefusalAX25CannotCarry)
	path := make([]aprs.PathEntry, 9)
	for i := range path {
		path[i] = aprs.PathEntry{Address: aprs.MustAddress("WIDE1-1")}
	}
	p, _ = aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), path, msg)
	_, err = p.AX25()
	eq(t, refusal(t, err), aprs.RefusalAX25CannotCarry)
	_, err = p.KISS(0)
	eq(t, refusal(t, err), aprs.RefusalAX25CannotCarry)
}

func TestBuilderRefusals(t *testing.T) {
	station := me(t)
	_, err := station.Message("G3NRW", strings.Repeat("x", 68)).Build()
	eq(t, refusal(t, err), aprs.RefusalTooLong)
	_, err = station.Object("TESTOBJ").At(1, 1).Symbol(aprs.SymbolCar).Build()
	eq(t, refusal(t, err), aprs.RefusalMissingField)
	_, err = station.Position(1, 1).Build()
	eq(t, refusal(t, err), aprs.RefusalMissingField)
	_, err = station.Weather().At(1, 1).TemperatureF(50).HumidityPercent(0).Build()
	eq(t, refusal(t, err), aprs.RefusalValueHasNoForm)
	_, err = station.Telemetry(1000).Build()
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
	_, err = station.Position(1, 1).Symbol(aprs.SymbolCar).Tone(aprs.ToneTone, 100).Build()
	eq(t, refusal(t, err), aprs.RefusalMissingField)
	_, err = aprs.NewStation("not a call")
	var invalid *aprs.InvalidAddressError
	eq(t, errors.As(err, &invalid), true)
}

func TestKISSFraming(t *testing.T) {
	frame := []byte{0x01, 0xC0, 0xDB, 0x02}
	wrapped := aprs.WrapKISS(frame, 3)
	eq(t, string(wrapped), string([]byte{0xC0, 0x30, 0x01, 0xDB, 0xDC, 0xDB, 0xDD, 0x02, 0xC0}))
	back, err := aprs.UnwrapKISS(wrapped)
	eq(t, err, nil)
	eq(t, string(back), string(frame))
	_, err = aprs.UnwrapKISS([]byte{0xC0, 0x01, 0x02, 0xC0})
	eq(t, err, aprs.ErrNotKISSData)
	_, err = aprs.DecodeKISS([]byte{0xC0, 0xC0}, aprs.Lenient)
	eq(t, err, aprs.ErrNotKISSData)
}

func TestAStationIsAValue(t *testing.T) {
	base := me(t)
	other, _ := base.To("APRS")
	via, _ := other.Via("WIDE1-1")
	eq(t, base.String(), "M0LTE-11>APZ001")
	eq(t, via.String(), "M0LTE-11>APRS,WIDE1-1")
	_, err := base.Via("WIDE1-1", "bad address")
	var invalid *aprs.InvalidAddressError
	eq(t, errors.As(err, &invalid), true)
	// A builder is a value too: setting on a copy does not change the original.
	b := base.Position(1, 1).Symbol(aprs.SymbolCar)
	_ = b.Comment("changed")
	p, _ := b.Build()
	eq(t, string(p.Information), "!0100.00N/00100.00E>")
}

// The vectors' codes.json lists every diagnostic and refusal code; this package has each, in the
// same order, with the same tolerability.
func TestCodesMatchTheVectors(t *testing.T) {
	text, err := os.ReadFile(filepath.Join(vectorsDir, "codes.json"))
	if err != nil {
		t.Skipf("%v: run 'git submodule update --init vectors'", err)
	}
	doc, err := neutral.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	codes := doc.(map[string]any)["codes"].([]any)
	all := aprs.AllCodes()
	eq(t, len(all), len(codes))
	for i, c := range codes {
		entry := c.(map[string]any)
		id := entry["id"].(string)
		code, ok := aprs.CodeFromID(id)
		if !ok || i >= len(all) || all[i] != code {
			t.Errorf("code %s is not number %d", id, i)
			continue
		}
		eq(t, code.Tolerable(), entry["tolerable"].(bool))
		eq(t, aprs.Lenient.Tolerates(code), code.Tolerable())
		eq(t, aprs.Strict.Tolerates(code), false)
	}
	refusals := doc.(map[string]any)["refusals"].([]any)
	allRefusals := aprs.AllRefusalCodes()
	eq(t, len(allRefusals), len(refusals))
	for i, r := range refusals {
		id := r.(map[string]any)["id"].(string)
		code, ok := aprs.RefusalCodeFromID(id)
		if !ok || i >= len(allRefusals) || allRefusals[i] != code || code.ID() != id {
			t.Errorf("refusal %s is not number %d", id, i)
		}
	}
}

// A !DAO! on an ambiguous position contradicts it whatever its precision, !W  ! too (vectors
// README, Encoding: it "reads back with dao-with-ambiguity").
func TestADAOOnAnAmbiguousPositionIsADefectWhateverItsPrecision(t *testing.T) {
	for _, l := range []string{"N0CALL>APZ001:!5132.  N/00005.  W-!W  !", "N0CALL>APZ001:!5132.  N/00005.  W-!3  !", "N0CALL>APZ001:!5132.  N/00005.  W-!W12!"} {
		p, _ := aprs.DecodeTNC2([]byte(l), aprs.Lenient)
		if len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != aprs.CodeDAOWithAmbiguity || p.Diagnostics[0].Severity != aprs.Warning {
			t.Errorf("%s: %v", l, p.Diagnostics)
		}
		strict, _ := aprs.DecodeTNC2([]byte(l), aprs.Strict)
		if strict.Data != aprs.Data(aprs.UnrecognizedMalformed) {
			t.Errorf("%s strict: %v", l, strict.Data)
		}
	}
}

// The zero ParseOptions is Lenient, the family's default, and every Code value is safe to ask
// about, including ones no code has.
func TestParseOptionsZeroValueIsLenientAndEveryCodeIsSafe(t *testing.T) {
	var zero aprs.ParseOptions
	eq(t, zero, aprs.Lenient)
	line := []byte("N1EOE>APN391:!4216.95n/07243.20w#phg6230/ Easthampton MA")
	p, _ := aprs.DecodeTNC2(line, zero)
	eq(t, p.HasErrors(), false)
	eq(t, p.HasWarnings(), true)
	for c := 0; c < 256; c++ {
		code := aprs.Code(c)
		eq(t, aprs.Lenient.Tolerates(code), code.Tolerable())
		eq(t, aprs.Strict.Tolerates(code), false)
		eq(t, aprs.Lenient.Without(code).Tolerates(code), false)
		eq(t, aprs.Strict.With(code).Tolerates(code), code.Tolerable())
	}
	eq(t, aprs.Lenient.Without(aprs.Code(250)), aprs.Lenient)
	eq(t, aprs.Severity(250).String(), "severity-250")
}

// To and Via keep the station on an error, and an empty address is never written.
func TestAnEmptyAddressIsRefused(t *testing.T) {
	base := me(t)
	via, err := base.Via("WIDE1-1", "bad*")
	if err == nil || via.String() != base.String() {
		t.Errorf("Via gave %s, %v", via, err)
	}
	to, err := base.To("not a call")
	if err == nil || to.String() != base.String() {
		t.Errorf("To gave %s, %v", to, err)
	}
	_, err = aprs.NewPacket(aprs.Address{}, aprs.MustAddress("APZ001"), nil, aprs.Status{Text: "x"})
	eq(t, refusal(t, err), aprs.RefusalInvalidAddress)
	_, err = aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.Address{}, nil, aprs.Status{Text: "x"})
	eq(t, refusal(t, err), aprs.RefusalInvalidAddress)
	_, err = aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), []aprs.PathEntry{{}}, aprs.Status{Text: "x"})
	eq(t, refusal(t, err), aprs.RefusalInvalidAddress)
	var zero aprs.Station
	_, err = zero.Status("x").Build()
	eq(t, refusal(t, err), aprs.RefusalInvalidAddress)
}

// Data no decoder could give is refused before anything is written: an enumeration value
// outside its type, or text that is not UTF-8. A pointer to data is the data it points to.
func TestHandBuiltDataIsCheckedFirst(t *testing.T) {
	pos := func() aprs.Positioned {
		return aprs.Positioned{Position: aprs.Position{Latitude: 51, Longitude: -1}, Symbol: aprs.SymbolCar}
	}
	f := pos()
	f.Frequency = &aprs.VoiceFrequency{MHz: 145.5, Tone: aprs.Ptr(aprs.Tone(9)), ToneValue: aprs.Ptr(uint16(100))}
	_, err := aprs.Encode(aprs.PositionReport{Fields: f})
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
	f = pos()
	f.DAO = &aprs.DAO{Datum: 'W', Precision: aprs.DAOPrecision(9)}
	_, err = aprs.Encode(aprs.PositionReport{Fields: f})
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
	f = pos()
	f.Symbol = aprs.Symbol{Table: '\\', Code: 'l'}
	f.Area = &aprs.AreaObject{Shape: aprs.AreaShape(99)}
	_, err = aprs.Encode(aprs.PositionReport{Fields: f})
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
	_, err = aprs.Encode(aprs.RawWeather{Format: aprs.RawWeatherFormat(9), Data: "x"})
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
	_, err = aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), nil, aprs.MicEReport{MicEMessage: -3, Fields: pos()})
	eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)

	_, err = aprs.Encode(aprs.Message{Addressee: "N0CALL", Text: "\xff\xfe"})
	eq(t, refusal(t, err), aprs.RefusalInvalidCharacter)
	f = pos()
	f.Comment = "\xff\x80"
	_, err = aprs.Encode(aprs.PositionReport{Fields: f})
	eq(t, refusal(t, err), aprs.RefusalInvalidCharacter)

	report := aprs.PositionReport{Fields: pos()}
	byValue, err := aprs.Encode(report)
	eq(t, err, nil)
	byPointer, err := aprs.Encode(&report)
	eq(t, err, nil)
	eq(t, string(byPointer), string(byValue))
	p, err := aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), nil, &report)
	eq(t, err, nil)
	_, isValue := p.Data.(aprs.PositionReport)
	eq(t, isValue, true)
	var none *aprs.PositionReport
	_, err = aprs.Encode(none)
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
	_, err = aprs.Encode(nil)
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
}

// A device's features are the caller's to change.
func TestDeviceFeaturesAreACopy(t *testing.T) {
	for _, tocall := range []string{"APAIOR", "APERRB"} {
		d, ok := aprs.DeviceFromTocall(tocall)
		if !ok || len(d.Features) == 0 {
			continue
		}
		before := d.Features[0]
		d.Features[0] = "changed"
		again, _ := aprs.DeviceFromTocall(tocall)
		eq(t, again.Features[0], before)
		return
	}
	t.Skip("no device with features found to try")
}

// A telemetry builder given six analog values and a line break in the comment refuses with the
// line break (invalid-character), which the refusal order puts before too-long.
func TestTelemetryBuilderRefusesInEncoderOrder(t *testing.T) {
	v := func(x float64) *float64 { return &x }
	st := me(t)
	_, err := st.Telemetry(1).Analog(v(1), v(2), v(3), v(4), v(5), v(6)).Comment("a\rb").Build()
	if err == nil {
		t.Fatal("expected a refusal")
	}
	eq(t, err.(*aprs.EncodeError).Code(), aprs.RefusalInvalidCharacter)
	_, err = st.Telemetry(1).Analog(v(1), v(2), v(3), v(4), v(5), v(6)).Build()
	eq(t, err.(*aprs.EncodeError).Code(), aprs.RefusalTooLong)
}
