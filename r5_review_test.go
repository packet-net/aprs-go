package aprs_test

import (
	"math"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// The review of round 5 found written bytes that decode differently, a panic on hostile inner
// data, a regression and a wrong refusal code; each reproducer is a test here.

// innerOf is a third-party inner packet as a program may hold one: any header, the information
// field as it stands and whatever data it says.
func innerOf(source aprs.Address, information string, data aprs.Data, diagnostics ...aprs.Diagnostic) *aprs.Packet {
	return &aprs.Packet{Source: source, Destination: aprs.MustAddress("APZ001"), Information: []byte(information), Data: data, Diagnostics: diagnostics}
}

// D1: a NaN or an infinity in an inner packet's data compared as absent, so bytes with no altitude
// were written for data that had one. Each case has a control that matches its bytes exactly and
// is written, so the refusal can only come from the number.
func TestInnerNonFiniteNumberIsNotANumber(t *testing.T) {
	n0call := aprs.MustAddress("N0CALL")
	position := func(altitude *float64) aprs.Data {
		return aprs.PositionReport{Fields: aprs.Positioned{
			Position: aprs.Position{Latitude: 51.5, Longitude: -0.5}, Symbol: aprs.NewSymbol('/', '>'), AltitudeFeet: altitude,
		}}
	}
	const information = "!5130.00N/00030.00W>"
	// The control: the same inner packet with no altitude is written.
	if b, err := aprs.Encode(aprs.ThirdParty{Packet: innerOf(n0call, information, position(nil))}); err != nil || string(b) != "}N0CALL>APZ001:"+information {
		t.Fatalf("the control: %q, %v", b, err)
	}
	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		b, err := aprs.Encode(aprs.ThirdParty{Packet: innerOf(n0call, information, position(aprs.Ptr(v)))})
		if err == nil {
			t.Fatalf("%v: wrote %q", v, b)
		}
		eq(t, refusal(t, err), aprs.RefusalNotANumber)
	}
	// A telemetry analog value, two levels down; its control is written.
	telemetry := func(second *float64) aprs.Data {
		return aprs.Telemetry{Sequence: "001", Analog: []*float64{aprs.Ptr(1.0), second, nil, nil, nil}, Bits: aprs.Ptr("00000000")}
	}
	const telemetryInformation = "T#001,1,,,,,00000000"
	twoDeep := func(second *float64) aprs.Data {
		inner := innerOf(n0call, telemetryInformation, telemetry(second))
		return aprs.ThirdParty{Packet: innerOf(n0call, "}N0CALL>APZ001:"+telemetryInformation, aprs.ThirdParty{Packet: inner})}
	}
	if b, err := aprs.Encode(twoDeep(nil)); err != nil {
		t.Fatalf("the telemetry control: %q, %v", b, err)
	}
	_, err := aprs.Encode(twoDeep(aprs.Ptr(math.NaN())))
	if err == nil {
		t.Fatal("a NaN two deep was written")
	}
	eq(t, refusal(t, err), aprs.RefusalNotANumber)
	// The same data at the top is refused the same way.
	_, err = aprs.Encode(position(aprs.Ptr(math.Inf(1))))
	eq(t, refusal(t, err), aprs.RefusalNotANumber)
}

// D2: an out-of-range enumeration in inner data indexed past a name table.
func TestInnerOutOfRangeValuesAreRefusedNotAPanic(t *testing.T) {
	n0call := aprs.MustAddress("N0CALL")
	bad := aprs.Positioned{Position: aprs.Position{Latitude: 51.5, Longitude: 0.5}, Symbol: aprs.NewSymbol('/', '>'), Compressed: true,
		Compression: &aprs.CompressionType{Fix: 9}}
	for name, data := range map[string]aprs.Data{
		"compression": aprs.PositionReport{Fields: bad},
		"raw weather": aprs.RawWeather{Format: 9, Data: "x"},
		"undecoded":   aprs.Unrecognized(200),
		"pointer":     &aprs.PositionReport{Fields: bad},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aprs.Encode(aprs.ThirdParty{Packet: innerOf(n0call, ">hi", data)})
			if err == nil {
				t.Fatal("written")
			}
			eq(t, refusal(t, err), aprs.RefusalValueOutOfRange)
		})
	}
}

