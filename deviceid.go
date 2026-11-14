package aprs

// Device identification from the APRS device identification database
// (https://github.com/aprsorg/aprs-deviceid, CC BY-SA 2.0, maintained by OH7LZB): the destination
// address ("tocall") for most packets, the comment prefix and suffix for Mic-E.

// Device is a device or program, as the database names it. A field the database leaves out is
// empty.
type Device struct {
	// Vendor is who makes it.
	Vendor string
	// Model is what it is.
	Model string
	// Class is the database's device class, such as rig, ht, tracker or software.
	Class string
	// OS is the operating system, for software.
	OS string
	// Features are feature flags, such as messaging.
	Features []string
}

// String is the vendor and model with a space between, as the Python library prints it
// ("Open Source APRSdroid"); a part the database leaves out is dropped.
func (d Device) String() string {
	switch {
	case d.Vendor == "":
		return d.Model
	case d.Model == "":
		return d.Vendor
	}
	return d.Vendor + " " + d.Model
}

// clone gives a copy whose Features a caller may change without changing the database.
func (d Device) clone() Device {
	d.Features = append([]string(nil), d.Features...)
	return d
}

type tocallEntry struct {
	pattern string
	device  Device
}

type micESuffixEntry struct {
	suffix string
	device Device
}

type micELegacyEntry struct {
	prefix    string
	suffix    string
	hasSuffix bool
	device    Device
}

// DeviceDatabaseVersion is the database version this package carries: the aprs-deviceid commit
// and its date.
const DeviceDatabaseVersion = deviceDatabaseVersion

// DeviceFromTocall gives the best match for a destination callsign: exact characters beat ? (any
// character) and n (a digit), which beat a trailing * (anything).
func DeviceFromTocall(callsign string) (Device, bool) {
	var best *tocallEntry
	var bestScore [2]int
	for i := range tocalls {
		e := &tocalls[i]
		literal, exact, ok := tocallScore(e.pattern, callsign)
		if !ok {
			continue
		}
		s := [2]int{literal, exact}
		// The last of equal scores wins.
		if best == nil || s[0] > bestScore[0] || (s[0] == bestScore[0] && s[1] >= bestScore[1]) {
			best, bestScore = e, s
		}
	}
	if best == nil {
		return Device{}, false
	}
	return best.device.clone(), true
}

// Device gives the sending device or program, if the database knows it: from the Mic-E type
// code and suffix for Mic-E, otherwise from the destination address.
func (p *Packet) Device() (Device, bool) {
	if m, ok := p.Data.(MicEReport); ok {
		if m.TypeCode == nil {
			return Device{}, false
		}
		switch prefix := *m.TypeCode; prefix {
		case '>', ']':
			for _, l := range micELegacy {
				if len(l.prefix) > 0 && rune(l.prefix[0]) == prefix && l.suffix == m.DeviceSuffix {
					return l.device.clone(), true
				}
			}
		case '`', '\'':
			for _, s := range micESuffixes {
				if s.suffix == m.DeviceSuffix {
					return s.device.clone(), true
				}
			}
		}
		return Device{}, false
	}
	return DeviceFromTocall(p.Destination.Callsign())
}

func tocallScore(pattern, call string) (int, int, bool) {
	literal := 0
	for i := 0; i < len(pattern); i++ {
		p := pattern[i]
		if p == '*' {
			return literal, 0, len(call) >= i
		}
		if i >= len(call) {
			return 0, 0, false
		}
		c := call[i]
		switch {
		case p == '?':
		case p == 'n':
			if !isDigit(c) {
				return 0, 0, false
			}
		case p == c:
			literal++
		default:
			return 0, 0, false
		}
	}
	return literal, 1, len(call) == len(pattern)
}

// micESuffixLen says how many bytes at the end of a Mic-E status text are a device suffix known
// for this type code.
func micESuffixLen(typeCode *byte, status []byte) int {
	if typeCode == nil {
		return 0
	}
	switch p := *typeCode; p {
	case '`', '\'':
		if len(status) >= 2 {
			tail := string(status[len(status)-2:])
			for _, s := range micESuffixes {
				if s.suffix == tail {
					return 2
				}
			}
		}
	case '>', ']':
		if len(status) > 0 {
			last := status[len(status)-1]
			for _, l := range micELegacy {
				if l.prefix == string([]byte{p}) && l.hasSuffix && l.suffix == string([]byte{last}) {
					return 1
				}
			}
		}
	}
	return 0
}
