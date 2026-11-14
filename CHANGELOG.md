# Changelog

## Unreleased

Round 3 of the conformance vectors (pinned at 10d4e25).

### Behaviour changes

- A lower-case grid locator is refused as `malformed` by `Encode` in a status, a Mic-E report and a Maidenhead beacon (it used to be written in upper case). `StatusBuilder.Locator` still writes it in upper case.
- An AX.25 address holding anything but upper-case letters and digits (punctuation, a space before the last character, a control character) is now `invalid-address`; a lower-case letter is still tolerated.
- The decoder reads at most four levels of third-party header, and the encoder refuses a chain nested deeper with `invalid-third-party-header`.
- More than five analog values in a telemetry report or in comment telemetry are `too-long`.

### Changes

- Third-party packets nest at most four deep: a fifth `}` makes the packet holding it `unrecognized` with `error:invalid-third-party`.
- A call of spaces only in an AX.25 frame is an empty source (`invalid-address`), destination (`empty-destination`) or digipeater (`empty-path-entry`, dropped from the path).
- An ambiguous position is cut to its box, not rounded, in uncompressed and Mic-E reports, so a value just under 90 or 180 degrees is written.
- An ambiguous position whose box would be centred past 90 degrees latitude or 180 degrees longitude is `value-has-no-form`.
- A compressed wind direction is judged in whole degrees before it is rounded to the 4-degree step: 360.5 and -0.5 are `value-out-of-range`.
- A field wanted twice (a PHG, RNG or DFS that loses the one data extension, or a course, speed or wind beside a range or a GGA altitude in the cs bytes) keeps its own range checks, so its refusal can outrank `extension-conflict`.
- A course or speed beside a compressed weather report's wind keeps its own checks (a course of 0 is `value-has-no-form`).
- Base-91 telemetry on a weather report is `field-not-allowed` and its own checks do not run.
- A reply-ack that is wrong on its own is `invalid-message-id` even with no message ID.
- Builders refuse in the encoder's order: out of range comes before the ambiguity edge's `value-has-no-form`, and the telemetry builder passes extra analog values to the encoder.
- A compressed GGA weather report with an altitude of 1 foot or less is written, the nearest cs value alone, and reads back rounded, as for any altitude its cs bytes cannot hold exactly.
- A speed, course or other whole-number field given as NaN or an infinity is `not-a-number`, not `value-out-of-range`.
- The dump tool reads the `NaN`, `Infinity` and `-Infinity` tokens the generator writes, and ranks a recipe that gives a quantity in two units: it builds once with each unit and reports the first code among those results and `value-given-twice`, never `unsupported`.
- The dump tool's reader names undecoded inner data in a third-party packet before a header fault, and reads an absent positioned symbol as an empty one (`invalid-symbol`).

## 1.0.1 (2026-11-13)

First Go release. It matches the conformance vectors at 1.0.1: every case passes, and it writes the same bytes as the other five implementations (C#, Rust, Python, TypeScript and C) over the vectors' cases and over differential runs of generated data, builder recipes and fuzzed packets.

- Decoding from TNC2 / APRS-IS text, AX.25 frames and KISS frames, every APRS 1.2 data type, with lenient, strict or per-code options. The zero `ParseOptions` is `Lenient`.
- Encoding and the station builder, with read-back checks and the 26 shared refusal codes (`EncodeError.Code()`). `Encode` and `NewPacket` take a data value or a pointer to one.
- TNC2, AX.25 and KISS output, device identification from the APRS device identification database, and named symbols.
- `cmd/aprs-diffdump`, the dump tool for the vectors' `tools/compare.py`.

The version starts at 1.0.1 to share its major and minor version with the vectors and the other implementations.
