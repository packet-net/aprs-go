package aprs_test

import (
	"testing"

	aprs "github.com/packet-net/aprs-go"
)

// Lifting an element never joins the text either side of it: variations on the shared cases, with
// other pairs of elements, in both orders, in each kind of comment.

func fieldsOf(t *testing.T, p *aprs.Packet) aprs.Positioned {
	t.Helper()
	switch d := p.Data.(type) {
	case aprs.PositionReport:
		return d.Fields
	case aprs.ObjectReport:
		return d.Fields
	case aprs.MicEReport:
		return d.Fields
	}
	t.Fatalf("not a report with a position: %T %v", p.Data, p.Diagnostics)
	return aprs.Positioned{}
}

func TestLiftingNeverJoinsTheTextEitherSide(t *testing.T) {
	const pos = "!4903.50N/07201.75W>"
	const mice = "`(_fn\"Oj/`"
	tests := []struct {
		name, info string
		dest       string
		comment    string
		check      func(t *testing.T, f aprs.Positioned)
	}{
		{"altitude split by telemetry", pos + "x/A=00|!!!!|0123", "APZ001", "x/A=000123", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.AltitudeFeet == nil, true)
			eq(t, f.Telemetry != nil, true)
		}},
		{"phg split by an altitude", pos + "PH/A=000100G3330 x", "APZ001", "PHG3330 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.PHG == nil, true)
			eq(t, *f.AltitudeFeet, 100.0)
		}},
		{"dfs split by a dao", pos + "DFS2!W12!360", "APZ001", "DFS2360", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.DFS == nil, true)
			eq(t, f.DAO != nil, true)
		}},
		{"dao split by an altitude", pos + "!W/A=000100 12!", "APZ001", "!W 12!", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.DAO == nil, true)
			eq(t, *f.AltitudeFeet, 100.0)
		}},
		{"offset after a dao after a tone", pos + "146.520MHz T100!W12! -060 x", "APZ001", "-060 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.ToneValue != nil, true)
			eq(t, f.Frequency.OffsetKHz == nil, true)
		}},
		{"offset split by an altitude", pos + "146.520MHz T100 -0/A=00010060 x", "APZ001", "-060 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.OffsetKHz == nil, true)
			eq(t, *f.AltitudeFeet, 100.0)
		}},
		{"offset after an altitude and a tone", pos + "146.520MHz T100/A=000100 -060 x", "APZ001", "x", func(t *testing.T, f aprs.Positioned) {
			eq(t, *f.Frequency.OffsetKHz, int32(-600))
		}},
		{"tone after a space and a dao", pos + "146.520MHz !W12!T100 x", "APZ001", "T100 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Tone == nil, true)
		}},
		{"tone after a space and telemetry", pos + "146.520MHz |!!!!|T100 x", "APZ001", "T100 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Tone == nil, true)
		}},
		{"offset after a space and a dao", pos + "146.520MHz T100 !W12!-060 x", "APZ001", "-060 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Tone != nil, true)
			eq(t, f.Frequency.OffsetKHz == nil, true)
		}},
		{"range after a space and a late phg", pos + "146.520MHz T100 -060 PHG3330R25m", "APZ001", "R25m", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Range == nil, true)
		}},
		{"tone with its space before an altitude", pos + "146.520MHz /A=000100T100 x", "APZ001", "T100 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Tone == nil, true)
			eq(t, *f.AltitudeFeet, 100.0)
		}},
		{"offset with its space before an altitude", pos + "146.520MHz T100 /A=000100-060 x", "APZ001", "-060 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Tone != nil, true)
			eq(t, f.Frequency.OffsetKHz == nil, true)
		}},
		{"signpost braces between a space and a tone", "@092345z4903.50N\\07201.75Wm146.520MHz {55}T100 x", "APZ001", "T100 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency.Tone == nil, true)
		}},
		{"frequency after a phg lifted from its start", pos + "PHG3330146.520MHz x", "APZ001", "x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency != nil, true)
		}},
		{"frequency split by telemetry", pos + "146.5|!!!!|20MHz x", "APZ001", "146.520MHz x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Frequency == nil, true)
		}},
		{"signpost split by telemetry", "@092345z4903.50N\\07201.75Wm{5|!!!!|5} x", "APZ001", "{55} x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Signpost == nil, true)
		}},
		{"software and unit split by telemetry in weather", pos[:19] + "_220/004g005t077X|!!!!|yz", "APZ001", "Xyz", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.Weather.Software == nil, true)
		}},
		{"mic-e phg split by a later altitude", mice + "PHG3\"4e}330 x", "S32UVT", "PHG3330 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.PHG == nil, true)
			eq(t, f.AltitudeFeet != nil, true)
		}},
		{"mic-e rng split by telemetry", mice + "RNG0|!!!!|011 x", "S32UVT", "RNG0011 x", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.RangeMiles == nil, true)
			eq(t, f.Telemetry != nil, true)
		}},
		{"mic-e dao split by a later altitude", mice + "!W1\"4e}2! hi", "S32UVT", "!W12! hi", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.DAO == nil, true)
		}},
		{"mic-e late phg split by a dao", mice + "x PH!W12!G3330", "S32UVT", "x PHG3330", func(t *testing.T, f aprs.Positioned) {
			eq(t, f.PHG == nil, true)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := aprs.Decode(aprs.MustAddress("N0CALL"), aprs.MustAddress(tc.dest), nil, []byte(tc.info), aprs.Lenient)
			f := fieldsOf(t, p)
			eq(t, f.Comment, tc.comment)
			tc.check(t, f)
		})
	}
}

// A strict decoder reads a later Mic-E altitude as text, and never rejects by it, whatever it
// would make with the bytes either side of a lifted place.
func TestStrictMicENeverRejectsByALaterAltitude(t *testing.T) {
	for _, status := range []string{
		"x \"4e} hi",
		"|!:&0hl!Y[/>\"4e}='p| hi",
		"!W1\"4e}2! hi",
		"x!W1\"4e}2!y|!!!!|",
	} {
		p := aprs.Decode(aprs.MustAddress("N0CALL"), aprs.MustAddress("S32UVT"), nil, []byte("`(_fn\"Oj/`"+status), aprs.Strict)
		if _, ok := p.Data.(aprs.MicEReport); !ok || hasCode(p, aprs.CodeMicEAltitudeNotFirst) {
			t.Errorf("%q: %T %v", status, p.Data, p.Diagnostics)
		}
	}
}

// Text left that would read back as an element has no clean form: it is refused.
func TestTextLeftThatReadsAsAnElementIsRefused(t *testing.T) {
	for _, info := range []string{
		"!4903.50N/07201.75W>x/A=00|!!!!|0123",
		"!4903.50N/07201.75W>PH!W12!G3330",
		"!4903.50N/07201.75W>RNG00|!!!!|11",
	} {
		p := aprs.Decode(aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001"), nil, []byte(info), aprs.Lenient)
		_, err := aprs.Encode(p.Data.(aprs.PositionReport))
		if err == nil {
			t.Errorf("%q: wrote it", info)
			continue
		}
		eq(t, refusal(t, err), aprs.RefusalTextReadsAsField)
	}
}