// D3: an inner packet whose source only a third-party header allows is written inside a
// third-party packet, as 1.2.0 wrote it; written on its own it is read back by the outer rules.
func TestInnerThirdPartySourceIsWrittenInsideAThirdPartyPacket(t *testing.T) {
	source, err := aprs.ThirdPartySource("PY2SP_R-R")
	if err != nil {
		t.Fatal(err)
	}
	inner := innerOf(source, ">hi", aprs.Status{Text: "hi"})
	inner.Destination = aprs.MustAddress("APRS")
	b, err := aprs.Encode(aprs.ThirdParty{Packet: inner})
	if err != nil || string(b) != "}PY2SP_R-R>APRS:>hi" {
		t.Fatalf("%q, %v", b, err)
	}
	_, err = inner.TNC2Line()
	eq(t, refusal(t, err), aprs.RefusalInvalidAddress)
	_, err = inner.AX25()
	eq(t, refusal(t, err), aprs.RefusalInvalidAddress)
}

// D4: an inner packet whose text is unchanged but whose decode has a warning its data does not
// give is does-not-read-back, not text-reads-as-field; a warning given that the bytes do not
// carry has no form.
func TestInnerDiagnosticsMismatchCodes(t *testing.T) {
	outer, err := aprs.DecodeTNC2([]byte("N0CALL>APRS:}N0CALL>APRS:!5130.00n/00030.00W>hi"), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	tp := outer.Data.(aprs.ThirdParty)
	if len(tp.Packet.Diagnostics) == 0 {
		t.Fatal("the decode gave the inner packet no diagnostic")
	}
	// As decoded it is written as received.
	if b, err := aprs.Encode(tp); err != nil || string(b) != "}N0CALL>APRS:!5130.00n/00030.00W>hi" {
		t.Fatalf("%q, %v", b, err)
	}
	// The same packet with its diagnostics cleared: its text has not changed, the decode has a
	// warning the data does not give.
	cleared := *tp.Packet
	cleared.Diagnostics = nil
	_, err = aprs.Encode(aprs.ThirdParty{Packet: &cleared})
	eq(t, refusal(t, err), aprs.RefusalDoesNotReadBack)
	// A warning given that clean bytes do not read back with.
	given := innerOf(aprs.MustAddress("N0CALL"), ">hi", aprs.Status{Text: "hi"}, aprs.Diagnostic{Severity: aprs.Warning, Code: aprs.CodeControlCharacterInText, Offset: -1})
	_, err = aprs.Encode(aprs.ThirdParty{Packet: given})
	eq(t, refusal(t, err), aprs.RefusalValueHasNoForm)
	// Text that does hold a rule's fault is that rule's code, as for top-level data.
	text := innerOf(aprs.MustAddress("N0CALL"), ">hi\x01there", aprs.Status{Text: "hi\x01there"})
	_, err = aprs.Encode(aprs.ThirdParty{Packet: text})
	eq(t, refusal(t, err), aprs.RefusalInvalidCharacter)
}

// Second review (R28): test and user-defined data are data, a CR or LF inside is written in an
// information field alone and refused by a line; only data that ENDS in one is invalid-character.
func TestLineBreakInsideTestAndUserDefinedData(t *testing.T) {
	for name, data := range map[string]aprs.Data{
		"test":         aprs.TestData{Data: "a\rb"},
		"user-defined": aprs.UserDefined{UserID: 'A', PacketType: 'a', Data: []byte("x\ny")},
	} {
		info, err := aprs.Encode(data)
		if err != nil {
			t.Errorf("%s: an information field with a line break inside was refused: %v", name, err)
			continue
		}
		p, err := aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), nil, data)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.TNC2Line()
		eq(t, refusal(t, err), aprs.RefusalTNC2CannotCarry)
		if frame, err := p.AX25(); err != nil || len(frame) == 0 {
			t.Errorf("%s: a frame carries it: %v", name, err)
		}
		_ = info
	}
	for name, data := range map[string]aprs.Data{
		"test":         aprs.TestData{Data: "ab\r"},
		"user-defined": aprs.UserDefined{UserID: 'A', PacketType: 'a', Data: []byte("xy\n")},
	} {
		_, err := aprs.Encode(data)
		if err == nil {
			t.Errorf("%s: data ending in a line break was written", name)
			continue
		}
		eq(t, refusal(t, err), aprs.RefusalInvalidCharacter)
	}
}

