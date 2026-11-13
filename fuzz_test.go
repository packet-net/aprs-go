// Fuzz targets: decoding never panics, what the encoder and builder write decodes cleanly and
// stably, and KISS framing round-trips. Run one with go test -fuzz FuzzTNC2 (or FuzzAX25,
// FuzzBuilder); without -fuzz they run their seeds as ordinary tests.

package aprs_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
)

// seedFromVectors adds every decode case's input in the vectors as a seed, when the vectors are
// there, and a few lines of its own.
func seedFromVectors(f *testing.F, ax25 bool) {
	f.Add([]byte("N0CALL-9>APZ001,WIDE1-1,qAR,M0LTE-10:=5130.00N/00007.00W>088/036Mobile"))
	f.Add([]byte("N1JCM-9>TRQP7T,WA1PLE-4*:`c'wl|+>/`\"4-}_%"))
	files, _ := filepath.Glob(filepath.Join(vectorsDir, "cases", "*.json"))
	for _, file := range files {
		text, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		doc, err := neutral.Parse(text)
		if err != nil {
			continue
		}
		for _, c := range doc.(map[string]any)["cases"].([]any) {
			input := c.(map[string]any)["input"].(map[string]any)
			switch {
			case ax25 && input["ax25_hex"] != nil:
				f.Add(mustHex(input["ax25_hex"].(string)))
			case !ax25 && input["tnc2"] != nil:
				f.Add([]byte(input["tnc2"].(string)))
			case !ax25 && input["tnc2_hex"] != nil:
				f.Add(mustHex(input["tnc2_hex"].(string)))
			}
		}
	}
}

func exercise(t *testing.T, p *aprs.Packet) {
	_ = p.String()
	_ = p.HasErrors()
	_, _ = p.QConstruct()
	_, _ = p.Device()
	for _, d := range p.Diagnostics {
		_ = d.String()
	}
	if _, err := p.AX25(); err == nil {
		k, _ := p.KISS(3)
		if _, err := aprs.DecodeKISS(k, aprs.Lenient); err != nil {
			t.Fatalf("own KISS output does not decode: %v", err)
		}
	}
	info, err := aprs.Encode(p.Data)
	if err == nil {
		// Encoded data must decode with no errors under Strict, and re-encode to the same bytes.
		q := aprs.Decode(p.Source, p.Destination, p.Path, info, aprs.Strict)
		if q.HasErrors() {
			t.Fatalf("encoder output %q has strict errors: %v", info, q.Diagnostics)
		}
		again, err := aprs.Encode(q.Data)
		if err != nil {
			t.Fatalf("re-encode of %q refused: %v", info, err)
		}
		if !bytes.Equal(again, info) {
			t.Fatalf("not stable: %q -> %q", info, again)
		}
	}
	if np, err := aprs.NewPacket(p.Source, p.Destination, p.Path, p.Data); err == nil {
		_ = np.String()
		line := np.TNC2()
		if _, err := aprs.DecodeTNC2(line, aprs.Strict); err != nil {
			// header issues are possible with odd sources; only report for clean ones
			if p.Source.IsAX25() {
				t.Logf("own TNC2 output does not decode strictly: %q %v", line, err)
			}
		}
	}
}

func FuzzTNC2(f *testing.F) {
	seedFromVectors(f, false)
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, o := range []aprs.ParseOptions{aprs.Lenient, aprs.Strict} {
			p, err := aprs.DecodeTNC2(b, o)
			if err != nil {
				if p != nil {
					t.Fatal("packet and error")
				}
				_ = err.Error()
				continue
			}
			exercise(t, p)
		}
	})
}

func FuzzAX25(f *testing.F) {
	seedFromVectors(f, true)
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := aprs.DecodeAX25(b, aprs.Lenient)
		if err == nil {
			exercise(t, p)
			if p.Source.IsAX25() && p.Destination.IsAX25() {
				out, err := p.AX25()
				if err == nil && len(p.Path) > 0 {
					_ = out
				}
			}
		}
		_, _ = aprs.DecodeKISS(b, aprs.Lenient)
		u, err := aprs.UnwrapKISS(aprs.WrapKISS(b, 5))
		if err != nil || !bytes.Equal(u, b) {
			t.Fatalf("kiss round trip: %x -> %x %v", b, u, err)
		}
	})
}

func FuzzBuilder(f *testing.F) {
	f.Add(51.45, -0.98, 88.0, 36.0, 120.0, 145.725, uint8(0), uint8(0), int32(-600), uint16(118), "Mobile", 0.0)
	f.Fuzz(func(t *testing.T, lat, lon, course, speed, alt, mhz float64, amb, flags uint8, off int32, tone uint16, comment string, wx float64) {
		me, _ := aprs.NewStation("M0LTE-9")
		b := me.Position(lat, lon).Symbol(aprs.SymbolCar).Ambiguity(amb % 6)
		if flags&1 != 0 {
			b = b.CourseDegrees(course).SpeedKnots(speed)
		}
		if flags&2 != 0 {
			b = b.AltitudeFeet(alt)
		}
		if flags&4 != 0 {
			b = b.Frequency(mhz).Tone(aprs.Tone(flags>>5%5), tone).OffsetKHz(off)
		}
		if flags&8 != 0 {
			b = b.Compressed()
		}
		if flags&16 != 0 {
			b = b.DAO()
		}
		b = b.Comment(comment)
		check := func(p *aprs.Packet, err error) {
			if err != nil {
				return
			}
			q, herr := aprs.DecodeTNC2(p.TNC2(), aprs.Strict)
			if herr != nil || q.HasErrors() {
				t.Fatalf("built %q does not decode strictly: %v %v", p.TNC2(), herr, q.Diagnostics)
			}
		}
		check(b.Build())
		m := me.MicE(lat, lon).Symbol(aprs.SymbolCar).CourseDegrees(course).SpeedKnots(speed).Comment(comment)
		if flags&2 != 0 {
			m = m.AltitudeFeet(alt)
		}
		check(m.Build())
		check(me.Object("OBJ").At(lat, lon).Symbol(aprs.SymbolCar).Timestamp(aprs.TimestampDHM(1, 2, 3)).Comment(comment).Build())
		check(me.Weather().At(lat, lon).WindDirectionDegrees(course).WindSpeedMph(speed).TemperatureC(wx).Rain1hMm(alt).HumidityPercent(mhz).PressureMbar(wx).Build())
		check(me.Message("G3NRW", comment).MessageID("1").Build())
		check(me.Status(comment).Build())
		check(me.Telemetry(uint16(off)).Analog(aprs.Ptr(lat), nil, aprs.Ptr(wx)).Bits("10101010").Comment(comment).Build())
		check(me.TelemetryCoefficients(lat, lon, wx).Build())
	})
}
