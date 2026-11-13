# aprs-go

An APRS (Automatic Packet Reporting System) encoder and decoder in Go. It covers every APRS 1.2 data type in [APRS12c](https://github.com/wb2osz/aprsspec), from TNC2 / APRS-IS text lines, AX.25 frames or KISS frames. Standard library only, no cgo, no `unsafe`.

Conforms to [aprs-vectors](https://github.com/packet-net/aprs-vectors) 1.0: every case passes, and it writes the same bytes as the [C#](https://github.com/packet-net/aprs-cs), [Rust](https://github.com/packet-net/aprs-rs), [Python](https://github.com/packet-net/aprs-py), [TypeScript](https://github.com/packet-net/aprs-ts) and [C](https://github.com/packet-net/aprs-c) implementations.

- **Decoding never fails because of the information field.** Every deviation from the spec becomes a `Diagnostic` with a stable `Code`, and the rest of the packet is still read where it can be.
- **Strict or lenient, one defect at a time.** `ParseOptions` says which of the 32 tolerable defects to accept. `aprs.Lenient` accepts them all, each with a warning; `aprs.Strict` accepts none; any single one can be switched off.
- **The encoder writes only what the spec allows.** It checks what it wrote by decoding it again: a comment that would read back as something else, such as an altitude or a `!DAO!`, is escaped or refused, never silently changed.

## Install

```sh
go get github.com/packet-net/aprs-go
```

```go
import aprs "github.com/packet-net/aprs-go"
```

## Decoding

```go
packet, err := aprs.DecodeTNC2([]byte("N0CALL-9>APZ001,WIDE1-1,qAR,M0LTE-10:=5130.00N/00007.00W>088/036Mobile"), aprs.Lenient)
if err != nil {
	return err // only an unusable header is an error
}
q, _ := packet.QConstruct()
fmt.Println(packet.Source, q.Construct, q.Station) // N0CALL-9 qAR M0LTE-10

report := packet.Data.(aprs.PositionReport)
fmt.Println(*report.Fields.CourseDegrees, *report.Fields.SpeedKnots, report.Fields.Comment) // 88 36 Mobile
```

`packet.Data` is one concrete type per APRS data type, so a type switch says what arrived:

```go
switch d := packet.Data.(type) {
case aprs.Message:
	fmt.Printf("to %s: %s\n", d.Addressee, d.Text)
case aprs.PositionReport:
	// ...
case aprs.Unrecognized:
	fmt.Println("nothing decoded:", packet.Diagnostics)
}
```

An optional value is a pointer, nil when absent; `aprs.Ptr(88.0)` makes one. Values are in the units APRS sends (knots, feet, mph, degrees Fahrenheit), with the unit in the field name.

`DecodeAX25` takes an AX.25 frame without flags or FCS, and `DecodeKISS` a frame as a KISS TNC sends it. `packet.TNC2()`, `packet.AX25()` and `packet.KISS(port)` write a packet back out.

## Strict and lenient

```go
line := []byte("N1EOE>APN391:!4216.95n/07243.20w#phg6230/ Easthampton MA")

lenient, _ := aprs.DecodeTNC2(line, aprs.Lenient) // read, with a lowercase-hemisphere warning
strict, _ := aprs.DecodeTNC2(line, aprs.Strict)   // UnrecognizedMalformed, with an error

// Or tolerate everything except this one defect.
picky, _ := aprs.DecodeTNC2(line, aprs.Lenient.Without(aprs.CodeLowercaseHemisphere))
```

## Building packets

Start from your station and say what the packet is. `Build()` gives the encoded `*Packet`, and `Data()` the data alone. A station and its builders are values, so keep one and build every packet from it.

```go
me, err := aprs.NewStation("M0LTE-9")
if err != nil {
	return err
}
if me, err = me.Via("WIDE1-1", "WIDE2-1"); err != nil {
	return err
}

beacon, err := me.Position(51.4543, -0.9781).Symbol(aprs.SymbolCar).CourseDegrees(88).SpeedKnots(36).AltitudeFeet(120).Comment("Mobile").Build()
// M0LTE-9>APZ001,WIDE1-1,WIDE2-1:!5127.26N/00058.69W>088/036/A=000120Mobile

message, err := me.Message("G3NRW", "Hi Ian").MessageID("01").Build()
// M0LTE-9>APZ001,WIDE1-1,WIDE2-1::G3NRW    :Hi Ian{01

repeater, err := me.Object("MYRPTR").At(51.45, -0.98).Symbol(aprs.SymbolRepeater).
	Timestamp(aprs.TimestampDHM(25, 18, 30)).Frequency(145.725).Tone(aprs.ToneTone, 118).OffsetKHz(-600).Build()
// M0LTE-9>APZ001,WIDE1-1,WIDE2-1:;MYRPTR   *251830z5127.00N/00058.80Wr145.725MHz T118 -060

weather, err := me.Weather().At(51.45, -0.98).WindDirectionDegrees(220).WindSpeedMph(4).WindGustMph(5).TemperatureF(77).Build()
// M0LTE-9>APZ001,WIDE1-1,WIDE2-1:!5127.00N/00058.80W_220/004g005t077
```

A setter has the name of the field it fills (`CourseDegrees`, `AltitudeFeet`), and one that converts changes only the unit (`AltitudeM`, `SpeedKmh`, `TemperatureC`, `Rain1hMm`). A value between the steps a field holds is rounded once, by the encoder, halves away from zero. A course of 0 is the spec's unknown course, and north is 360.

`Station` also starts items, Mic-E reports, acks and rejects, bulletins, status reports, telemetry and its `PARM.`/`UNIT.`/`EQNS.`/`BITS.` metadata. The destination is `APZ001`, from the experimental range, until you set your own tocall with `To`. The package has no clock, so an object or a positionless weather report needs its timestamp given.

## Encoding by hand

Fill in the types, then make a packet with `NewPacket`, or just the information field with `Encode`:

```go
info, err := aprs.Encode(aprs.Message{Addressee: "G3NRW", Text: "Hi Ian", MessageID: aprs.Ptr("01"), ReplyAck: aprs.Ptr("")})
// :G3NRW    :Hi Ian{01}
```

What the spec does not allow is refused with an `*EncodeError`. Its `Code()` says why, from the same 26 reasons the other implementations use (the `refusals` list in the vectors' `codes.json`); the message is for people and may change.

```go
_, err := aprs.Encode(aprs.Message{Addressee: "N0CALL", Text: strings.Repeat("x", 68)})
var refused *aprs.EncodeError
if errors.As(err, &refused) && refused.Code() == aprs.RefusalTooLong {
	// ...
}
```

New codes may be added in later 1.x releases, so give a switch over them a default case.

## Mic-E and device identification

Mic-E carries half its position in the destination address, so `NewPacket` works that address out (the destination you pass is not used), and `Encode` refuses a Mic-E report rather than drop half of it. `packet.Device()` names the sending device or program from the [APRS device identification database](https://github.com/aprsorg/aprs-deviceid).

```go
packet, _ := aprs.DecodeTNC2([]byte("N1JCM-9>TRQP7T,WA1PLE-4*:`c'wl|+>/`\"4-}_%"), aprs.Lenient)
device, _ := packet.Device() // Yaesu FTM-400DR

report := packet.Data.(aprs.MicEReport)
again, _ := aprs.NewPacket(packet.Source, packet.Destination, packet.Path, report) // the same bytes and destination
```

Every snippet here is a runnable example in `example_test.go`.

## Conformance

`go test ./...` runs the whole [aprs-vectors](https://github.com/packet-net/aprs-vectors) suite (a git submodule at `vectors/`; clone with `--recurse-submodules`, or run `git submodule update --init`). Each case is checked lenient, strict, with only its own tolerance switched off, re-encoded byte for byte, and read back. A deliberate difference would be listed, with its reason, in `testdata/known-differences.txt`; there are none.

`cmd/aprs-diffdump` is the dump tool the vectors' `tools/compare.py` reads. This package was compared with the other five over every vectors case, a million generated encode cases, a million builder recipes and two million fuzzed packets. It writes the same bytes as they do throughout; the only differences left were a handful of refusal codes on which the other five do not agree among themselves either.

## Licence

AGPL-3.0-or-later ([`LICENSE`](LICENSE)). It includes data from the APRS device identification database, CC BY-SA 2.0; see [`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md).
