// Command aprs-diffdump is the dump tool for differential comparison with other implementations
// of the conformance vectors, as packet-net/aprs-vectors describes it in its README ("Comparing
// implementations"); its tools/compare.py compares any number of such dumps. Every file is
// gzip-compressed JSON lines, and the output keeps the input's order.
//
//   - decode (the default): reads a capture extracted as one hex-encoded TNC2 line per line, and
//     writes the lenient and strict results in the vectors' neutral form, what re-encoding gives
//     and the bytes it wrote (written, and written_destination for Mic-E), and the API view: what
//     the packet's own accessors say.
//   - -encode: reads data in the neutral form (tools/generate.py), encodes it, and writes the
//     bytes, or the refusal, and what they decode to.
//   - -build: reads builder recipes (tools/generate.py --recipes), builds each with the builder
//     as a program would, and writes the TNC2 line, or the refusal, and what it decodes to.
//
// Usage:
//
//	aprs-diffdump [-encode | -build] <in.gz> <out.jsonl.gz>
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
	"github.com/packet-net/aprs-go/internal/recipe"
)

type mode int

const (
	decodeMode mode = iota
	encodeMode
	buildMode
)

func main() {
	args := os.Args[1:]
	m := decodeMode
	if len(args) > 0 {
		switch args[0] {
		case "-encode", "--encode":
			m, args = encodeMode, args[1:]
		case "-build", "--build":
			m, args = buildMode, args[1:]
		}
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "aprs-diffdump [-encode | -build] <in.gz> <out.jsonl.gz>")
		os.Exit(2)
	}
	n, err := run(m, args[0], args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "aprs-diffdump:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d records\n", args[1], n)
}

func run(m mode, in, out string) (int, error) {
	f, err := os.Open(in)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	o, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	zw, _ := gzip.NewWriterLevel(o, gzip.BestSpeed)
	w := bufio.NewWriterSize(zw, 1<<20)

	r := bufio.NewReaderSize(gz, 1<<20)
	const batch = 2048
	workers := runtime.GOMAXPROCS(0)
	n := 0
	for {
		lines := make([]string, 0, batch)
		for len(lines) < batch {
			line, err := r.ReadString('\n')
			if len(line) > 0 {
				lines = append(lines, trimEOL(line))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return n, err
			}
		}
		if len(lines) == 0 {
			break
		}
		results := make([][]byte, len(lines))
		var wg sync.WaitGroup
		next := make(chan int, len(lines))
		for i := range lines {
			next <- i
		}
		close(next)
		for k := 0; k < workers; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range next {
					results[i] = record(m, n+i, lines[i])
				}
			}()
		}
		wg.Wait()
		for _, b := range results {
			w.Write(b)
			w.WriteByte('\n')
		}
		n += len(lines)
		if len(lines) < batch {
			break
		}
	}
	if err := w.Flush(); err != nil {
		return n, err
	}
	if err := zw.Close(); err != nil {
		return n, err
	}
	return n, o.Close()
}

func trimEOL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func record(m mode, n int, line string) []byte {
	var v map[string]any
	switch m {
	case decodeMode:
		v = decodeRecord(n, hexBytes(line))
	default:
		parsed, err := neutral.Parse([]byte(line))
		input, ok := parsed.(map[string]any)
		if err != nil || !ok {
			v = map[string]any{"n": int64(n), "result": "unsupported", "reason": "not a JSON object"}
		} else if m == encodeMode {
			v = encodeRecord(input)
		} else {
			v = buildRecord(input)
		}
	}
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func hexBytes(text string) []byte {
	out := make([]byte, 0, len(text)/2)
	for i := 0; i+1 < len(text); i += 2 {
		b, err := hex.DecodeString(text[i : i+2])
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, b[0])
	}
	return out
}

func toHex(b []byte) string {
	return hex.EncodeToString(b)
}

// ------------------------------------------------------------------ decode