// A corridor width belongs to a line area: anywhere else it is field-not-allowed, not
// does-not-read-back.
func TestCorridorWidthOnAnAreaThatIsNotALine(t *testing.T) {
	area := func(shape aprs.AreaShape) aprs.Data {
		return aprs.ObjectReport{Name: "AREA", Timestamp: &aprs.Timestamp{Kind: aprs.DayHourMinute, UTC: true, Day: 9, Hour: 23, Minute: 45},
			Fields: aprs.Positioned{Position: aprs.Position{Latitude: 49.05833333333333, Longitude: -72.02916666666667}, Symbol: aprs.NewSymbol('\\', 'l'),
				Area: &aprs.AreaObject{Shape: shape, Color: aprs.ColorBlue, LatOffset: 20, LonOffset: 20, CorridorWidthMiles: aprs.Ptr(uint16(5))}}}
	}
	if _, err := aprs.Encode(area(aprs.AreaLineDownRight)); err != nil {
		t.Fatalf("a line with a corridor width: %v", err)
	}
	_, err := aprs.Encode(area(aprs.AreaOpenCircle))
	if err == nil {
		t.Fatal("a corridor width on an open circle was written")
	}
	eq(t, refusal(t, err), aprs.RefusalFieldNotAllowed)
}

// An inner path two levels down with an entry not marked used before one that is: the rule's code
// is invalid-third-party-header, not does-not-read-back.
func TestOutOfOrderPathTwoLevelsDown(t *testing.T) {
	n0call := aprs.MustAddress("N0CALL")
	deeper := innerOf(n0call, ">hi", aprs.Status{Text: "hi"})
	deeper.Path = []aprs.PathEntry{{Address: aprs.MustAddress("WIDE1-1")}, {Address: aprs.MustAddress("WIDE2-1"), Used: true}}
	middle := innerOf(n0call, "}N0CALL>APZ001,WIDE1-1,WIDE2-1*:>hi", aprs.ThirdParty{Packet: deeper})
	_, err := aprs.Encode(aprs.ThirdParty{Packet: middle})
	if err == nil {
		t.Fatal("written")
	}
	eq(t, refusal(t, err), aprs.RefusalInvalidThirdPartyHeader)
}

