package aprs

// Lifting an element out of a comment never joins the text either side of it (vectors
// interpretations.md): an element is recognised only where its bytes stand whole as sent. So
// the text being lifted from carries, for each byte, where something was lifted from just
// before it, and no recogniser reads across such a place.

// cutKind says what was lifted from a place: a /A= altitude or anything else. A voice
// frequency's fields may follow a lifted /A= (APRS12c ch. 6: it may stand anywhere), and
// nothing else.
const (
	cutAltitude uint8 = 1 << iota
	cutOther
)

// liftedText is the bytes left of a text and, at each byte, the kinds of element lifted from
// just before it.
type liftedText struct {
	b   []byte
	cut []uint8
}

func newLiftedText(b []byte) *liftedText {
	return &liftedText{b: append([]byte(nil), b...), cut: make([]uint8, len(b))}
}

// lift takes b[from:to] out and records the place it came from.
func (t *liftedText) lift(from, to int, kind uint8) {
	merged := kind | t.cut[from]
	if to < len(t.b) {
		merged |= t.cut[to]
	}
	t.b = append(t.b[:from], t.b[to:]...)
	t.cut = append(t.cut[:from], t.cut[to:]...)
	if from < len(t.b) {
		t.cut[from] = merged
	}
}

// whole says whether b[from:to] stands as one run: nothing was lifted from inside it.
func (t *liftedText) whole(from, to int) bool {
	for i := from + 1; i < to && i < len(t.cut); i++ {
		if t.cut[i] != 0 {
			return false
		}
	}
	return true
}

// pieceEnd gives where the run that holds b[at] ends, looking no further than max bytes on: a
// reader that takes at most max bytes need not know more, and a scan to the end of the text for
// every candidate would be quadratic on a long run of them.
func (t *liftedText) pieceEnd(at, max int) int {
	limit := len(t.b)
	if at+max < limit {
		limit = at + max
	}
	for i := at + 1; i < limit; i++ {
		if t.cut[i] != 0 {
			return i
		}
	}
	return limit
}
