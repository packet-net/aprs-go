package aprs_test

import (
	"strings"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

const micEInfo = "`(_fn\"Oj/"

// A held packet whose data and information field no longer agree is not written, whatever its
// type and by every writer: the field is read under the header written and compared with the data.
func TestHeldPacketThatNoLongerAgreesWithItsFieldIsNotWritten(t *testing.T) {
	refused := func(t *testing.T, name string, p *aprs.Packet) {
		t.Helper()
		if _, err := p.TNC2Line(); err == nil || refusal(t, err) != aprs.RefusalNotEncodable {
			t.Errorf("%s: TNC2Line: %v", name, err)
		}
		if p.TNC2() != nil {
			t.Errorf("%s: TNC2 wrote a line", name)
		}
		if _, err := p.AX25(); err == nil || refusal(t, err) != aprs.RefusalNotEncodable {
			t.Errorf("%s: AX25: %v", name, err)
		}
		if _, err := p.KISS(0); err == nil || refusal(t, err) != aprs.RefusalNotEncodable {
			t.Errorf("%s: KISS: %v", name, err)
		}
	}
	decode := func(line string) *aprs.Packet {
		p, err := aprs.DecodeTNC2([]byte(line), aprs.Lenient)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// The controls are written.
	for _, line := range []string{"N0CALL>APZ001:}X>S32U6T:" + micEInfo, "N0CALL>APZ001:!5130.00N/00030.00W>hi", "N0CALL>APZ001:>hi"} {
		if _, err := decode(line).TNC2Line(); err != nil {
			t.Errorf("control %q: %v", line, err)
		}
	}
	// An inner packet whose destination was changed, which a Mic-E field is read under.
	p := decode("N0CALL>APZ001:}X>S32U6T:" + micEInfo)
	p.Data.(aprs.ThirdParty).Packet.Destination = aprs.MustAddress("APZ001")
	refused(t, "inner destination at depth 1", p)
	// The same two levels down.
	p = decode("N0CALL>APZ001:}X>APZ001:}Y>S32U6T:" + micEInfo)
	p.Data.(aprs.ThirdParty).Packet.Data.(aprs.ThirdParty).Packet.Destination = aprs.MustAddress("APZ001")
	refused(t, "inner destination at depth 2", p)
	// A position whose latitude was changed in place, and by copy with changes.
	p = decode("N0CALL>APZ001:!5130.00N/00030.00W>hi")
	pos := p.Data.(aprs.PositionReport)
	pos.Fields.Position.Latitude = 52
	p.Data = pos
	refused(t, "latitude changed", p)
	// By a hair, below what a built packet's rounding would allow: a decoded one is exact.
	p = decode("N0CALL>APZ001:!5130.00N/00030.00W>hi")
	pos = p.Data.(aprs.PositionReport)
	pos.Fields.Position.Latitude += 0.000001
	p.Data = pos
	refused(t, "latitude changed by a hair", p)
	// Status text, and information bytes.
	p = decode("N0CALL>APZ001:>hi")
	p.Data = aprs.Status{Text: "bye"}
	refused(t, "status text", p)
	p = decode("N0CALL>APZ001:>hi")
	p.Information = []byte(">bye")
	refused(t, "information bytes", p)
	// A packet built from data is held as given: a latitude finer than the form is no fault.
	built, err := aprs.NewPacket(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), nil,
		aprs.PositionReport{Fields: aprs.Positioned{Position: aprs.Position{Latitude: 51.500001, Longitude: -0.5}, Symbol: aprs.NewSymbol('/', '>')}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := built.TNC2Line(); err != nil {
		t.Errorf("built packet: %v", err)
	}
}

// The destination given to NewPacket for a Mic-E report is the one worked out or it is none: it is
// never substituted.
func TestMicEDestinationGivenIsNeverSubstituted(t *testing.T) {
	d, err := aprs.DecodeTNC2([]byte("N0CALL>S32U6T:"+micEInfo), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	report := d.Data.(aprs.MicEReport)
	n0call := aprs.MustAddress("N0CALL")
	for _, given := range []string{"S32U7T", "APRS", "S32U6T-1"} {
		_, err := aprs.NewPacket(n0call, aprs.MustAddress(given), nil, report)
		if err == nil || refusal(t, err) != aprs.RefusalNotEncodable {
			t.Errorf("%s given: %v", given, err)
		}
	}
	for _, given := range []aprs.Address{{}, aprs.MustAddress(aprs.DefaultDestination), aprs.MustAddress("S32U6T")} {
		p, err := aprs.NewPacket(n0call, given, nil, report)
		if err != nil || p.Destination.String() != "S32U6T" {
			t.Errorf("%q given: %v", given.String(), err)
		}
	}
}

// A refusal for data that disagrees with the field says so; it names a Mic-E destination only for
// Mic-E data.
func TestHeldRefusalNamesItsCause(t *testing.T) {
	p, _ := aprs.DecodeTNC2([]byte("N0CALL>APZ001:>hi"), aprs.Lenient)
	p.Data = aprs.Status{Text: "bye"}
	_, err := p.TNC2Line()
	if err == nil || !strings.Contains(err.Error(), "information field") || strings.Contains(err.Error(), "Mic-E") {
		t.Errorf("status: %v", err)
	}
	p, _ = aprs.DecodeTNC2([]byte("N0CALL>S32U6T:"+micEInfo), aprs.Lenient)
	p.Destination = aprs.MustAddress("APZ001")
	_, err = p.TNC2Line()
	if err == nil || !strings.Contains(err.Error(), "do not agree") || !strings.Contains(err.Error(), "Mic-E") {
		t.Errorf("mic-e: %v", err)
	}
}
