package aprs

// context collects diagnostics while decoding, and applies the parse options.
type context struct {
	options     ParseOptions
	diagnostics []Diagnostic
	// depth is how many third-party headers enclose the packet being read.
	depth int
}

func newContext(options ParseOptions) *context {
	return &context{options: options}
}

func (c *context) clone() *context {
	d := make([]Diagnostic, len(c.diagnostics))
	copy(d, c.diagnostics)
	return &context{options: c.options, diagnostics: d, depth: c.depth}
}

func (c *context) push(severity Severity, code Code, message string, offset int) {
	c.diagnostics = append(c.diagnostics, Diagnostic{Severity: severity, Code: code, Message: message, Offset: offset})
}

func (c *context) info(code Code, message string, offset int) {
	c.push(Info, code, message, offset)
}

func (c *context) warn(code Code, message string, offset int) {
	c.push(Warning, code, message, offset)
}

func (c *context) error(code Code, message string, offset int) {
	c.push(Error, code, message, offset)
}

// tolerate handles a defect the options may tolerate: a warning and true if they do, an error and
// false if not.
func (c *context) tolerate(code Code, message string, offset int) bool {
	if c.options.Tolerates(code) {
		c.warn(code, message, offset)
		return true
	}
	c.error(code, message, offset)
	return false
}

// allows says whether an extra reading (one that is not about a defect) is allowed; it warns when
// it is.
func (c *context) allows(code Code, message string, offset int) bool {
	if c.options.Tolerates(code) {
		c.warn(code, message, offset)
		return true
	}
	return false
}

func (c *context) hasErrors() bool {
	for _, d := range c.diagnostics {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

func (c *context) headerError() *HeaderError {
	d := make([]Diagnostic, len(c.diagnostics))
	copy(d, c.diagnostics)
	return &HeaderError{Diagnostics: d}
}

// none is the offset of a diagnostic that refers to no one place.
const none = -1
