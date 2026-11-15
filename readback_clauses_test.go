package aprs

import "testing"

// Each clause of the read-back's comparison is pinned here on its own, by handing readBack bytes
// and data directly: no vectors case can reach them, because a rule check always comes first.
// A test fails when the clause it names is taken out.

func clausePosition() PositionReport {
	return PositionReport{Fields: Positioned{Position: Position{Latitude: 51.5, Longitude: -0.5}, Symbol: NewSymbol('/', '>')}}
}

func readsBackAs(t *testing.T, data Data, bytes string) *EncodeError {
	t.Helper()
	_, _, err := readBack(data, onlyForm([]byte(bytes)))
	return err
}

// A read-back that sees only a warning refuses: the bytes decode to exactly the data given, and a
// lower-case hemisphere is the one warning.
func TestReadBackRefusesBytesThatDecodeWithOnlyAWarning(t *testing.T) {
	if err := readsBackAs(t, clausePosition(), "!5130.00N/00030.00W>"); err != nil {
		t.Fatalf("the clean bytes were refused: %v", err)
	}
	err := readsBackAs(t, clausePosition(), "!5130.00n/00030.00W>")
	if err == nil {
		t.Fatal("bytes that decode with a warning were accepted")
	}
	if err.Code() != RefusalDoesNotReadBack {
		t.Errorf("refused with %s", err.Code().ID())
	}
}

// An info the rules list for no form refuses: a reserved data type reads back as itself, with an
// info nothing lists.
func TestReadBackRefusesAnUnlistedInfo(t *testing.T) {
	err := readsBackAs(t, UnrecognizedReservedDataType, "&something")
	if err == nil {
		t.Fatal("bytes that decode with an unlisted info were accepted")
	}
	// The same data is not refused for its fields: only the info is wrong.
	ctx := newContext(Lenient)
	if got := decodeInformation(ctx, Address{DefaultDestination}, []byte("&something")); got != Data(UnrecognizedReservedDataType) {
		t.Fatalf("the bytes decode to %v, not the data given", got)
	}
}

// An info listed for one form is not accepted on another: obsolete-format belongs to positionless
// weather, raw weather, raw NMEA, a Maidenhead beacon and Mic-E Rev 0 telemetry, the others to
// their own forms.
func TestAnInfoListedForOneFormIsNotAcceptedOnAnother(t *testing.T) {
	typeCode := '`'
	forms := map[string]Data{
		"positionless weather": WeatherReport{}, "raw weather": RawWeather{}, "nmea": NMEA{}, "maidenhead": MaidenheadBeacon{},
		"mic-e rev 0": MicEReport{LegacyTelemetry: []byte{1}, TypeCode: &typeCode}, "mic-e without a type code": MicEReport{},
		"mic-e": MicEReport{TypeCode: &typeCode}, "message": Message{}, "status": Status{}, "position": clausePosition(),
		"third-party": ThirdParty{}, "object": ObjectReport{}, "telemetry": Telemetry{},
	}
	listed := map[Code][]string{
		CodeObsoleteFormat:           {"positionless weather", "raw weather", "nmea", "maidenhead", "mic-e rev 0"},
		CodeMicEMissingDeviceType:    {"mic-e without a type code"},
		CodeInvalidQuery:             {"message"},
		CodeInvalidTelemetryMetadata: {"message"},
	}
	for code, ok := range listed {
		for name, data := range forms {
			want := false
			for _, n := range ok {
				want = want || n == name
			}
			if got := allowedInfo(code, data); got != want {
				t.Errorf("%s on %s: accepted %v, want %v", code.ID(), name, got, want)
			}
		}
	}
	// And through the read-back: raw weather's obsolete-format is accepted, a Maidenhead beacon's
	// bytes with a form that does not list it are not.
	if err := readsBackAs(t, RawWeather{Format: UltimeterPacket, Data: "x"}, "$ULTWx"); err != nil {
		t.Errorf("raw weather: %v", err)
	}
}

