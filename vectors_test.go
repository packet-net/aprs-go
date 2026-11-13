package aprs_test

// Runs the language-neutral conformance vectors (the vectors submodule, packet-net/aprs-vectors)
// against this package. Every case becomes one subtest per check, named <check>::<case id>:
//
//   - lenient: decoding with every tolerance on gives the case's expect;
//   - strict: decoding with none gives its strict result;
//   - tolerance: turning off only the tolerance behind a single tolerated defect gives the strict
//     result;
//   - reencode: encoding the decoded data again gives what reencode says, and writes exactly the
//     case's canonical_info when it has one;
//   - encode: an encode case writes the expected information field, or refuses with the expected
//     reason;
//   - readback: the neutral form of decoded data reads back into this package's types unchanged.
//
// Checks listed in testdata/known-differences.txt are skipped, each with its reason.

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
)

const vectorsDir = "vectors"

type vectorCase = map[string]any

func loadCases(t *testing.T) []vectorCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(vectorsDir, "cases", "*.json"))
	if err != nil || len(files) == 0 {
		// A module download has no submodule; a clone needs 'git submodule update --init vectors'.
		t.Skipf("%s/cases is missing: run 'git submodule update --init vectors'", vectorsDir)
	}
	sort.Strings(files)
	var cases []vectorCase
	for _, f := range files {
		text, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := neutral.Parse(text)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, c := range doc.(map[string]any)["cases"].([]any) {
			cases = append(cases, c.(map[string]any))
		}
	}
	return cases
}

func knownDifferences(t *testing.T) map[string]string {
	t.Helper()
	known := map[string]string{}
	f, err := os.Open(filepath.Join("testdata", "known-differences.txt"))
	if err != nil {
		return known
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, reason, _ := strings.Cut(line, " ")
		known[name] = strings.TrimSpace(reason)
	}
	return known
}

func TestVectors(t *testing.T) {
	known := knownDifferences(t)
	cases := loadCases(t)
	ran := 0
	for _, c := range cases {
		id := c["id"].(string)
		input := c["input"].(map[string]any)
		run := func(check string, f func(vectorCase) []string) {
			name := check + "::" + id
			ran++
			t.Run(name, func(t *testing.T) {
				if reason, ok := known[name]; ok {
					t.Skip(reason)
				}
				if problems := f(c); len(problems) > 0 {
					t.Error(strings.Join(problems, "\n"))
				}
			})
		}
		if _, ok := input["encode"]; ok {
			run("encode", encodeCheck)
			continue
		}
		run("lenient", lenientCheck)
		run("strict", strictCheck)
		if _, ok := c["strict"].(map[string]any); ok {
			if _, ok := toleranceFor(c); ok {
				run("tolerance", toleranceCheck)
			}
		}
		if _, ok := c["reencode"]; ok {
			run("reencode", reencodeCheck)
		}
		run("readback", readbackCheck)
	}
	t.Logf("%d cases, %d checks", len(cases), ran)
}

// ------------------------------------------------------------------ decoding

