# Changelog

## 1.0.1 (2026-11-13)

First Go release. It matches the conformance vectors at 1.0.1: every case passes, and it writes the same bytes as the other five implementations (C#, Rust, Python, TypeScript and C) over the vectors' cases and over differential runs of generated data, builder recipes and fuzzed packets.

- Decoding from TNC2 / APRS-IS text, AX.25 frames and KISS frames, every APRS 1.2 data type, with lenient, strict or per-code options. The zero `ParseOptions` is `Lenient`.
- Encoding and the station builder, with read-back checks and the 26 shared refusal codes (`EncodeError.Code()`). `Encode` and `NewPacket` take a data value or a pointer to one.
- TNC2, AX.25 and KISS output, device identification from the APRS device identification database, and named symbols.
- `cmd/aprs-diffdump`, the dump tool for the vectors' `tools/compare.py`.

The version starts at 1.0.1 to share its major and minor version with the vectors and the other implementations.
