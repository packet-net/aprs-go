package aprs

// Base-91 numbers: each byte is a digit, ! (33) to { (123).

func isBase91(b byte) bool {
	return b >= 33 && b <= 123
}

// base91Decode gives the value of a run of base-91 digits, most significant first.
func base91Decode(b []byte) (uint32, bool) {
	var n uint32
	for _, c := range b {
		if !isBase91(c) {
			return 0, false
		}
		n = n*91 + uint32(c-33)
	}
	return n, true
}

// base91Encode appends value as width base-91 digits, most significant first.
func base91Encode(out []byte, value uint32, width int) []byte {
	start := len(out)
	for i := 0; i < width; i++ {
		out = append(out, 0)
	}
	for i := width - 1; i >= 0; i-- {
		out[start+i] = byte(value%91) + 33
		value /= 91
	}
	return out
}
