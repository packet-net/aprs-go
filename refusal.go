package aprs

import "fmt"

// RefusalCode says why data was refused. The ids match the refusals list in the conformance
// vectors' codes.json. New codes may be added in later 1.x releases, so a switch over them should
// have a default case.
type RefusalCode uint8

const (
	// RefusalInvalidAddress: a header address is not 1-9 letters, digits or - in TNC2 text.
	RefusalInvalidAddress RefusalCode = iota
	// RefusalAX25CannotCarry: an AX.25 frame cannot carry the header: a call over 6 characters
	// or in lower case, an SSID over 15 or with a leading zero, more than 8 path entries.
	RefusalAX25CannotCarry
	// RefusalTNC2CannotCarry: a TNC2 line cannot carry the packet.
	RefusalTNC2CannotCarry
	// RefusalInvalidThirdPartyHeader: a third-party packet's inner header cannot be written as received.
	RefusalInvalidThirdPartyHeader
	// RefusalInvalidAddressee: a message, bulletin, telemetry metadata or directed query
	// addressee is not the form its report needs.
	RefusalInvalidAddressee
	// RefusalInvalidName: an object or item name is the wrong length or holds a character a name cannot.
	RefusalInvalidName
	// RefusalInvalidMessageID: a message ID is empty, too long or not letters and digits, or an
	// ack or reject carries the wrong kind of ID.
	RefusalInvalidMessageID
	// RefusalPaddingSpace: text starts or ends with a space that would be read back as padding.
	RefusalPaddingSpace
	// RefusalTooLong: free text or a list is longer than its field allows.
	RefusalTooLong
	// RefusalMalformed: text in a fixed-form field is not that form (a locator, a bit sense, a
	// capability token).
	RefusalMalformed
	// RefusalInvalidCharacter: free text holds a character its field cannot carry, such as a line break.
	RefusalInvalidCharacter
	// RefusalTextReadsAsField: free text would read back as a structured element and no clean form avoids it.
	RefusalTextReadsAsField
	// RefusalNotANumber: a value is NaN or infinite.
	RefusalNotANumber
	// RefusalValueOutOfRange: a number is outside what its field holds, after rounding.
	RefusalValueOutOfRange
	// RefusalValueHasNoForm: a value inside the range that no form reads back as.
	RefusalValueHasNoForm
	// RefusalValueGivenTwice: a builder input was given in two units.
	RefusalValueGivenTwice
	// RefusalInvalidTimestamp: a timestamp has a field out of range, or is a format the report does not take.
	RefusalInvalidTimestamp
	// RefusalMissingField: the data lacks something its report or another field needs.
	RefusalMissingField
	// RefusalInvalidSymbol: the symbol table or code is not a valid symbol.
	RefusalInvalidSymbol
	// RefusalSymbolMismatch: the symbol disagrees with the data.
	RefusalSymbolMismatch
	// RefusalExtensionConflict: two things want the one data extension or the compressed cs bytes.
	RefusalExtensionConflict
	// RefusalFieldNotAllowed: the report has no place for a field, or it contradicts another.
	RefusalFieldNotAllowed
	// RefusalInvalidCode: an enumerated value is outside its set.
	RefusalInvalidCode
	// RefusalInvalidNMEA: an NMEA sentence is malformed, its checksum does not match, or a field
	// given beside it disagrees.
	RefusalInvalidNMEA
	// RefusalNotEncodable: the call has nothing it can write.
	RefusalNotEncodable
	// RefusalDoesNotReadBack: fallback: the library's own read-back check found the bytes decode
	// to different data and no rule names why.
	RefusalDoesNotReadBack
)

var refusalIDs = [...]string{
	"invalid-address", "ax25-cannot-carry", "tnc2-cannot-carry", "invalid-third-party-header",
	"invalid-addressee", "invalid-name", "invalid-message-id", "padding-space", "too-long", "malformed",
	"invalid-character", "text-reads-as-field", "not-a-number", "value-out-of-range", "value-has-no-form",
	"value-given-twice", "invalid-timestamp", "missing-field", "invalid-symbol", "symbol-mismatch",
	"extension-conflict", "field-not-allowed", "invalid-code", "invalid-nmea", "not-encodable",
	"does-not-read-back",
}

