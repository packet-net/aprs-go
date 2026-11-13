package aprs

// Code is a diagnostic code. The names match the ids in the conformance vectors' codes.json.
// New codes may be added in later 1.x releases.
type Code uint8

const (
	// CodeInvalidHeader: The TNC2 header is not `SOURCE>DEST[,PATH]:`.
	CodeInvalidHeader Code = iota
	// CodeInvalidAddress: An address is empty, too long, or has characters that cannot appear in one.
	CodeInvalidAddress
	// CodeEmptyDestination: The destination address is empty (UAP 5.2).
	CodeEmptyDestination
	// CodeEmptyPathEntry: The digipeater path has an empty entry (UAP 5.6).
	CodeEmptyPathEntry
	// CodeMultipleUsedMarkers: More than one path entry is marked used with *; only the last used one should be (UAP 5.30).
	CodeMultipleUsedMarkers
	// CodeNotAprsFrame: The AX.25 frame is not a UI frame with PID 0xF0, or is too short (APRS12c ch. 3).
	CodeNotAprsFrame
	// CodeNulPaddedAddress: An AX.25 address is padded with NUL rather than spaces (UAP 5.29).
	CodeNulPaddedAddress
	// CodeInvalidAX25AddressCharacters: An AX.25 address has characters other than upper-case letters and digits.
	CodeInvalidAX25AddressCharacters
	// CodeTooManyDigipeaters: The AX.25 frame has more than 8 digipeater addresses.
	CodeTooManyDigipeaters
	// CodeTrailingLineBreak: The information field ends with CR or LF (APRS12c ch. 5, UAP 5.13).
	CodeTrailingLineBreak
	// CodeNonUTF8Text: Text that is not valid UTF-8 (UAP 5.16).
	CodeNonUTF8Text
	// CodeTruncated: The information field is shorter than its format requires.
	CodeTruncated
	// CodeNotAprs: The first byte is not a data type identifier (APRS12c ch. 20).
	CodeNotAprs
	// CodeReservedDataType: A reserved data type identifier with no defined format (APRS12c ch. 5).
	CodeReservedDataType
	// CodeObsoleteFormat: A format the spec marks obsolete or not recommended, e.g. raw NMEA, raw weather, or the Rev 0 Mic-E data type identifiers 0x1C and 0x1D.
	CodeObsoleteFormat
	// CodeOutOfRangeValue: A value in a well-formed field is out of range and was dropped.
	CodeOutOfRangeValue
	// CodeInvalidTimestamp: A timestamp is malformed or out of range (APRS12c ch. 6, UAP 5.8).
	CodeInvalidTimestamp
	// CodeInvalidPosition: The position is missing or starts with something that cannot begin one.
	CodeInvalidPosition
	// CodeInvalidLatitude: The latitude is malformed (APRS12c ch. 6, UAP 5.7).
	CodeInvalidLatitude
	// CodeInvalidLongitude: The longitude is malformed (APRS12c ch. 6, UAP 5.7).
	CodeInvalidLongitude
	// CodeLowercaseHemisphere: A lower-case hemisphere letter (UAP 5.9).
	CodeLowercaseHemisphere
	// CodeInvalidSymbolTable: The symbol table identifier is not /, \, 0-9 or A-Z.
	CodeInvalidSymbolTable
	// CodeInvalidSymbolCode: The symbol code is not printable ASCII.
	CodeInvalidSymbolCode
	// CodeInvalidCompressedPosition: A compressed position is malformed (APRS12c ch. 9).
	CodeInvalidCompressedPosition
	// CodeDAOWithAmbiguity: A !DAO! adds precision to an ambiguous position, which contradicts it.
	CodeDAOWithAmbiguity
	// CodeDataExtensionInComment: A data extension (PHG, RNG, DFS) appears later in the comment (UAP 5.15).
	CodeDataExtensionInComment
	// CodeInvalidObjectName: The object name is empty or not printable ASCII.
	CodeInvalidObjectName
	// CodeObjectNameNotPadded: The object name is not padded to 9 characters.
	CodeObjectNameNotPadded
	// CodeObjectWithoutTimestamp: An object report has no timestamp (APRS12c ch. 11).
	CodeObjectWithoutTimestamp
	// CodeInvalidItemName: The item name is not 3-9 printable characters followed by ! or _.
	CodeInvalidItemName
	// CodeIncompleteWeather: A weather report lacks a mandatory field (APRS12c ch. 12).
	CodeIncompleteWeather
	// CodeWeatherComment: Text after the weather data; weather reports have no comment (UAP 2.7.1, ch. 5.33).
	CodeWeatherComment
	// CodeInvalidWeather: Positionless or raw weather data that could not be decoded.
	CodeInvalidWeather
	// CodeInvalidMicEDestination: The destination address is not a valid Mic-E encoding (APRS12c ch. 10).
	CodeInvalidMicEDestination
	// CodeInvalidMicEInformation: The Mic-E information field is malformed (APRS12c ch. 10).
	CodeInvalidMicEInformation
	// CodeKenwoodFFPadding: Kenwood TM-D710 0xFF padding was removed from Mic-E status text, after the destination and the nine fixed bytes decoded (UAP 5.10).
	CodeKenwoodFFPadding
	// CodeMicEMissingDeviceType: A Mic-E report without a device type prefix (UAP 5.4).
	CodeMicEMissingDeviceType
	// CodeInvalidMessage: A message is malformed (APRS12c ch. 14).
	CodeInvalidMessage
	// CodeUnpaddedAddressee: The addressee is not padded to 9 characters.
	CodeUnpaddedAddressee
	// CodeMessageIDOnAck: An ack or rej carries a message ID of its own (UAP 5.32).
	CodeMessageIDOnAck
	// CodeInvalidTelemetryMetadata: A telemetry metadata message (PARM/UNIT/EQNS/BITS) is malformed (APRS12c ch. 13).
	CodeInvalidTelemetryMetadata
	// CodeInvalidQuery: A directed query is malformed (APRS12c ch. 15, UAP 5.18).
	CodeInvalidQuery
	// CodeInvalidTelemetry: A telemetry report is malformed (APRS12c ch. 13).
	CodeInvalidTelemetry
	// CodeInvalidStatus: A status report is malformed (APRS12c ch. 16).
	CodeInvalidStatus
	// CodeInvalidLocator: A Maidenhead locator is malformed.
	CodeInvalidLocator
	// CodeInvalidNMEA: The text after `$` is not an NMEA 0183 sentence: not printable ASCII, no valid address field, or a reserved character in a field.
	CodeInvalidNMEA
	// CodeNMEAChecksumMismatch: An NMEA sentence's checksum does not match, so the sentence is corrupt and is not decoded.
	CodeNMEAChecksumMismatch
	// CodeInvalidThirdParty: A third-party header is malformed, or (strict) its inner header has a defect a lenient decoder tolerates (APRS12c ch. 17).
	CodeInvalidThirdParty
	// CodeInvalidGeneralQuery: A general query is malformed, or its footprint is out of range (APRS12c ch. 15).
	CodeInvalidGeneralQuery
	// CodeInvalidCapabilities: A station capabilities report is malformed (APRS12c ch. 15).
	CodeInvalidCapabilities
	// CodeInvalidUserDefined: A user-defined packet is shorter than its 3-byte header (APRS12c ch. 19).
	CodeInvalidUserDefined
	// CodeInvalidAgreloDF: An Agrelo DF report is not exactly `%`, a bearing of 000 to 360, `/` and a quality digit.
	CodeInvalidAgreloDF
	// CodeMissingSpaceAfterLocator: A grid-locator status report lacks the mandatory space before its text (UAP 5.17).
	CodeMissingSpaceAfterLocator
	// CodeCompressionTypeReservedBits: A compressed position's type byte sets its unused high bits (APRS12c ch. 9).
	CodeCompressionTypeReservedBits
	// CodeMalformedTimestamp: A timestamped position report whose timestamp is missing or not timestamp-shaped (UAP 5.8).
	CodeMalformedTimestamp
	// CodePositionNotAtStart: A ! position found after other text (obsolete TNC beacon rule).
	CodePositionNotAtStart
	// CodeNonStandardWeatherFieldWidth: A weather field is one character shorter or longer than its fixed width (UAP 5.31).
	CodeNonStandardWeatherFieldWidth
	// CodeWindFieldsInsteadOfExtension: Wind sent as c/s fields in a position weather report instead of the DDD/SSS extension, or after a compressed position whose cs bytes carry no wind.
	CodeWindFieldsInsteadOfExtension
	// CodeWindExtensionAfterCompressed: An uncompressed wind extension after a compressed weather position (UAP 5.33).
	CodeWindExtensionAfterCompressed
	// CodeMicEAltitudeNotFirst: A Mic-E altitude after other status text instead of first (APRS12c ch. 10).
	CodeMicEAltitudeNotFirst
	// CodeBraceInMessageText: Message, bulletin or telemetry metadata text contains a { that does not start a valid message ID, including the reply-ack form on a bulletin or metadata (APRS12c ch. 14).
	CodeBraceInMessageText
	// CodeInvalidAddresseeCharacters: A message addressee contains a space or : (APRS12c ch. 14).
	CodeInvalidAddresseeCharacters
	// CodeLetterGroupBulletin: A bulletin addressee has a group name after a letter, e.g. BLNCNET; group bulletins use a digit (APRS12c ch. 14).
	CodeLetterGroupBulletin
	// CodeFreeTextCapabilities: A < station capabilities packet holds free text rather than TOKEN / TOKEN=VALUE items: a token that is empty or holds a space or a control character, or a value that holds a control character (APRS12c ch. 15).
	CodeFreeTextCapabilities
)

