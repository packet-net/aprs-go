# Changelog

## Unreleased

- `Device` now has a `String` method giving "vendor model" (for example `Yaesu FTM-400DR`), the same as the Python library's `str(packet.device)`. Printing a `Device` used to show the raw struct.
- The README's Mic-E and device snippet is now a tested example (`Example_micEDevice`); every other README snippet already was.
- Tests: when the `vectors` submodule is missing in a git clone, the vectors tests now fail with a one-line message to run `git submodule update --init`, instead of skipping and reporting ok. A copy fetched through the module proxy (no `.git`, and the module zip never holds the submodule) still skips, with the same message.
- Round 4 of the conformance vectors (vectors submodule pinned at a405bac, branch `round-4`), release 1.2.0 to come. Licence: this library is MIT from this release; versions up to v1.1.0 were published under AGPL-3.0-or-later (the vectors are CC0).
- Round 4, run 5: the Mic-E builder writes free text after a voice frequency as ` /X` when ` X` at the end of the status text would read back as the device suffix, and not run on after the `MHz` (` X` stays plain before telemetry or a `!DAO!`); and the weather text of a positioned report ends where lifted base-91 telemetry or a `!DAO!` starts, wherever it sits, so a run-on value that reaches it is dropped (`out-of-range-value`) and the text after the element is comment text on its own, not joined to the run.
- A Mic-E locator followed by base-91 telemetry or a `!DAO!` and then a spaced comment (`` `IO91/G|!:&0'p| DG2MM ``) no longer gives `missing-space-after-locator`: the telemetry is lifted before the locator's space is looked for, so strict accepts it and the comment is `DG2MM`.
- An information field over 256 bytes is refused by every encoder and builder as `information-field-too-long` (new refusal code `RefusalInformationFieldTooLong`, last in the list; `too-long` now means only a field over its own limit); the size is the bytes the data's fields make, UTF-8 and number text included, and a third-party packet's limit is on the outermost field. The size is the last check, made on the bytes actually written after every other one (the read-back included): a record any other rule refuses gets that rule's code, and only an otherwise sound record over 256 bytes is `information-field-too-long` (a field over its own limit, such as message text over 67 characters, keeps `too-long` at its place in the order). Nothing is measured on a record that is refused. A decoded packet whose field is over 256 bytes decodes as before and re-encodes `refused`, `information-field-too-long`. `Packet.TNC2`, `AX25` and `KISS` write out a packet the program already holds: they are not encoders and write its information field as it stands, with no size check (their docs say so). The README no longer says the encoder has no size limit.
- A byte below 0x20 or the byte 0x7F in text (a comment, status text, a message, bulletin or NWS bulletin text, telemetry names, units and project title, capability values, the free text a weather report or an NMEA sentence leaves, a Maidenhead beacon's comment) gives one `control-character-in-text` warning (new code, tolerable), judged after the text's structure and encoding; `aprs.Strict` rejects it. TAB, and a CR or LF inside the text, count. User-defined, raw weather, test and undecoded data, Mic-E fixed bytes and Rev 0 telemetry, object and item names, capability tokens, addressees, headers and NMEA sentences keep their own rules. The encoder and builders refuse such text as `invalid-character` (it was only CR and LF); a Mic-E status text starting with 0x1D is refused instead of being written after a `/`.
- New code `CodeImplementationLimit` (`implementation-limit`) so the codes match the vectors. This library has no fixed capacity and never gives it.
- A fixed-width weather field whose digits run on by two or more, followed by the end of the information field or by a defined field still to be read, is `out-of-range-value`: the value is dropped and the fields carry on after the run (`t2147483647r000` has a rain of 0 and no temperature). The dropped value counts as present for `incomplete-weather`. Anything else after the run keeps the 1.1.0 rule.
- Builder defaults, written down: a builder never reads the clock; an object or positionless weather report with no timestamp is `missing-field`; a position, object, item or Mic-E report with no symbol is `invalid-symbol` (it was `missing-field`); a weather report's symbol is `/_`. A builder's refusal for a symbol or timestamp left out is named at its place in the order of refusals, not first: an object with a name too long and no symbol is `invalid-name`, and a positionless weather report with a humidity of 0 and no timestamp is `value-has-no-form`. A Mic-E altitude outside the Mic-E bytes' range is ranked the same way, so `not-a-number` elsewhere in the report wins over its `value-out-of-range`. The other defaults (not messaging, Mic-E off duty and type code `'`, telemetry bits `00000000`, no path, destination `APZ001`, no ambiguity, no compression) are unchanged.
- A Mic-E altitude goes in the Mic-E altitude bytes, rounded once to the nearest metre from the value given in either unit (1000 feet is 305 m); outside -10000 to 743570 m it is `value-out-of-range`, not moved to a `/A=`. `MicEBuilder.AltitudeFeet` no longer writes a `/A=`. The encoder, given neutral data, writes a `/A=` only for a whole number of feet within six digits that the Mic-E bytes do not hold exactly (1000 feet is `/A=001000`, -50000 feet is `/A=-50000`), and otherwise the Mic-E bytes (100.5 feet is 31 m); an altitude read from a `/A=` is still written back as one.
- Neutral telemetry data with no sequence is `missing-field` (it was `malformed`), and a positionless weather report with no timestamp is `missing-field` (it was `invalid-timestamp`).
- A Mic-E altitude with no type code before it, that no `/A=` holds exactly (not a whole number of feet, or beyond six digits) and whose first Mic-E byte would read as a type code, is `value-has-no-form`, not `text-reads-as-field`.
- A run-on weather value followed by its own field's letter (`t05700t057`) keeps the 1.1.0 reading: the run's own field counts as read, so it does not qualify.
- The vectors test runner runs the new `build` cases (builder recipes) through the real builders, with the same code as the dump tool's build mode (`internal/recipe`).
- The dump tool passes only the keys a recipe gives and never fills one in: a telemetry recipe with no sequence is `refused`, `missing-field` (the dump tool used to build it with sequence 0).
- The README says 1.2 and 33 tolerable defects.
- The dump tool, build mode: a recipe giving a quantity in two units whose one-unit builds are refused only for the 256-byte size is `value-given-twice`; the size ranks after it.
- The dump tool no longer reports `unsupported` for an input whose value alone decides a refusal code: a symbol that is not a table and a code (`invalid-symbol`), a third-party path entry that is not an address (`invalid-third-party-header`), undecoded inner data (`not-encodable`), an Agrelo bearing or quality out of range (`value-out-of-range`, `not-a-number` for NaN or an infinity), a bulletin id that is not one character (`invalid-addressee`) and a telemetry sequence outside 0-65535 (`value-out-of-range`) or not a number. The input is replaced by a valid stand-in, the record is encoded or built, and the first code in the Refusal reasons order among that result and the input's own is reported, with `"via": "tool"` when the tool's reader made it. The vectors test runner reads encode data the same way, and the `encode/nested-third-party-ending-in-undecoded-data` case, which it used to skip, now runs.
- Agrelo bearing rounding (12.5 is 013, 360.4 is 360, 360.5 is refused) is done by the dump tool's reader on neutral data, not by the library: `AgreloDF.BearingDegrees` is a whole number of degrees, and a caller of the Go API rounds its own value.
- Storm data's fields are digits: a field of dots or spaces is no longer storm data (the text stays in the comment), a whole gale radius that is not digits ends the storm data, and an encoder refuses storm data missing a required field as `missing-field` instead of writing dots. `Storm` fields that are nil in decoded data can no longer occur for the required ones.
- A telemetry sequence of `MIC` followed by more characters given to an encoder is `malformed` (it was `text-reads-as-field`).
- A run-on weather value in a positioned report is judged against the end of the weather text: a trailing base-91 telemetry block or `!DAO!` (lifted first) is not part of it, so `t2147483647!W12!` drops the run with `out-of-range-value` and reads the `!DAO!`.
- `Data()` on the object, item, position, Mic-E and weather builders refuses a symbol or timestamp left out with the same code `Build()` gives (`invalid-symbol`, `missing-field`), as in 1.1.0; it briefly returned the data with no error in this round.
- The dump tool marks every refusal it makes on the library's behalf `"via": "tool"` (a quantity in two units, a telemetry recipe with no sequence, the stand-ins), supplies no default the library lacks, and builds a recipe with no `station` from N0CALL as the vectors say.

### Breaking changes for callers

- An encoder or builder refuses an information field over 256 bytes (`information-field-too-long`); writing out an existing packet with `TNC2`, `AX25` or `KISS` is not encoding and does not.
- Text with a control character is refused (`invalid-character`), and a decoder gives `control-character-in-text`, so `aprs.Strict` now rejects such packets.
- A run-on weather value before a defined field or the end of the field is dropped with `out-of-range-value`, where it used to be read at its own width with the rest as text.
- A position, object, item or Mic-E report built with no symbol is refused as `invalid-symbol` (it was `missing-field`), and an object or positionless weather report built with no timestamp as `missing-field`, each ranked at its place in the order of refusals.
- A positionless `WeatherReport` with no timestamp, and a `Telemetry` with no sequence, are `missing-field` (they were `invalid-timestamp` and `malformed`).
- A Mic-E report's altitude given in feet goes in the Mic-E altitude bytes, rounded to the metre, instead of a `/A=` in the status text.
- Neutral Mic-E data of -50000 feet is written as `/A=-50000` instead of refused; neutral Mic-E altitudes that are neither whole feet nor whole metres go in the Mic-E bytes (they were a `/A=`).
- New `Code` values `CodeControlCharacterInText` (inserted in the middle of the list, so every `Code` after it moves up one) and `CodeImplementationLimit` (appended at the end), and a new `RefusalCode`, `RefusalInformationFieldTooLong` (appended at the end): do not store a `Code` or `RefusalCode` as a number.
- The 256-byte size refusal is `information-field-too-long`, not `too-long`: code that matched `RefusalTooLong` for an oversize field must match the new code too.
- Storm data with a dotted or blank field is comment text now, and an encoder refuses a `Storm` with a nil required field (`missing-field`) instead of writing dots.
- A telemetry sequence beginning `MIC` and longer than `MIC` is refused as `malformed`.

## 1.1.0 (2026-11-14)

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
- An NMEA field or a compressed coordinate that is not a number is `not-a-number`, named before an invalid sentence, a comment with a line break, or the other coordinate being out of range.
- A speed, course or other whole-number field given as NaN or an infinity is `not-a-number`, not `value-out-of-range`.
- The dump tool reads the `NaN`, `Infinity` and `-Infinity` tokens the generator writes, and ranks a recipe that gives a quantity in two units: it builds once with each unit and reports the first code among those results and `value-given-twice`, never `unsupported`. A recipe ambiguity that is not a finite number is ranked as `not-a-number`.
- The dump tool's reader names undecoded inner data in a third-party packet before a header fault, and reads an absent positioned symbol as an empty one (`invalid-symbol`).

## 1.0.1 (2026-11-13)

First Go release. It matches the conformance vectors at 1.0.1: every case passes, and it writes the same bytes as the other five implementations (C#, Rust, Python, TypeScript and C) over the vectors' cases and over differential runs of generated data, builder recipes and fuzzed packets.

- Decoding from TNC2 / APRS-IS text, AX.25 frames and KISS frames, every APRS 1.2 data type, with lenient, strict or per-code options. The zero `ParseOptions` is `Lenient`.
- Encoding and the station builder, with read-back checks and the 26 shared refusal codes (`EncodeError.Code()`). `Encode` and `NewPacket` take a data value or a pointer to one.
- TNC2, AX.25 and KISS output, device identification from the APRS device identification database, and named symbols.
- `cmd/aprs-diffdump`, the dump tool for the vectors' `tools/compare.py`.

The version starts at 1.0.1 to share its major and minor version with the vectors and the other implementations.
