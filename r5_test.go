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

// Round 5: a garbled timestamp before an uncompressed position. When the seventh byte after the
// DTI is z, / or h and an uncompressed position follows the seven bytes, the position is read
// there, whatever the bytes straight after the DTI read as.
func decodeInfo(info string, o aprs.ParseOptions) *aprs.Packet {
	return aprs.Decode(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), nil, []byte(info), o)
}

func hasCode(p *aprs.Packet, code aprs.Code) bool {
	for _, d := range p.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestGarbledTimestampBeforeAnUncompressedPositionReadsAfterTheSevenBytes(t *testing.T) {
	for _, dti := range []string{"@", "/"} {
		for _, ind := range []string{"z", "/", "h"} {
			// R30549z2831.0 would read as a compressed position straight after the DTI.
			info := dti + "R30549" + ind + "2831.07N/08142.92W_015/001g006t058"
			p := decodeInfo(info, aprs.Lenient)
			r, ok := p.Data.(aprs.PositionReport)
			if !ok {
				t.Fatalf("%q: %T", info, p.Data)
			}
			if r.Fields.Compressed || r.Timestamp != nil || r.Messaging != (dti == "@") {
				t.Errorf("%q: compressed %v, timestamp %v, messaging %v", info, r.Fields.Compressed, r.Timestamp, r.Messaging)
			}
			if got := r.Fields.Position.Latitude; got < 28.51 || got > 28.52 {
				t.Errorf("%q: latitude %v", info, got)
			}
			if !hasCode(p, aprs.CodeMalformedTimestamp) || p.HasErrors() {
				t.Errorf("%q: %v", info, p.Diagnostics)
			}
			// Strict rejects it by the malformed timestamp.
			s := decodeInfo(info, aprs.Strict)
			if !s.HasErrors() || !hasCode(s, aprs.CodeMalformedTimestamp) {
				t.Errorf("%q strict: %v", info, s.Diagnostics)
			}
			if _, ok := s.Data.(aprs.PositionReport); ok {
				t.Errorf("%q strict: decoded", info)
			}
		}
	}
}

func TestGarbledTimestampRuleLeavesOtherShapesAlone(t *testing.T) {
	// A seventh byte that is not a time indicator: the compressed reading straight after the DTI stands.
	p := decodeInfo("@R30549x2831.07N/08142.92W_", aprs.Lenient)
	if r, ok := p.Data.(aprs.PositionReport); !ok || !r.Fields.Compressed {
		t.Errorf("seventh byte x: %v", p.Data)
	}
	// A compressed position after the seven bytes does not win.
	p = decodeInfo("@R30549z/5L!!<*e7>7P[", aprs.Lenient)
	if r, ok := p.Data.(aprs.PositionReport); !ok || !r.Fields.Compressed || r.Timestamp != nil {
		t.Errorf("compressed after seven: %v", p.Data)
	}
	// A short field cannot take the new branch.
	for _, info := range []string{"@R30549z", "@R30549z2", "/R30549h"} {
		decodeInfo(info, aprs.Lenient)
	}
}

func TestGarbledTimestampPositionIsJudgedOnThePositionAlone(t *testing.T) {
	// Lower-case hemisphere letters decode leniently, so the position after the seven bytes is read.
	p := decodeInfo("@R30549z2831.07n/08142.92w_015/001", aprs.Lenient)
	r, ok := p.Data.(aprs.PositionReport)
	if !ok || r.Fields.Compressed || r.Fields.Position.Latitude < 28 || r.Fields.Position.Latitude > 29 {
		t.Errorf("lower-case lenient: %v", p.Data)
	}
	if p := decodeInfo("@R30549z2831.07n/08142.92w_015/001", aprs.Strict); !p.HasErrors() {
		t.Errorf("lower-case strict: %v", p.Diagnostics)
	}
	// Position ambiguity in the following position.
	p = decodeInfo("/R30549h28  .  N/081  .  W_", aprs.Lenient)
	r, ok = p.Data.(aprs.PositionReport)
	if !ok || r.Fields.Compressed || r.Fields.Position.Ambiguity != 4 {
		t.Errorf("ambiguity lenient: %v %v", p.Data, p.Diagnostics)
	}
	if p := decodeInfo("/R30549h28  .  N/081  .  W_", aprs.Strict); !hasCode(p, aprs.CodeMalformedTimestamp) {
		t.Errorf("ambiguity strict: %v", p.Diagnostics)
	}
	// A position after the seven bytes that does not decode (longitude hemisphere X): as before.
	p = decodeInfo("@R30549z2831.07N/08142.92X_", aprs.Lenient)
	if r, ok := p.Data.(aprs.PositionReport); ok && !r.Fields.Compressed {
		t.Errorf("not a position: %v", p.Data)
	}
}

// Round 5: a Mic-E locator's space is judged on the decoder's own result, the text left once the
// altitude, base-91 telemetry and !DAO! are lifted as for the data, and that space separates the
// locator from the comment (vectors README, Mic-E).
func TestMicELocatorSpaceIsJudgedOnWhatTheDecoderLifts(t *testing.T) {
	const head = "`(_fn\"Oj/`IO91/G"
	tests := []struct {
		name, tail, comment string
		missingSpace        bool
	}{
		// A !DAO! that exists only by joining across a lifted altitude is text, so the space is missing.
		{"dao joined across an altitude", "!W1\"4e}2! hi", "!W12! hi", true},
		{"lower-case dao joined across an altitude", "!w1\"4e}2!x", "!w12!x", true},
		{"dao joined, no comment after", "!W3\"4e}4!", "!W34!", true},
		// Two spaces after the locator: one separates, one is the comment's delimiter.
		{"two spaces then a later altitude", "\"4e}  y", "y", false},
		{"three spaces then a later altitude", "\"4e}   y", " y", false},
		{"two spaces, then an altitude, then text", "  \"4e}z", "z", false},
		// Telemetry an altitude splits is text, so nothing after it has its space.
		{"two spaces after telemetry split by an altitude", "|!:&0hl!Y[/>\"4e}='p|  y", "|!:&0hl!Y[/>='p|  y", true},
		{"one space after telemetry split by an altitude", "|!:&0hl!Y[/>\"4e}='p| z", "|!:&0hl!Y[/>='p| z", true},
		// Two spaces after whole telemetry that follows a later altitude.
		{"two spaces after telemetry after an altitude", "\"4e}|!:&0'p|  y", "y", false},
		// Two spaces after the locator and telemetry.
		{"two spaces after telemetry", "|!:&0'p|  y", "y", false},
		{"three spaces after telemetry", "|!:&0'p|   y", " y", false},
		{"two spaces after a dao", "!W12!  y", "y", false},
	}
	for _, tc := range tests {
		info := head + tc.tail
		p := aprs.Decode(aprs.MustAddress("N0CALL"), aprs.MustAddress("S32UVT"), nil, []byte(info), aprs.Lenient)
		m, ok := p.Data.(aprs.MicEReport)
		if !ok {
			t.Errorf("%s: %q is not Mic-E: %v", tc.name, info, p.Data)
			continue
		}
		if m.Fields.Comment != tc.comment {
			t.Errorf("%s: comment %q, want %q", tc.name, m.Fields.Comment, tc.comment)
		}
		if got := hasCode(p, aprs.CodeMissingSpaceAfterLocator); got != tc.missingSpace {
			t.Errorf("%s: missing-space-after-locator %v, want %v", tc.name, got, tc.missingSpace)
		}
	}
}
