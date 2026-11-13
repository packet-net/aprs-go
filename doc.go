// Package aprs is an APRS (Automatic Packet Reporting System) encoder and decoder: every APRS 1.2
// data type in APRS12c, from TNC2 / APRS-IS text lines, AX.25 frames or KISS frames.
//
// Decoding never fails because of the information field: every deviation from the spec becomes a
// Diagnostic with a stable Code, and ParseOptions says which tolerable defects to accept
// (Lenient accepts them all, each with a warning; Strict accepts none). Only an unusable header
// is an error, a *HeaderError.
//
// Encoding writes only what the spec allows, checking what it wrote by decoding it again. A
// refusal is an *EncodeError whose Code says why, from the reasons the conformance vectors list.
// Station starts a fluent builder for each kind of packet a program sends.
//
// The package conforms to the language-neutral conformance vectors at
// https://github.com/packet-net/aprs-vectors, and writes the same bytes as the C#, Rust,
// Python, TypeScript and C implementations there.
package aprs
