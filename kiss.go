package aprs

import "errors"

// KISS framing (the KISS TNC protocol): FEND (0xC0) delimits a frame, FESC (0xDB) escapes FEND
// and FESC inside it, and the first byte is the command, whose low four bits are 0 for a data
// frame and whose high four bits are the port.

const (
	kissFEND  = 0xC0
	kissFESC  = 0xDB
	kissTFEND = 0xDC
	kissTFESC = 0xDD
)

// ErrNotKISSData is returned for a KISS frame that is empty or is not a data frame.
var ErrNotKISSData = errors.New("not a KISS data frame")

// WrapKISS wraps an AX.25 frame (as AX25 gives it) for a KISS TNC: FEND, a data command for port
// (0-15), the frame with FEND and FESC escaped, and FEND.
func WrapKISS(frame []byte, port uint8) []byte {
	out := make([]byte, 0, len(frame)+4)
	out = append(out, kissFEND, (port&0x0F)<<4)
	for _, b := range frame {
		switch b {
		case kissFEND:
			out = append(out, kissFESC, kissTFEND)
		case kissFESC:
			out = append(out, kissFESC, kissTFESC)
		default:
			out = append(out, b)
		}
	}
	return append(out, kissFEND)
}

// UnwrapKISS removes KISS framing: FEND delimiters, FESC escapes and the command byte. It gives
// ErrNotKISSData when the frame is empty or its command is not a data frame.
func UnwrapKISS(frame []byte) ([]byte, error) {
	start, end := 0, len(frame)
	for start < end && frame[start] == kissFEND {
		start++
	}
	for end > start && frame[end-1] == kissFEND {
		end--
	}
	out := make([]byte, 0, end-start)
	for i := start; i < end; i++ {
		b := frame[i]
		if b == kissFESC && i+1 < end {
			i++
			switch frame[i] {
			case kissTFEND:
				out = append(out, kissFEND)
			case kissTFESC:
				out = append(out, kissFESC)
			default:
				out = append(out, frame[i])
			}
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 || out[0]&0x0F != 0 {
		return nil, ErrNotKISSData
	}
	return out[1:], nil
}

// DecodeKISS decodes a KISS data frame as read from a TNC: any FEND delimiters, the command byte,
// then an escaped AX.25 frame. It fails with ErrNotKISSData for a frame that is not a KISS data
// frame, and otherwise as DecodeAX25 does.
func DecodeKISS(frame []byte, options ParseOptions) (*Packet, error) {
	ax25, err := UnwrapKISS(frame)
	if err != nil {
		return nil, err
	}
	return DecodeAX25(ax25, options)
}

// KISS gives the packet as a KISS data frame for port (0-15), with FEND delimiters, for a KISS
// TNC. It fails as AX25 does.
func (p *Packet) KISS(port uint8) ([]byte, error) {
	frame, err := p.AX25()
	if err != nil {
		return nil, err
	}
	return WrapKISS(frame, port), nil
}
