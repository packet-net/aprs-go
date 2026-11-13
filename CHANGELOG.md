# Changelog

## Unreleased

Round 3 of the conformance vectors (pinned at cb5dcc0).

- A lower-case locator in a Mic-E report or a Maidenhead beacon is `malformed`, as in a status. Base-91 telemetry on a weather report is `field-not-allowed` and its own checks do not run. A course or speed beside a compressed weather report's wind keeps its own checks (a course of 0 is `value-has-no-form`). Builders refuse in the encoder's order: out of range comes before the ambiguity edge's `value-has-no-form`.

- A decoder reads at most four levels of third-party header; a fifth `}` makes the packet holding it `unrecognized` with `error:invalid-third-party`.
- An AX.25 address holding anything but upper-case letters and digits (a lower-case letter is still tolerated) is `invalid-address`, and a call of spaces only is an empty source (`invalid-address`), destination (`empty-destination`) or digipeater (`empty-path-entry`).
- An ambiguous position is cut to its box, not rounded, so a latitude just under 90 degrees is written in the box below the edge, in an uncompressed report and in Mic-E.
- A wind direction in a compressed weather report is judged in whole degrees before it is rounded to the 4-degree step: 360.5 and -0.5 are `value-out-of-range`.
- More than five analog values in a telemetry report or comment telemetry are `too-long`; a status grid locator that is not upper case is `malformed`; a field that is wanted twice (a PHG, RNG or DFS that loses the one data extension, or a course, speed or wind beside a range or a GGA altitude in the cs bytes) keeps its own range checks, so its refusal can outrank `extension-conflict`.
- An ambiguous position whose box would be centred past 90 degrees latitude or 180 degrees longitude is `value-has-no-form`; a reply-ack that is wrong on its own is `invalid-message-id` even with no message ID; undecoded data inside a third-party packet is named before a header fault in the dump tool's reader.
- `StatusBuilder.Locator` writes a lower-case locator in upper case, as before; the encoder, given a lower-case locator, refuses.

## 1.0.1 (2026-11-13)

First Go release. It matches the conformance vectors at 1.0.1: every case passes, and it writes the same bytes as the other five implementations (C#, Rust, Python, TypeScript and C) over the vectors' cases and over differential runs of generated data, builder recipes and fuzzed packets.

- Decoding from TNC2 / APRS-IS text, AX.25 frames and KISS frames, every APRS 1.2 data type, with lenient, strict or per-code options. The zero `ParseOptions` is `Lenient`.
- Encoding and the station builder, with read-back checks and the 26 shared refusal codes (`EncodeError.Code()`). `Encode` and `NewPacket` take a data value or a pointer to one.
- TNC2, AX.25 and KISS output, device identification from the APRS device identification database, and named symbols.
- `cmd/aprs-diffdump`, the dump tool for the vectors' `tools/compare.py`.

The version starts at 1.0.1 to share its major and minor version with the vectors and the other implementations.