// The read-back is under the header written: unrecognized data whose raw bytes are Mic-E-shaped,
// put under a destination that makes them a Mic-E report, is not-encodable at the top and as a
// third-party inner packet, and the same bytes under a plain destination are written.
func TestFieldIsReadUnderTheHeaderWritten(t *testing.T) {
	const raw = "`(_fn\"Oj/"
	n0call := aprs.MustAddress("N0CALL")
	// Held as a decoder holds it under a plain destination: unrecognized, with its error.
	plain, err := aprs.DecodeTNC2([]byte("N0CALL>APZ001:"+raw), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	held := func(destination string) *aprs.Packet {
		p := *plain
		p.ThirdParty = true
		p.Destination = aprs.MustAddress(destination)
		return &p
	}
	if line, err := held("APZ001").TNC2Line(); err != nil || string(line) != "N0CALL>APZ001:"+raw {
		t.Fatalf("the plain destination: %q, %v", line, err)
	}
	if _, err := aprs.Encode(aprs.ThirdParty{Packet: held("APZ001")}); err != nil {
		t.Fatalf("the plain destination inside a third-party packet: %v", err)
	}
	mic := held("S32U6T")
	_, err = mic.TNC2Line()
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
	_, err = mic.AX25()
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
	_, err = aprs.Encode(aprs.ThirdParty{Packet: mic})
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
	// A Mic-E report put under a plain destination is the same fault the other way.
	heard, err := aprs.DecodeTNC2([]byte("N0CALL>S32U6T:"+raw), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	moved := *heard
	moved.Destination = aprs.MustAddress("APZ001")
	_, err = moved.TNC2Line()
	eq(t, refusal(t, err), aprs.RefusalNotEncodable)
	if line, err := heard.TNC2Line(); err != nil || string(line) != "N0CALL>S32U6T:"+raw {
		t.Fatalf("under its own destination: %q, %v", line, err)
	}
	// Inner Mic-E data under the destination it computes is written.
	inner := &aprs.Packet{Source: n0call, Destination: aprs.MustAddress("S32U6T"), Information: []byte(raw), Data: heard.Data, ThirdParty: true}
	if b, err := aprs.Encode(aprs.ThirdParty{Packet: inner}); err != nil || string(b) != "}N0CALL>S32U6T:"+raw {
		t.Fatalf("inner Mic-E: %q, %v", b, err)
	}
}

// A held Mic-E report re-addressed to another Mic-E destination is not-encodable: its data is read
// back under the destination written, and a different latitude digit or SSID is different data.
// A packet built from a latitude between steps is still written under its computed destination.
func TestHeldMicEReportReAddressedToAnotherMicEDestination(t *testing.T) {
	const raw = "`(_fn\"Oj/"
	heard, err := aprs.DecodeTNC2([]byte("N0CALL>S32U6T:"+raw), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := heard.TNC2Line(); err != nil {
		t.Fatalf("under its own destination: %v", err)
	}
	for _, destination := range []string{"S32U7T", "S32U6T-3", "T32U6T"} {
		moved := *heard
		moved.Destination = aprs.MustAddress(destination)
		_, err := moved.TNC2Line()
		if err == nil {
			t.Errorf("%s: written", destination)
			continue
		}
		eq(t, refusal(t, err), aprs.RefusalNotEncodable)
		_, err = moved.AX25()
		eq(t, refusal(t, err), aprs.RefusalNotEncodable)
	}
	built, err := me(t).MicE(33.4273333, -12.12904).MicEMessage(aprs.MicEReturning).Symbol(aprs.NewSymbol('/', 'j')).Build()
	if err != nil {
		t.Fatal(err)
	}
	if line, err := built.TNC2Line(); err != nil {
		t.Errorf("a built packet under its computed destination: %q, %v", line, err)
	}
}

// A defect of one field or entry is listed once for each occurrence (two empty path entries, in an
// outer header and in an inner one alike), a judgement about a whole once (several used markers,
// a text's control characters), and a strict decoder lists only what it met up to the defect it
// rejects by.
func TestDiagnosticsAreListedPerOccurrenceAndStrictStopsAtTheFirst(t *testing.T) {
	count := func(ds []aprs.Diagnostic, code aprs.Code) int {
		n := 0
		for _, d := range ds {
			if d.Code == code {
				n++
			}
		}
		return n
	}
	outer, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001,,,WIDE1-1:>hi"), aprs.Lenient)
	eq(t, count(outer.Diagnostics, aprs.CodeEmptyPathEntry), 2)
	nested, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001:}A>B,,:>hi"), aprs.Lenient)
	eq(t, count(nested.Data.(aprs.ThirdParty).Packet.Diagnostics, aprs.CodeEmptyPathEntry), 2)
	markers, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001,A*,B*,C*:>hi"), aprs.Lenient)
	eq(t, count(markers.Diagnostics, aprs.CodeMultipleUsedMarkers), 1)
	text, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001:>a\x01b\x02c"), aprs.Lenient)
	eq(t, count(text.Diagnostics, aprs.CodeControlCharacterInText), 1)
	hemispheres, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001:!5130.00n/00030.00w>"), aprs.Lenient)
	eq(t, count(hemispheres.Diagnostics, aprs.CodeLowercaseHemisphere), 2)
	strict, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001:!5130.00n/00030.00w>\x01x"), aprs.Strict)
	eq(t, len(strict.Diagnostics), 1)
}