var codeIDs = [...]string{
	CodeInvalidHeader:                "invalid-header",
	CodeInvalidAddress:               "invalid-address",
	CodeEmptyDestination:             "empty-destination",
	CodeEmptyPathEntry:               "empty-path-entry",
	CodeMultipleUsedMarkers:          "multiple-used-markers",
	CodeNotAprsFrame:                 "not-aprs-frame",
	CodeNulPaddedAddress:             "nul-padded-address",
	CodeInvalidAX25AddressCharacters: "invalid-ax25-address-characters",
	CodeTooManyDigipeaters:           "too-many-digipeaters",
	CodeTrailingLineBreak:            "trailing-line-break",
	CodeNonUTF8Text:                  "non-utf8-text",
	CodeTruncated:                    "truncated",
	CodeNotAprs:                      "not-aprs",
	CodeReservedDataType:             "reserved-data-type",
	CodeObsoleteFormat:               "obsolete-format",
	CodeOutOfRangeValue:              "out-of-range-value",
	CodeInvalidTimestamp:             "invalid-timestamp",
	CodeInvalidPosition:              "invalid-position",
	CodeInvalidLatitude:              "invalid-latitude",
	CodeInvalidLongitude:             "invalid-longitude",
	CodeLowercaseHemisphere:          "lowercase-hemisphere",
	CodeInvalidSymbolTable:           "invalid-symbol-table",
	CodeInvalidSymbolCode:            "invalid-symbol-code",
	CodeInvalidCompressedPosition:    "invalid-compressed-position",
	CodeDAOWithAmbiguity:             "dao-with-ambiguity",
	CodeDataExtensionInComment:       "data-extension-in-comment",
	CodeInvalidObjectName:            "invalid-object-name",
	CodeObjectNameNotPadded:          "object-name-not-padded",
	CodeObjectWithoutTimestamp:       "object-without-timestamp",
	CodeInvalidItemName:              "invalid-item-name",
	CodeIncompleteWeather:            "incomplete-weather",
	CodeWeatherComment:               "weather-comment",
	CodeInvalidWeather:               "invalid-weather",
	CodeInvalidMicEDestination:       "invalid-mic-e-destination",
	CodeInvalidMicEInformation:       "invalid-mic-e-information",
	CodeKenwoodFFPadding:             "kenwood-ff-padding",
	CodeMicEMissingDeviceType:        "mic-e-missing-device-type",
	CodeInvalidMessage:               "invalid-message",
	CodeUnpaddedAddressee:            "unpadded-addressee",
	CodeMessageIDOnAck:               "message-id-on-ack",
	CodeInvalidTelemetryMetadata:     "invalid-telemetry-metadata",
	CodeInvalidQuery:                 "invalid-query",
	CodeInvalidTelemetry:             "invalid-telemetry",
	CodeInvalidStatus:                "invalid-status",
	CodeInvalidLocator:               "invalid-locator",
	CodeInvalidNMEA:                  "invalid-nmea",
	CodeNMEAChecksumMismatch:         "nmea-checksum-mismatch",
	CodeInvalidThirdParty:            "invalid-third-party",
	CodeInvalidGeneralQuery:          "invalid-general-query",
	CodeInvalidCapabilities:          "invalid-capabilities",
	CodeInvalidUserDefined:           "invalid-user-defined",
	CodeInvalidAgreloDF:              "invalid-agrelo-df",
	CodeMissingSpaceAfterLocator:     "missing-space-after-locator",
	CodeCompressionTypeReservedBits:  "compression-type-reserved-bits",
	CodeMalformedTimestamp:           "malformed-timestamp",
	CodePositionNotAtStart:           "position-not-at-start",
	CodeNonStandardWeatherFieldWidth: "non-standard-weather-field-width",
	CodeWindFieldsInsteadOfExtension: "wind-fields-instead-of-extension",
	CodeWindExtensionAfterCompressed: "wind-extension-after-compressed",
	CodeMicEAltitudeNotFirst:         "mic-e-altitude-not-first",
	CodeBraceInMessageText:           "brace-in-message-text",
	CodeInvalidAddresseeCharacters:   "invalid-addressee-characters",
	CodeLetterGroupBulletin:          "letter-group-bulletin",
	CodeFreeTextCapabilities:         "free-text-capabilities",
}

