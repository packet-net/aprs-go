package aprs

import (
	"strings"
	"unicode/utf8"
)

// decodeText reads text: valid UTF-8 as UTF-8; any invalid byte means the whole text is read as
// Latin-1, a tolerated defect (CodeNonUTF8Text). Then a byte below 0x20 or 0x7F anywhere in the
// text is one more tolerated defect (CodeControlCharacterInText), judged last, after the text's
// encoding. False when a defect is not tolerated.
func decodeText(ctx *context, b []byte, offset int) (string, bool) {
	s, ok := decodeTextUnjudged(ctx, b, offset)
	if !ok {
		return "", false
	}
	if !checkControlCharacters(ctx, b, offset) {
		return "", false
	}
	return s, true
}

// decodeTextUnjudged is decodeText without the control-character judgement, for text whose
// control characters are judged some other way, or not at all.
func decodeTextUnjudged(ctx *context, b []byte, offset int) (string, bool) {
	if utf8.Valid(b) {
		return string(b), true
	}
	if !ctx.tolerate(CodeNonUTF8Text, "text that is not valid UTF-8, read as Latin-1 (UAP 5.16)", offset) {
		return "", false
	}
	return latin1(b), true
}

// checkControlCharacters gives one tolerated defect for text with a byte below 0x20 or the byte
// 0x7F in it, wherever and however many. False when the defect is not tolerated.
func checkControlCharacters(ctx *context, b []byte, offset int) bool {
	for i, c := range b {
		if c < 0x20 || c == 0x7F {
			return ctx.tolerate(CodeControlCharacterInText, "text with a control character in it (a byte below 0x20 or 0x7F); APRS12c gives text as printable characters", offset+i)
		}
	}
	return true
}

// hasControl says whether text has a byte below 0x20 or the byte 0x7F, which an encoder refuses.
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7F {
			return true
		}
	}
	return false
}

// latin1 reads every byte as the code point of the same value.
func latin1(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

// forDisplay is UTF-8 if valid, otherwise Latin-1; it never fails, and reports nothing.
func forDisplay(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return latin1(b)
}

// lossy reads UTF-8, replacing each maximal invalid sequence with U+FFFD, as Rust's
// String::from_utf8_lossy does.
func lossy(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r == utf8.RuneError && n <= 1 {
			sb.WriteRune(utf8.RuneError)
			b = b[maximalInvalid(b):]
			continue
		}
		sb.WriteRune(r)
		b = b[n:]
	}
	return sb.String()
}

// maximalInvalid gives the length of the maximal prefix of b that starts a well-formed UTF-8
// sequence without completing one (at least 1), per the Unicode substitution rule.
func maximalInvalid(b []byte) int {
	c := b[0]
	var need int
	lo, hi := byte(0x80), byte(0xBF)
	switch {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c == 0xE0:
		need, lo = 2, 0xA0
	case c >= 0xE1 && c <= 0xEC, c == 0xEE, c == 0xEF:
		need = 2
	case c == 0xED:
		need, hi = 2, 0x9F
	case c == 0xF0:
		need, lo = 3, 0x90
	case c >= 0xF1 && c <= 0xF3:
		need = 3
	case c == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	i := 1
	for ; i <= need && i < len(b); i++ {
		if b[i] < lo || b[i] > hi {
			break
		}
		lo, hi = 0x80, 0xBF
	}
	return i
}

// hasLineBreak says whether text has a CR or LF, which would end the packet on the air or on
// APRS-IS.
func hasLineBreak(b []byte) bool {
	for _, c := range b {
		if c == '\r' || c == '\n' {
			return true
		}
	}
	return false
}

// isPrintableASCII says whether every byte is printable ASCII (space to ~).
func isPrintableASCII(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

func allDigits(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if !isDigit(c) {
			return false
		}
	}
	return true
}

// digits is the value of a run of ASCII digits (wrapping as a u32 would).
func digits(b []byte) uint32 {
	var n uint32
	for _, c := range b {
		n = n*10 + uint32(c-'0')
	}
	return n
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isAlpha(c byte) bool { return isUpper(c) || isLower(c) }
func isAlnum(c byte) bool { return isAlpha(c) || isDigit(c) }
func isHex(c byte) bool   { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func toUpper(c byte) byte {
	if isLower(c) {
		return c - 32
	}
	return c
}
func toLower(c byte) byte {
	if isUpper(c) {
		return c + 32
	}
	return c
}

// asciiUpper upper-cases ASCII letters only, as Rust's to_ascii_uppercase does.
func asciiUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		b[i] = toUpper(c)
	}
	return string(b)
}

func runeIsASCIIUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func runeIsASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }
func runeIsASCIIAlpha(r rune) bool { return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') }
func runeToASCIILower(r rune) rune {
	if runeIsASCIIUpper(r) {
		return r + 32
	}
	return r
}

func contains(b []byte, c byte) bool {
	for _, x := range b {
		if x == c {
			return true
		}
	}
	return false
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func lastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func hasPrefix(b []byte, p string) bool {
	return len(b) >= len(p) && string(b[:len(p)]) == p
}

// get gives b[from:to] when it is in range, as Rust's slice get does.
func get(b []byte, from, to int) ([]byte, bool) {
	if from < 0 || to > len(b) || from > to {
		return nil, false
	}
	return b[from:to], true
}

// from gives b[at:], or an empty slice when at is past the end.
func from(b []byte, at int) []byte {
	if at >= len(b) {
		return nil
	}
	return b[at:]
}

// runeCount counts the characters of text, as Rust's chars().count() does.
func runeCount(s string) int {
	return utf8.RuneCountInString(s)
}
