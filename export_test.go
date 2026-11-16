package aprs

// SetWriterFault makes every encoder's bytes pass through fault before the read-back sees them,
// as a field writer that wrote a wrong byte would, and gives back what undoes it.
func SetWriterFault(fault func(b []byte) []byte) (restore func()) {
	writerFault = fault
	return func() { writerFault = nil }
}
