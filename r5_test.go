package aprs_test

import (
	"strings"
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// A Mic-E comment after a voice frequency follows a space; a / goes before it only where it
// would not read back as itself, which is text that would end the status text as a device suffix
// (round 4, run 5).
func TestMicECommentAfterAFrequencyTakesASlashOnlyWhereItReadsAsASuffix(t *testing.T) {
	build := func(b aprs.MicEBuilder) string {
		t.Helper()
		p, err := b.Build()
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		heard, err := aprs.DecodeTNC2(p.TNC2(), aprs.Lenient)
		if err != nil {
			t.Fatal(err)
		}
		report := heard.Data.(aprs.MicEReport)
		if report.DeviceSuffix != "" || report.Fields.Frequency == nil {
			t.Errorf("%q did not read back as a frequency and a comment", p.Information)
		}
		return string(p.Information[strings.Index(string(p.Information), "MHz")+3:])
	}
	base := func(comment string) aprs.MicEBuilder {
		return me(t).MicE(8.43, 44.11).Symbol(aprs.SymbolCar).Comment(comment).Frequency(144.39)
	}
	// X at the end would read as the device suffix " X".
	eq(t, build(base("X")), " /X")
	// Any other single character, or text with an X inside, takes the plain space.
	eq(t, build(base("Y")), " Y")
	eq(t, build(base("=")), " =")
	eq(t, build(base("Xray")), " Xray")
	eq(t, build(base("a X b")), " a X b")
	// With telemetry or a !DAO! after it, " X" reads back as itself.
	telemetry := aprs.CommentTelemetry{Sequence: 3, Analog: []uint16{10}}
	if got := build(base("X").Telemetry(telemetry)); !strings.HasPrefix(got, " X|") {
		t.Errorf("telemetry: %q", got)
	}
	if got := build(base("X").DAO()); !strings.HasPrefix(got, " X!") {
		t.Errorf("dao: %q", got)
	}
}

// Text that ends as a device suffix whatever comes before it is refused, not hidden behind a /.
func TestMicECommentThatEndsAsASuffixIsRefusedAfterAFrequency(t *testing.T) {
	for _, comment := range []string{"a X", "_#", " X"} {
		_, err := me(t).MicE(8.43, 44.11).Symbol(aprs.SymbolCar).Comment(comment).Frequency(144.39).Build()
		eq(t, refusal(t, err), aprs.RefusalTextReadsAsField)
	}
}

// A comment of X in a Mic-E report that has no frequency, a PHG or an altitude is written as it
// always was.
func TestMicECommentXAfterAPHGOrAnAltitudeNeedsNoSlash(t *testing.T) {
	phg := me(t).MicE(8.43, 44.11).Symbol(aprs.SymbolCar).Comment("X").PHG(aprs.PHG{Power: 2, Height: 3, Gain: 3, Directivity: 2})
	p, err := phg.Build()
	if err != nil {
		t.Fatal(err)
	}
	eq(t, strings.HasSuffix(string(p.Information), "PHG2332X"), true)
	heard, err := aprs.DecodeTNC2(p.TNC2(), aprs.Lenient)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, heard.Data.(aprs.MicEReport).Fields.Comment, "X")
	eq(t, heard.Data.(aprs.MicEReport).DeviceSuffix, "")
}

// The weather text of a positioned report ends where lifted base-91 telemetry or a !DAO! starts,
// wherever it sits: a run-on value that reaches it is dropped, and the text after it is comment
// text on its own (round 4, run 5).
func TestRunOnWeatherValueBeforeALiftedElementEndsTheWeatherText(t *testing.T) {
	for _, tc := range []struct {
		name, info, comment string
		telemetry, dao      bool
	}{
		{"extension, telemetry, text", "!4903.50N/07201.75W_220/004g005t2147483647|!!!!|7k SvxLink", "7k SvxLink", true, false},
		{"extension, dao, text", "!4903.50N/07201.75W_220/004g005t247483647!W12!12!", "12!", false, true},
		{"gust run-on, dao, text", "!4903.50N/07201.75W_220/004g0051234!W12!r000", "r000", false, true},
		{"humidity run-on, dao, text", "!4903.50N/07201.75W_220/004g005t050h12345!W12!b10236", "b10236", false, true},
		{"fields instead of extension", "!4903.50N/07201.75W_c220s004g005t2147483647|!!!!|7k", "7k", true, false},
		{"dao before telemetry", "!4903.50N/07201.75W_220/004g005t2147483647!W12!|!!!!|7k Text", "7k Text", true, true},
		{"compressed", "@252047z/c7NIs@rE_!!bg000t04600780|!)%Z'n|!r000p003P000h94b10236", "!r000p003P000h94b10236", true, false},
	} {
		line := "N0CALL>APZ001:" + tc.info
		heard, err := aprs.DecodeTNC2([]byte(line), aprs.Lenient)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		report, ok := heard.Data.(aprs.PositionReport)
		if !ok {
			t.Fatalf("%s: %T", tc.name, heard.Data)
		}
		eq(t, report.Fields.Comment, tc.comment)
		eq(t, report.Fields.Weather != nil, true)
		eq(t, report.Fields.Telemetry != nil, tc.telemetry)
		eq(t, report.Fields.DAO != nil, tc.dao)
		dropped := false
		for _, d := range heard.Diagnostics {
			dropped = dropped || d.Code == aprs.CodeOutOfRangeValue
		}
		eq(t, dropped, true)
		// Strict refuses the dropped run.
		if strict, err := aprs.DecodeTNC2([]byte(line), aprs.Strict); err == nil && !strict.HasErrors() {
			t.Errorf("%s: strict accepted it", tc.name)
		}
	}
}