// A decode with a field the data does not give refuses, at the top and inside a third-party
// packet: the bytes decode to the data given and an altitude besides.
func TestReadBackRefusesADecodeWithAFieldTheDataLacks(t *testing.T) {
	if err := readsBackAs(t, clausePosition(), "!5130.00N/00030.00W>"); err != nil {
		t.Fatalf("the bytes with no altitude were refused: %v", err)
	}
	err := readsBackAs(t, clausePosition(), "!5130.00N/00030.00W>/A=000100")
	if err == nil {
		t.Fatal("bytes that decode with an altitude the data lacks were accepted")
	}
	// Inside a third-party packet the same holds.
	n0call := Address{"N0CALL"}
	inner := &Packet{Source: n0call, Destination: Address{"APZ001"}, Information: []byte("!5130.00N/00030.00W>/A=000100"), Data: clausePosition(), ThirdParty: true}
	if err := readsBackAs(t, ThirdParty{Packet: inner}, "}N0CALL>APZ001:!5130.00N/00030.00W>/A=000100"); err == nil {
		t.Error("an inner decode with an altitude the data lacks was accepted")
	}
}

// A third-party packet's inner warnings and errors are compared as a multiset with those given:
// the same ones (in any order, infos aside) match, one more or one fewer, or a repeat, does not.
func TestInnerWarningMultiset(t *testing.T) {
	// "n" and "w" lower case: two lower-case-hemisphere warnings.
	const information = "!5130.00n/00030.00w>"
	decoded := Decode(Address{"N0CALL"}, Address{"APZ001"}, nil, []byte(information), Lenient)
	decoded.ThirdParty = true
	given := decoded.Diagnostics
	warnings := 0
	for _, d := range given {
		if d.Severity == Warning {
			warnings++
		}
	}
	if warnings != 2 {
		t.Fatalf("expected two warnings from the decode, got %v", given)
	}
	outer := func(diagnostics []Diagnostic) *EncodeError {
		inner := *decoded
		inner.Diagnostics = diagnostics
		_, _, err := readBack(ThirdParty{Packet: &inner}, onlyForm([]byte("}N0CALL>APZ001:"+information)))
		return err
	}
	if err := outer(given); err != nil {
		t.Fatalf("the decode's own diagnostics were refused: %v", err)
	}
	reversed := []Diagnostic{given[len(given)-1]}
	reversed = append(reversed, given[:len(given)-1]...)
	if err := outer(reversed); err != nil {
		t.Errorf("the same diagnostics in another order were refused: %v", err)
	}
	withInfo := append(append([]Diagnostic(nil), given...), Diagnostic{Severity: Info, Code: CodeObsoleteFormat, Offset: -1})
	if err := outer(withInfo); err != nil {
		t.Errorf("an info given that the decode lacks was refused: %v", err)
	}
	if err := outer(given[:1]); err == nil || err.Code() != RefusalDoesNotReadBack {
		t.Errorf("one warning fewer than the decode gives was accepted or wrongly refused: %v", err)
	}
	extra := append(append([]Diagnostic(nil), given...), given[0])
	err := outer(extra)
	if err == nil {
		t.Fatal("a warning repeated once more than the decode gives was accepted")
	}
	if err.Code() != RefusalValueHasNoForm {
		t.Errorf("a warning given that the bytes do not carry: %s", err.Code().ID())
	}
	if err := outer(nil); err == nil || err.Code() != RefusalDoesNotReadBack {
		t.Errorf("no warnings given where the decode has two: %v", err)
	}
}

// The header written is read back with no diagnostic the held packet does not carry: a decoded
// packet that holds the empty-destination warning is written empty, and the same packet without
// it is refused.
func TestHeaderDiagnosticsAreReadBack(t *testing.T) {
	p, err := DecodeTNC2([]byte("N0CALL>:>hi"), Lenient)
	if err != nil {
		t.Fatal(err)
	}
	if line, err := p.TNC2Line(); err != nil || string(line) != "N0CALL>:>hi" {
		t.Fatalf("%q, %v", line, err)
	}
	held := *p
	held.Diagnostics = nil
	if _, err := held.TNC2Line(); err == nil || err.(*EncodeError).Code() != RefusalInvalidAddress {
		t.Errorf("an empty destination the packet does not carry: %v", err)
	}
	// The read-back itself, handed a decode whose diagnostic the packet lacks.
	back, herr := decodeTNC2Line([]byte("N0CALL>:"), Lenient, 0)
	if herr != nil {
		t.Fatal(herr)
	}
	if err := checkHeaderBack(&held, back, nil); err == nil {
		t.Error("a header that reads back with a diagnostic the packet lacks was accepted")
	}
	if err := checkHeaderBack(p, back, nil); err != nil {
		t.Errorf("a diagnostic the packet carries was refused: %v", err)
	}
}