var codeTolerable = [...]bool{
	CodeEmptyDestination:             true,
	CodeEmptyPathEntry:               true,
	CodeMultipleUsedMarkers:          true,
	CodeNulPaddedAddress:             true,
	CodeInvalidAX25AddressCharacters: true,
	CodeTrailingLineBreak:            true,
	CodeNonUTF8Text:                  true,
	CodeOutOfRangeValue:              true,
	CodeInvalidTimestamp:             true,
	CodeLowercaseHemisphere:          true,
	CodeDAOWithAmbiguity:             true,
	CodeDataExtensionInComment:       true,
	CodeObjectNameNotPadded:          true,
	CodeObjectWithoutTimestamp:       true,
	CodeIncompleteWeather:            true,
	CodeWeatherComment:               true,
	CodeKenwoodFFPadding:             true,
	CodeUnpaddedAddressee:            true,
	CodeMessageIDOnAck:               true,
	CodeInvalidTelemetry:             true,
	CodeMissingSpaceAfterLocator:     true,
	CodeCompressionTypeReservedBits:  true,
	CodeMalformedTimestamp:           true,
	CodePositionNotAtStart:           true,
	CodeNonStandardWeatherFieldWidth: true,
	CodeWindFieldsInsteadOfExtension: true,
	CodeWindExtensionAfterCompressed: true,
	CodeMicEAltitudeNotFirst:         true,
	CodeBraceInMessageText:           true,
	CodeInvalidAddresseeCharacters:   true,
	CodeLetterGroupBulletin:          true,
	CodeFreeTextCapabilities:         true,
}
