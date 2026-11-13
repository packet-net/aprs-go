package aprs_test

import (
	"errors"
	"fmt"
	"strings"

	aprs "github.com/packet-net/aprs-go"
)

func Example_decoding() {
	packet, err := aprs.DecodeTNC2([]byte("N0CALL-9>APZ001,WIDE1-1,qAR,M0LTE-10:=5130.00N/00007.00W>088/036Mobile"), aprs.Lenient)
	if err != nil {
		panic(err) // only an unusable header is an error
	}
	q, _ := packet.QConstruct()
	fmt.Println(packet.Source, q.Construct, q.Station)

	report := packet.Data.(aprs.PositionReport)
	f := report.Fields
	fmt.Printf("%.4f %.4f messaging=%v\n", f.Position.Latitude, f.Position.Longitude, report.Messaging)
	fmt.Println(*f.CourseDegrees, *f.SpeedKnots, f.Comment, len(packet.Diagnostics))
	// Output:
	// N0CALL-9 qAR M0LTE-10
	// 51.5000 -0.1167 messaging=true
	// 88 36 Mobile 0
}

func Example_typeSwitch() {
	packet, _ := aprs.DecodeTNC2([]byte("M0LTE>APZ001::G3NRW    :Hi Ian{01"), aprs.Lenient)
	switch d := packet.Data.(type) {
	case aprs.Message:
		fmt.Printf("to %s: %s (id %s)\n", d.Addressee, d.Text, *d.MessageID)
	case aprs.PositionReport:
		fmt.Println("a position")
	case aprs.Unrecognized:
		fmt.Println("nothing decoded:", packet.Diagnostics)
	}
	// Output: to G3NRW: Hi Ian (id 01)
}

func Example_ax25() {
	frame := []byte{
		0x82, 0xA0, 0xB4, 0x60, 0x60, 0x62, 0xE0, 0x9A, 0x60, 0x98, 0xA8, 0x8A, 0x40, 0x72, 0x9A, 0x60, 0x98, 0xA8, 0x8A, 0x40,
		0xE2, 0xAE, 0x92, 0x88, 0x8A, 0x64, 0x40, 0x63, 0x03, 0xF0, '>', 'h', 'e', 'l', 'l', 'o',
	}
	packet, err := aprs.DecodeAX25(frame, aprs.Lenient)
	if err != nil {
		panic(err)
	}
	fmt.Println(packet)
	again, _ := packet.AX25()
	fmt.Println(string(again) == string(frame))
	// Output:
	// M0LTE-9>APZ001,M0LTE-1*,WIDE2-1:>hello
	// true
}

func Example_strictAndLenient() {
	line := []byte("N1EOE>APN391:!4216.95n/07243.20w#phg6230/ Easthampton MA")

	// Lenient: the lower-case hemispheres are read, and each is reported.
	lenient, _ := aprs.DecodeTNC2(line, aprs.Lenient)
	_, isPosition := lenient.Data.(aprs.PositionReport)
	fmt.Println(isPosition, lenient.Diagnostics[0].Severity, lenient.Diagnostics[0].Code)

	// Strict: the same defect is an error, and the data is not decoded.
	strict, _ := aprs.DecodeTNC2(line, aprs.Strict)
	fmt.Println(strict.Data == aprs.Data(aprs.UnrecognizedMalformed), strict.HasErrors())

	// Or tolerate everything except this one defect.
	options := aprs.Lenient.Without(aprs.CodeLowercaseHemisphere)
	picky, _ := aprs.DecodeTNC2(line, options)
	fmt.Println(picky.HasErrors())
	// Output:
	// true warning lowercase-hemisphere
	// true true
	// true
}