func decodeRecord(n int, line []byte) map[string]any {
	lenient, packet := result(line, aprs.Lenient)
	strict, _ := result(line, aprs.Strict)
	how, info, destination, refusal := reencode(packet)
	r := map[string]any{"n": int64(n), "lenient": lenient, "strict": strict, "reencode": how}
	if refusal != nil {
		r["reencode_refusal"] = refusal.ID()
	}
	if info != nil {
		r["written"] = toHex(info)
		if destination != nil {
			r["written_destination"] = *destination
		}
	}
	if packet != nil {
		r["api"] = api(packet)
	}
	return r
}

// result gives the header, data and diagnostics, or the header error's diagnostics.
func result(line []byte, options aprs.ParseOptions) (map[string]any, *aprs.Packet) {
	p, err := aprs.DecodeTNC2(line, options)
	if err != nil {
		return map[string]any{"header_error": anyStrings(neutral.Diagnostics(err.(*aprs.HeaderError).Diagnostics))}, nil
	}
	return map[string]any{
		"header":      neutral.Header(p),
		"data":        neutral.Data(p.Data),
		"diagnostics": anyStrings(neutral.Diagnostics(p.Diagnostics)),
	}, p
}

func anyStrings(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// reencode says what encoding the lenient data again gives, as the vectors' reencode check sees
// it, and what it wrote: the information field and, for Mic-E, the destination.
func reencode(p *aprs.Packet) (string, []byte, *string, *aprs.RefusalCode) {
	if p == nil {
		return "none", nil, nil, nil
	}
	if _, ok := p.Data.(aprs.Unrecognized); ok {
		return "none", nil, nil, nil
	}
	var info []byte
	destination := p.Destination
	var err error
	m, isMicE := p.Data.(aprs.MicEReport)
	if isMicE {
		var created *aprs.Packet
		created, err = aprs.NewPacket(p.Source, p.Destination, p.Path, m)
		if err == nil {
			info, destination = created.Information, created.Destination
		}
	} else {
		info, err = aprs.Encode(p.Data)
	}
	if err != nil {
		code := err.(*aprs.EncodeError).Code()
		return "refused", nil, nil, &code
	}
	var wroteDestination *string
	if isMicE {
		s := destination.String()
		wroteDestination = &s
	}
	if info == nil {
		info = []byte{}
	}
	original := p.Information
	for len(original) > 0 && (original[len(original)-1] == '\r' || original[len(original)-1] == '\n') {
		original = original[:len(original)-1]
	}
	if bytes.Equal(info, original) && destination == p.Destination {
		return "identical", info, wroteDestination, nil
	}
	again := aprs.Decode(p.Source, destination, p.Path, info, aprs.Lenient)
	var differences []string
	neutral.Differences(any(neutral.Data(p.Data)), any(neutral.Data(again.Data)), "", &differences)
	how := "fails"
	if len(differences) == 0 && !again.HasWarnings() && !again.HasErrors() {
		how = "equivalent"
	}
	return how, info, wroteDestination, nil
}

// api is the API view: what the packet's own public accessors say, as a program would read them.
func api(p *aprs.Packet) map[string]any {
	o := map[string]any{
		"source":       p.Source.String(),
		"destination":  p.Destination.String(),
		"third_party":  p.ThirdParty,
		"has_errors":   p.HasErrors(),
		"has_warnings": p.HasWarnings(),
		"tnc2":         toHex(p.TNC2()),
	}
	path := make([]any, len(p.Path))
	for i, e := range p.Path {
		s := e.Address.String()
		if e.Used {
			s += "*"
		}
		path[i] = s
	}
	o["path"] = path
	if q, ok := p.QConstruct(); ok {
		qo := map[string]any{"construct": q.Construct}
		if q.Station != nil {
			qo["station"] = q.Station.String()
		}
		o["q_construct"] = qo
	} else {
		o["q_construct"] = nil
	}
	if frame, err := p.AX25(); err != nil {
		o["ax25"] = "refused"
		o["ax25_refusal"] = err.(*aprs.EncodeError).Code().ID()
	} else {
		o["ax25"] = toHex(frame)
	}
	if d, ok := p.Device(); ok {
		named := map[string]any{}
		for _, kv := range [][2]string{{"vendor", d.Vendor}, {"model", d.Model}, {"class", d.Class}} {
			if kv[1] != "" {
				named[kv[0]] = kv[1]
			}
		}
		o["device"] = named
	} else {
		o["device"] = nil
	}
	var fields *aprs.Positioned
	var symbol *aprs.Symbol
	switch d := p.Data.(type) {
	case aprs.PositionReport:
		fields = &d.Fields
	case aprs.MicEReport:
		fields = &d.Fields
	case aprs.ObjectReport:
		fields = &d.Fields
	case aprs.ItemReport:
		fields = &d.Fields
	case aprs.Status:
		symbol = d.Symbol
	}
	if fields != nil {
		symbol = &fields.Symbol
	}
	if symbol != nil {
		if description, ok := symbol.Description(); ok {
			o["symbol"] = map[string]any{"description": description}
		}
	}
	if fields != nil && fields.PHG != nil {
		phg := fields.PHG
		var directivity any
		if d, ok := phg.DirectivityDegrees(); ok {
			directivity = int64(d)
		}
		o["phg"] = map[string]any{
			"watts":               int64(phg.PowerWatts()),
			"height_feet":         phg.HeightFeet(),
			"gain_db":             int64(phg.GainDB()),
			"directivity_degrees": directivity,
		}
	}
	if tp, ok := p.Data.(aprs.ThirdParty); ok {
		o["inner"] = api(tp.Packet)
	}
	return o
}

// ------------------------------------------------------------------ encode

func encodeRecord(input map[string]any) map[string]any {
	n := input["n"]
	data, pending, unreadable := neutral.ReadDataStandIn(input["data"])
	if unreadable != nil {
		if unreadable.Refused {
			return refusedRecord(n, unreadable.Reason, unreadable.Code, unreadable.Tool)
		}
		return map[string]any{"n": n, "result": "unsupported", "reason": unreadable.Reason}
	}
	source, destination := aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001")
	var info []byte
	var err error
	if m, ok := data.(aprs.MicEReport); ok {
		var p *aprs.Packet
		p, err = aprs.NewPacket(source, destination, nil, m)
		if err == nil {
			info, destination = p.Information, p.Destination
		}
	} else {
		info, err = aprs.Encode(data)
	}
	if err != nil || pending != nil {
		var code *aprs.RefusalCode
		reason := ""
		if err != nil {
			e := err.(*aprs.EncodeError)
			c := e.Code()
			code, reason = &c, e.Error()
		}
		settled, why, tool := neutral.Settle(pending, code, reason)
		return refusedRecord(n, why, settled, tool)
	}
	// The bytes decoded leniently under N0CALL> and the destination.
	p := aprs.Decode(source, destination, nil, info, aprs.Lenient)
	again := map[string]any{"data": neutral.Data(p.Data), "diagnostics": anyStrings(neutral.Diagnostics(p.Diagnostics))}
	return map[string]any{"n": n, "result": "written", "info": toHex(info), "destination": destination.String(), "again": again}
}

// refusedRecord is a refused dump record; "via" says "tool" when the dump tool's reader made the
// refusal (an input this package cannot hold whose value alone decides the code), not the library.
func refusedRecord(n any, reason string, code aprs.RefusalCode, tool bool) map[string]any {
	r := map[string]any{"n": n, "result": "refused", "reason": reason, "refusal": code.ID()}
	if tool {
		r["via"] = "tool"
	}
	return r
}

// ------------------------------------------------------------------ build

// buildRecord builds a recipe with the shared builder mapping (internal/recipe), which the
// vectors test runner uses too, and writes the dump record.
func buildRecord(input map[string]any) map[string]any {
	n := input["n"]
	o := recipe.Build(input)
	switch {
	case o.Unsupported != "":
		return map[string]any{"n": n, "result": "unsupported", "reason": o.Unsupported}
	case o.Refusal != "":
		r := map[string]any{"n": n, "result": "refused", "reason": o.Reason, "refusal": o.Refusal}
		if o.Tool {
			r["via"] = "tool"
		}
		return r
	}
	line := o.Packet.TNC2()
	decoded, _ := result(line, aprs.Lenient)
	return map[string]any{"n": n, "result": "built", "tnc2": toHex(line), "again": decoded}
}