type decoded struct {
	packet *aprs.Packet
	header *aprs.HeaderError
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func textOr(m map[string]any, key, otherwise string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return otherwise
}

func decodeInput(input map[string]any, options aprs.ParseOptions) decoded {
	wrap := func(p *aprs.Packet, err error) decoded {
		if err != nil {
			return decoded{header: err.(*aprs.HeaderError)}
		}
		return decoded{packet: p}
	}
	if line, ok := input["tnc2"].(string); ok {
		return wrap(aprs.DecodeTNC2([]byte(line), options))
	}
	if line, ok := input["tnc2_hex"].(string); ok {
		return wrap(aprs.DecodeTNC2(mustHex(line), options))
	}
	if frame, ok := input["ax25_hex"].(string); ok {
		return wrap(aprs.DecodeAX25(mustHex(frame), options))
	}
	var info []byte
	if text, ok := input["info"].(string); ok {
		info = []byte(text)
	} else {
		info = mustHex(input["info_hex"].(string))
	}
	var path []aprs.PathEntry
	if p, ok := input["path"].([]any); ok {
		for _, e := range p {
			entry, ok := neutral.ReadPathEntry(e.(string))
			if !ok {
				panic("a bad path entry in a case: " + e.(string))
			}
			path = append(path, entry)
		}
	}
	source := aprs.MustAddress(textOr(input, "source", "N0CALL"))
	destination := aprs.MustAddress(textOr(input, "destination", "APZ001"))
	return decoded{packet: aprs.Decode(source, destination, path, info, options)}
}

// ------------------------------------------------------------------ comparing

func diagnosticDifferences(expected any, actual []string) []string {
	remaining := append([]string(nil), actual...)
	var missing, out []string
	list, _ := expected.([]any)
	for _, e := range list {
		s := e.(string)
		found := false
		for i, a := range remaining {
			if a == s {
				remaining = append(remaining[:i], remaining[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf("diagnostics: expected %q, not reported", missing))
	}
	if len(remaining) > 0 {
		out = append(out, fmt.Sprintf("diagnostics: unexpected %q", remaining))
	}
	return out
}

func describe(d decoded) string {
	if d.header != nil {
		return fmt.Sprintf("header error %q", neutral.Diagnostics(d.header.Diagnostics))
	}
	return fmt.Sprintf("%s with %q", neutral.Show(neutral.Data(d.packet.Data)), neutral.Diagnostics(d.packet.Diagnostics))
}

func check(expect map[string]any, d decoded) []string {
	var out []string
	if headerError, ok := expect["header_error"]; ok {
		if d.header == nil {
			return []string{"expected a header error, decoded " + neutral.Show(neutral.Data(d.packet.Data))}
		}
		return diagnosticDifferences(headerError, neutral.Diagnostics(d.header.Diagnostics))
	}
	if d.header != nil {
		return []string{fmt.Sprintf("header error: %q", neutral.Diagnostics(d.header.Diagnostics))}
	}
	p := d.packet
	neutral.Differences(expect["data"], any(neutral.Data(p.Data)), "data", &out)
	out = append(out, diagnosticDifferences(expect["diagnostics"], neutral.Diagnostics(p.Diagnostics))...)
	if header, ok := expect["header"]; ok {
		neutral.Differences(header, any(neutral.Header(p)), "header", &out)
	}
	if device, ok := expect["device"]; ok {
		found := map[string]any{}
		if dev, ok := p.Device(); ok {
			if dev.Vendor != "" {
				found["vendor"] = dev.Vendor
			}
			if dev.Model != "" {
				found["model"] = dev.Model
			}
		}
		neutral.Differences(device, any(found), "device", &out)
	}
	return out
}

func checkStrict(c vectorCase, d decoded) []string {
	switch s := c["strict"].(type) {
	case nil:
		return check(c["expect"].(map[string]any), d)
	case string:
		if s == "same" {
			return check(c["expect"].(map[string]any), d)
		}
	case map[string]any:
		if rejectedBy, ok := s["rejected_by"].(string); ok {
			want := "error:" + rejectedBy
			if header, _ := s["header"].(bool); header {
				if d.header != nil && containsString(neutral.Diagnostics(d.header.Diagnostics), want) {
					return nil
				}
				return []string{fmt.Sprintf("strict: expected the header to be rejected with %s, got %s", want, describe(d))}
			}
			if d.packet != nil && d.packet.Data == aprs.Data(aprs.UnrecognizedMalformed) && containsString(neutral.Diagnostics(d.packet.Diagnostics), want) {
				return nil
			}
			return []string{fmt.Sprintf("strict: expected rejection with %s, got %s", want, describe(d))}
		}
		return check(s, d)
	}
	return []string{fmt.Sprintf("strict: unknown expectation %s", neutral.Show(c["strict"]))}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// toleranceFor gives the one tolerance a case's lenient decoding used, if exactly one.
func toleranceFor(c vectorCase) (aprs.Code, bool) {
	seen := map[aprs.Code]bool{}
	expect := c["expect"].(map[string]any)
	list, _ := expect["diagnostics"].([]any)
	for _, d := range list {
		id, ok := strings.CutPrefix(d.(string), "warning:")
		if !ok {
			continue
		}
		if code, ok := aprs.CodeFromID(id); ok && code.Tolerable() {
			seen[code] = true
		}
	}
	if len(seen) != 1 {
		return 0, false
	}
	for code := range seen {
		return code, true
	}
	return 0, false
}

// ------------------------------------------------------------------ the checks

func lenientCheck(c vectorCase) []string {
	return check(c["expect"].(map[string]any), decodeInput(c["input"].(map[string]any), aprs.Lenient))
}

func strictCheck(c vectorCase) []string {
	return checkStrict(c, decodeInput(c["input"].(map[string]any), aprs.Strict))
}

func toleranceCheck(c vectorCase) []string {
	code, _ := toleranceFor(c)
	d := decodeInput(c["input"].(map[string]any), aprs.Lenient.Without(code))
	var out []string
	for _, p := range checkStrict(c, d) {
		out = append(out, fmt.Sprintf("with only %s not tolerated: %s", code, p))
	}
	return out
}

func show(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return neutral.Show(string(r))
}

// reencodeOf encodes a decoded packet's data again: the information field and, for Mic-E, the
// destination it computed.
func reencodeOf(p *aprs.Packet) ([]byte, aprs.Address, error) {
	if m, ok := p.Data.(aprs.MicEReport); ok {
		created, err := aprs.NewPacket(p.Source, p.Destination, p.Path, m)
		if err != nil {
			return nil, aprs.Address{}, err
		}
		return created.Information, created.Destination, nil
	}
	info, err := aprs.Encode(p.Data)
	return info, p.Destination, err
}

func trimLineBreaks(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\r' || b[len(b)-1] == '\n') {
		b = b[:len(b)-1]
	}
	return b
}

func reencodeCheck(c vectorCase) []string {
	d := decodeInput(c["input"].(map[string]any), aprs.Lenient)
	if d.header != nil {
		return []string{fmt.Sprintf("header error %q", neutral.Diagnostics(d.header.Diagnostics))}
	}
	p := d.packet
	expected := c["reencode"].(string)
	info, destination, err := reencodeOf(p)
	if err != nil {
		if expected != "refused" {
			return []string{fmt.Sprintf("reencode: expected %s, but the encoder refused: %v", expected, err)}
		}
		if want, ok := c["reencode_refusal"].(string); ok {
			if got := err.(*aprs.EncodeError).Code().ID(); got != want {
				return []string{fmt.Sprintf("reencode: refused with %s, expected %s", got, want)}
			}
		}
		return nil
	}
	original := trimLineBreaks(p.Information)
	var out []string
	switch expected {
	case "refused":
		out = append(out, "reencode: expected the encoder to refuse, it wrote "+show(info))
	case "identical":
		if string(info) != string(original) {
			out = append(out, "reencode: expected the input back, got "+show(info))
		}
		if _, ok := p.Data.(aprs.MicEReport); ok && destination != p.Destination {
			out = append(out, fmt.Sprintf("reencode: expected the Mic-E destination %s back, got %s", p.Destination, destination))
		}
	case "equivalent", "rounded":
		// A rounded value reads back as the nearest step, so only the bytes are compared.
		if expected == "equivalent" {
			again := aprs.Decode(p.Source, destination, p.Path, info, aprs.Lenient)
			neutral.Differences(any(neutral.Data(p.Data)), any(neutral.Data(again.Data)), "reencode", &out)
			if again.HasErrors() || again.HasWarnings() {
				out = append(out, fmt.Sprintf("reencode: %s decodes with %q", show(info), neutral.Diagnostics(again.Diagnostics)))
			}
		}
		// The bytes the Encoding rule gives are binding: an encoder writes exactly those.
		canonical, ok := c["canonical_info"].(string)
		switch {
		case ok && string(info) != canonical:
			out = append(out, fmt.Sprintf("reencode: expected canonical_info %s, wrote %s", neutral.Show(canonical), show(info)))
		case !ok && expected == "rounded":
			out = append(out, "reencode: a rounded case needs its canonical_info")
		}
	default:
		out = append(out, "reencode: unknown expectation "+expected)
	}
	return out
}

func encodeCheck(c vectorCase) []string {
	input := c["input"].(map[string]any)
	expect := c["expect"].(map[string]any)
	wantRefused, _ := expect["refused"].(bool)
	data, unreadable := neutral.ReadData(input["encode"])
	var p *aprs.Packet
	var refusal *aprs.RefusalCode
	var reason string
	switch {
	case unreadable != nil && !unreadable.Refused:
		// A record this package's types cannot hold is unsupported, not a failure.
		return nil
	case unreadable != nil:
		refusal, reason = &unreadable.Code, unreadable.Reason
	default:
		source := aprs.MustAddress(textOr(input, "source", "N0CALL"))
		destination := aprs.MustAddress(textOr(input, "destination", "APZ001"))
		created, err := aprs.NewPacket(source, destination, nil, data)
		if err != nil {
			code := err.(*aprs.EncodeError).Code()
			refusal, reason = &code, err.Error()
		}
		p = created
	}
	if refusal != nil {
		if !wantRefused {
			return []string{fmt.Sprintf("encode: expected %s, but the encoder refused: %s", neutral.Show(expect["info"]), reason)}
		}
		if want, ok := expect["refusal"].(string); ok && refusal.ID() != want {
			return []string{fmt.Sprintf("encode: refused with %s (%s), expected %s", refusal.ID(), reason, want)}
		}
		return nil
	}
	if wantRefused {
		return []string{"encode: expected a refusal, wrote " + show(p.Information)}
	}
	var out []string
	if string(p.Information) != expect["info"].(string) {
		out = append(out, fmt.Sprintf("encode: expected %s, wrote %s", neutral.Show(expect["info"]), show(p.Information)))
	}
	if d, ok := expect["destination"].(string); ok && d != p.Destination.String() {
		out = append(out, fmt.Sprintf("encode: expected destination %s, computed %s", d, p.Destination))
	}
	return out
}

func readbackCheck(c vectorCase) []string {
	d := decodeInput(c["input"].(map[string]any), aprs.Lenient)
	if d.header != nil {
		return nil
	}
	// Undecoded data has no neutral form to read back, at any depth of third-party packets.
	data0 := d.packet.Data
	for {
		tp, ok := data0.(aprs.ThirdParty)
		if !ok {
			break
		}
		data0 = tp.Packet.Data
	}
	if _, ok := data0.(aprs.Unrecognized); ok {
		return nil
	}
	written := neutral.Data(d.packet.Data)
	data, unreadable := neutral.ReadData(any(written))
	if unreadable != nil {
		// A third-party packet keeps its inner information field, which the neutral form does not
		// carry, so one whose inner data the encoder refuses cannot be made from it.
		if unreadable.Refused {
			return nil
		}
		return []string{"readback: " + unreadable.Reason}
	}
	var out []string
	neutral.Differences(any(written), any(neutral.Data(data)), "readback", &out)
	return out
}