func ExampleStation() {
	me, err := aprs.NewStation("M0LTE-9")
	if err != nil {
		panic(err)
	}
	if me, err = me.Via("WIDE1-1", "WIDE2-1"); err != nil {
		panic(err)
	}

	beacon, _ := me.Position(51.4543, -0.9781).Symbol(aprs.SymbolCar).CourseDegrees(88).SpeedKnots(36).AltitudeFeet(120).Comment("Mobile").Build()
	fmt.Println(beacon)

	message, _ := me.Message("G3NRW", "Hi Ian").MessageID("01").Build()
	fmt.Println(message)

	repeater, _ := me.Object("MYRPTR").
		At(51.45, -0.98).
		Symbol(aprs.SymbolRepeater).
		Timestamp(aprs.TimestampDHM(25, 18, 30)).
		Frequency(145.725).
		Tone(aprs.ToneTone, 118).
		OffsetKHz(-600).
		Build()
	fmt.Println(repeater)

	weather, _ := me.Weather().At(51.45, -0.98).WindDirectionDegrees(220).WindSpeedMph(4).WindGustMph(5).TemperatureF(77).Build()
	fmt.Println(weather)
	// Output:
	// M0LTE-9>APZ001,WIDE1-1,WIDE2-1:!5127.26N/00058.69W>088/036/A=000120Mobile
	// M0LTE-9>APZ001,WIDE1-1,WIDE2-1::G3NRW    :Hi Ian{01
	// M0LTE-9>APZ001,WIDE1-1,WIDE2-1:;MYRPTR   *251830z5127.00N/00058.80Wr145.725MHz T118 -060
	// M0LTE-9>APZ001,WIDE1-1,WIDE2-1:!5127.00N/00058.80W_220/004g005t077
}

func ExampleStation_telemetry() {
	me, _ := aprs.NewStation("M0LTE-9")
	report, _ := me.Telemetry(5).Analog(aprs.Ptr(13.8), nil, aprs.Ptr(21.0)).Bits("10000000").Build()
	fmt.Println(report)
	names, _ := me.TelemetryNames("Battery", "", "Temp").Build()
	fmt.Println(names)
	// Output:
	// M0LTE-9>APZ001:T#005,13.8,,021,,,10000000
	// M0LTE-9>APZ001::M0LTE-9  :PARM.Battery,,Temp
}

func ExampleNewPacket() {
	report := aprs.PositionReport{
		Messaging: true,
		Fields: aprs.Positioned{
			Position:      aprs.Position{Latitude: 51.5, Longitude: -0.116667},
			Symbol:        aprs.SymbolCar,
			CourseDegrees: aprs.Ptr(88.0),
			SpeedKnots:    aprs.Ptr(36.0),
			AltitudeFeet:  aprs.Ptr(120.0),
			Comment:       "Mobile",
		},
	}
	path := []aprs.PathEntry{{Address: aprs.MustAddress("WIDE1-1")}}
	packet, err := aprs.NewPacket(aprs.MustAddress("N0CALL-9"), aprs.MustAddress("APZ001"), path, report)
	if err != nil {
		panic(err)
	}
	fmt.Println(packet)
	// Output: N0CALL-9>APZ001,WIDE1-1:=5130.00N/00007.00W>088/036/A=000120Mobile
}

func ExampleEncode() {
	message := aprs.Message{
		Addressee: "G3NRW",
		Text:      "Hi Ian",
		MessageID: aprs.Ptr("01"),
		ReplyAck:  aprs.Ptr(""), // says this station supports reply-acks
	}
	info, _ := aprs.Encode(message)
	fmt.Println(string(info))

	// What the spec forbids is refused, with the reason.
	tooLong := aprs.Message{Addressee: "N0CALL", Text: strings.Repeat("x", 68)}
	_, err := aprs.Encode(tooLong)
	var refused *aprs.EncodeError
	if errors.As(err, &refused) {
		fmt.Println(refused.Code(), "-", refused)
	}
	// Output:
	// :G3NRW    :Hi Ian{01}
	// too-long - message text is limited to 67 characters (APRS12c ch. 14)
}

func ExamplePacket_Device() {
	packet, _ := aprs.DecodeTNC2([]byte("N1JCM-9>TRQP7T,WA1PLE-4*:`c'wl|+>/`\"4-}_%"), aprs.Lenient)
	report := packet.Data.(aprs.MicEReport)
	fmt.Println(*report.Fields.CourseDegrees, *report.Fields.SpeedKnots, report.DeviceSuffix)

	device, _ := packet.Device()
	fmt.Println(device.Vendor, device.Model)

	// And back: the same information field and destination.
	again, _ := aprs.NewPacket(packet.Source, packet.Destination, packet.Path, report)
	fmt.Println(again.Destination == packet.Destination, string(again.Information) == string(packet.Information))
	// Output:
	// 215 9 _%
	// Yaesu FTM-400DR
	// true true
}

func ExamplePacket_KISS() {
	me, _ := aprs.NewStation("M0LTE-9")
	status, _ := me.Status("hello").Build()
	frame, _ := status.KISS(0)
	fmt.Printf("% X\n", frame[:4])

	back, _ := aprs.DecodeKISS(frame, aprs.Lenient)
	fmt.Println(back)
	// Output:
	// C0 00 82 A0
	// M0LTE-9>APZ001:>hello
}
