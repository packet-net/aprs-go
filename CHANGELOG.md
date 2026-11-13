# Changelog

## Unreleased

Round 3 of the conformance vectors (pinned at e0e8e39).

- A decoder reads at most four levels of third-party header; a fifth `}` makes the packet holding it `unrecognized` with `error:invalid-third-party`.
- An AX.25 address holding anything but upper-case letters and digits (a lower-case letter is still tolerated) is `invalid-address`, and a call of spaces only is an empty source (`invalid-address`), destination (`empty-destination`) or digipeater (`empty-path-entry`).
- An ambiguous position is cut to its box, not rounded, so a latitude just under 90 degrees is written in the box below the edge, in an uncompressed report and in Mic-E.
- A wind direction in a compressed weather report is judged in whole degrees before it is rounded to the 4-degree step: 360.5 and -0.5 are `value-out-of-range`.
- More than five analog values in a telemetry report or comment telemetry are `too-long`; a status grid locator that is not upper case is `malformed`; a PHG, RNG or DFS that loses to another extension in the one data extension keeps its own range checks.

## 1.0.1 (2026-11-13)

First Go release. It matches the conformance vectors at 1.0.1: every case passes, and it writes the same bytes as the other five implementations (C#, Rust, Python, TypeScript and C) over the vectors' cases and over differential runs of generated data, builder recipes and fuzzed packets.

- Decoding from TNC2 / APRS-IS text, AX.25 frames and KISS frames, every APRS 1.2 data type, with lenient, strict or per-code options. The zero `ParseOptions` is `Lenient`.
- Encoding and the station builder, with read-back checks and the 26 shared refusal codes (`EncodeError.Code()`). `Encode` and `NewPacket` take a data value or a pointer to one.
- TNC2, AX.25 and KISS output, device identification from the APRS device identification database, and named symbols.
- `cmd/aprs-diffdump`, the dump tool for the vectors' `tools/compare.py`.

The version starts at 1.0.1 to share its major and minor version with the vectors and the other implementations.
