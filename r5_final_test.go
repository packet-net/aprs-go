package aprs_test

import (
	"strings"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// The search for a later Mic-E altitude (three base-91 characters and }) never uses the bytes of
// a /A= altitude that stands whole as sent (round 5, final pass).
func TestLaterMicEAltitudeNeverUsesTheBytesOfAWholeAltitude(t *testing.T) {
	const head = "N0CALL>S32UVT:`(_fn\"Oj/`"
	tests := []struct {
		name, status, comment string
		feet                  float64
		warning               bool // the later Mic-E altitude is lifted, with its warning
	}{
		{"overlapping", "x/A=000100}", "x}", 100, false},
		{"overlapping a negative altitude", "x/A=-00100}", "x}", -100, false},
		{"altitude at the start of the text", "/A=000100}", "}", 100, false},
		{"clear of it, after", "x/A=000100}y\"4e}z", "x}yz", 100, true},
		{"clear of it, before", "y\"4e}x/A=000100}", "yx}", 100, true},
		{"overlapping twice, then clear", "/A=000100}/A=000200}\"4e}", "}/A=000200}", 100, true},
		{"leading altitude as well", "\"4e}x/A=000100}", "x}", 100, false},
	}
	for _, tc := range tests {
		for _, options := range []aprs.ParseOptions{aprs.Lenient, aprs.Strict} {
			t.Run(tc.name, func(t *testing.T) {
				p, err := aprs.DecodeTNC2([]byte(head+tc.status), options)
				if tc.warning && options == aprs.Strict {
					// A strict decoder takes no later altitude: the run is text.
					if err != nil {
						t.Fatal(err)
					}
					if f := fieldsOf(t, p); f.AltitudeFeet == nil || *f.AltitudeFeet != tc.feet {
						t.Errorf("strict altitude %v, want %v", f.AltitudeFeet, tc.feet)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				f := fieldsOf(t, p)
				if f.AltitudeFeet == nil || *f.AltitudeFeet != tc.feet {
					t.Errorf("altitude %v, want %v", f.AltitudeFeet, tc.feet)
				}
				eq(t, f.Comment, tc.comment)
				warned := false
				for _, d := range p.Diagnostics {
					warned = warned || d.Code == aprs.CodeMicEAltitudeNotFirst
				}
				eq(t, warned, tc.warning)
			})
		}
	}
}

// A /A= that is not whole protects nothing, and a Mic-E altitude clear of one is taken.
func TestLaterMicEAltitudeOverlappingAShortAltitudeIsTaken(t *testing.T) {
	for _, status := range []string{"x/A=00100}", "x/A=-0100}", "x/A=0001}"} {
		p, err := aprs.DecodeTNC2([]byte("N0CALL>S32UVT:`(_fn\"Oj/`"+status), aprs.Lenient)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range p.Diagnostics {
			found = found || d.Code == aprs.CodeMicEAltitudeNotFirst
		}
		if !found {
			t.Errorf("%q: no later Mic-E altitude was taken", status)
		}
	}
}

// A comment after a /A= in Mic-E status text is written straight on, and reads back.
func TestMicECommentAfterAnAltitudeNeedsNoSpace(t *testing.T) {
	for comment, want := range map[string]string{"x}": "/A=000100x}", "}": "/A=000100}", "a}b": "/A=000100a}b"} {
		heard, err := aprs.DecodeTNC2([]byte("N0CALL>S32UVT:`(_fn\"Oj/`/A=000100"+comment), aprs.Lenient)
		if err != nil {
			t.Fatal(err)
		}
		made, err := aprs.NewPacket(heard.Source, heard.Destination, heard.Path, heard.Data)
		if err != nil {
			t.Fatalf("%q: %v", comment, err)
		}
		if !strings.HasSuffix(string(made.Information), want) {
			t.Errorf("%q: wrote %q", comment, made.Information)
		}
	}
}

// Braces and a late extension are still found after a long run of near misses, and only within
// reach: the first well-formed {55} after many {, and a PHG after PHG1 runs.
func TestBracesAndLateExtensionAfterManyNearMisses(t *testing.T) {
	const pos = "N0CALL>APRS:!4903.50N\\07201.75Wm"
	p, err := aprs.DecodeTNC2([]byte(pos+strings.Repeat("{", 5000)+"{55}x"), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	f := fieldsOf(t, p)
	if f.Signpost == nil || *f.Signpost != "55" {
		t.Errorf("signpost %v", f.Signpost)
	}
	eq(t, f.Comment, strings.Repeat("{", 5000)+"x")
	// Braces with four characters between are not well formed; three are.
	for text, want := range map[string]string{"{abcd}": "", "{abc}": "abc", "{{a}": "a", "{a{b}": "b"} {
		p, err := aprs.DecodeTNC2([]byte(pos+text), aprs.Lenient)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if s := fieldsOf(t, p).Signpost; s != nil {
			got = *s
		}
		eq(t, got, want)
	}
	q, err := aprs.DecodeTNC2([]byte("N0CALL>APRS:!4903.50N/07201.75W-"+strings.Repeat("PHG1", 5000)+"PHG3330/x"), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	g := fieldsOf(t, q)
	if g.PHG == nil || g.PHG.Power != 3 {
		t.Errorf("phg %v", g.PHG)
	}
}