// AllRefusalCodes gives every refusal code, in the order of the vectors' refusals list.
func AllRefusalCodes() []RefusalCode {
	out := make([]RefusalCode, len(refusalIDs))
	for i := range out {
		out[i] = RefusalCode(i)
	}
	return out
}

// ID gives the kebab-case id used in the conformance vectors, such as "value-out-of-range".
func (c RefusalCode) ID() string {
	if int(c) < len(refusalIDs) {
		return refusalIDs[c]
	}
	return fmt.Sprintf("refusal-%d", uint8(c))
}

// String gives the code's id.
func (c RefusalCode) String() string {
	return c.ID()
}

// RefusalCodeFromID gives the refusal code with this vectors id, if there is one.
func RefusalCodeFromID(id string) (RefusalCode, bool) {
	for i, s := range refusalIDs {
		if s == id {
			return RefusalCode(i), true
		}
	}
	return 0, false
}

// refusalForValue is the code for a number a check refused: not-a-number for NaN or an
// infinity, else out of range.
func refusalForValue(x float64) RefusalCode {
	if isFinite(x) {
		return RefusalValueOutOfRange
	}
	return RefusalNotANumber
}

// refusalOrder is the order in which a refusal with several faults names its code (the vectors
// README, Refusal reasons): the first code that applies.
var refusalOrder = [...]RefusalCode{
	RefusalNotEncodable, RefusalInvalidAddress, RefusalAX25CannotCarry, RefusalTNC2CannotCarry,
	RefusalInvalidThirdPartyHeader, RefusalInvalidAddressee, RefusalInvalidName, RefusalInvalidTimestamp,
	RefusalNotANumber, RefusalValueOutOfRange, RefusalValueHasNoForm, RefusalInvalidCode, RefusalInvalidSymbol,
	RefusalInvalidCharacter, RefusalTooLong, RefusalPaddingSpace, RefusalMalformed, RefusalInvalidMessageID,
	RefusalInvalidNMEA, RefusalExtensionConflict, RefusalFieldNotAllowed, RefusalSymbolMismatch,
	RefusalMissingField, RefusalValueGivenTwice, RefusalTextReadsAsField, RefusalDoesNotReadBack,
}

func (c RefusalCode) rank() int {
	for i, o := range refusalOrder {
		if o == c {
			return i
		}
	}
	return len(refusalOrder)
}

// EncodeError is data that cannot be written as asked, with the reason. Error gives the reason as
// text for people, naming the field at fault; its wording is not part of the stable API. Code is
// the machine-readable reason.
type EncodeError struct {
	code    RefusalCode
	message string
}

func newEncodeError(code RefusalCode, message string) *EncodeError {
	return &EncodeError{code: code, message: message}
}

// Code says why the data was refused, as a code a program can act on. Callers must allow codes
// they do not know.
func (e *EncodeError) Code() RefusalCode {
	return e.code
}

// Error says why the data was refused, in a sentence.
func (e *EncodeError) Error() string {
	return e.message
}

// pending collects the faults of one piece of data and keeps the one whose code comes first in
// the order above, so the refusal does not depend on which check happens to run first. The
// encoder goes on after a fault, writing bytes that are thrown away, and gives the result at the
// end.
type pending struct {
	err *EncodeError
}

func (p *pending) note(e *EncodeError) {
	if p.err == nil || e.code.rank() < p.err.code.rank() {
		p.err = e
	}
}

// run notes a fault, if there is one, and says whether there was none.
func (p *pending) run(e *EncodeError) bool {
	if e != nil {
		p.note(e)
		return false
	}
	return true
}

// maxInformationField is the most bytes an information field holds (APRS12c ch. 3).
const maxInformationField = 256

// size notes a field over 256 bytes: n is the bytes the data's fields make as given, before
// anything else is looked at, so a later fault in the order of refusals does not hide it. A
// field the report has no place for, or one left out, makes none.
func (p *pending) size(n int) {
	if n > maxInformationField {
		p.note(newEncodeError(RefusalTooLong, fmt.Sprintf("an information field is at most 256 bytes (APRS12c ch. 3), and this one would be %d", n)))
	}
}

func (p *pending) finish() *EncodeError {
	return p.err
}
