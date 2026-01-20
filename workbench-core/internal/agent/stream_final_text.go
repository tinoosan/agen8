package agent

import "unicode/utf8"

// finalTextStreamDecoder incrementally decodes only the {"op":"final","text":"..."} text field
// from a streamed JSON object and yields decoded text as it becomes available.
//
// This is a best-effort streaming decoder tailored to the Workbench agent contract:
// top-level JSON object with string fields "op" and "text". It avoids emitting the raw
// JSON to the UI.
type finalTextStreamDecoder struct {
	state decodeState

	curKey string
	keyBuf []byte

	keyStr jsonStringParser
	valStr jsonStringParser

	target valueTarget

	opBuf     []byte
	opKnown   bool
	opIsFinal bool

	// If "text" arrives before we know op=="final", buffer decoded text here (bounded).
	pendingText []byte

	// Keep incomplete UTF-8 fragments across chunks.
	utf8Tail []byte

	// skipping non-string values (best-effort)
	skipDepth  int
	skipInStr  bool
	skipEscape bool
}

type decodeState int

const (
	stSeekObjStart decodeState = iota
	stSeekKey
	stReadKey
	stSeekColon
	stSeekValue
	stReadStringValue
	stSkipValue
	stDone
)

type valueTarget int

const (
	tOther valueTarget = iota
	tOp
	tText
)

const maxPendingTextBytes = 16 * 1024

// Consume consumes a chunk of streamed JSON bytes and returns newly decoded final.text bytes.
func (d *finalTextStreamDecoder) Consume(in string) string {
	if d == nil || in == "" {
		return ""
	}

	var emitted []byte
	flushPending := func() {
		if d.opKnown && d.opIsFinal && len(d.pendingText) != 0 {
			emitted = append(emitted, d.pendingText...)
			d.pendingText = nil
		}
	}

	b := []byte(in)
	for i := 0; i < len(b); i++ {
		ch := b[i]

	reprocess:
		switch d.state {
		case stSeekObjStart:
			if ch == '{' {
				d.state = stSeekKey
			}

		case stSeekKey:
			switch ch {
			case ' ', '\t', '\r', '\n', ',':
				continue
			case '}':
				d.state = stDone
				flushPending()
				continue
			case '"':
				d.keyStr.Reset()
				d.keyBuf = d.keyBuf[:0]
				d.state = stReadKey
				continue
			default:
				continue
			}

		case stReadKey:
			done, out := d.keyStr.Step(ch)
			if len(out) != 0 {
				d.keyBuf = append(d.keyBuf, out...)
			}
			if done {
				d.curKey = string(d.keyBuf)
				d.state = stSeekColon
			}

		case stSeekColon:
			switch ch {
			case ' ', '\t', '\r', '\n':
				continue
			case ':':
				d.state = stSeekValue
			default:
				continue
			}

		case stSeekValue:
			switch ch {
			case ' ', '\t', '\r', '\n':
				continue
			case '"':
				d.valStr.Reset()
				d.target = tOther
				switch d.curKey {
				case "op":
					d.target = tOp
					d.opBuf = d.opBuf[:0]
				case "text":
					d.target = tText
				}
				d.state = stReadStringValue
			default:
				// Start skipping a non-string value and process this byte as part of the skip.
				d.startSkipValue(ch)
				d.state = stSkipValue
				// We already consumed the first value byte into skip state.
				continue
			}

		case stReadStringValue:
			done, out := d.valStr.Step(ch)
			if len(out) != 0 {
				switch d.target {
				case tText:
					if d.opKnown && d.opIsFinal {
						emitted = append(emitted, out...)
					} else if len(d.pendingText) < maxPendingTextBytes {
						remaining := maxPendingTextBytes - len(d.pendingText)
						if len(out) > remaining {
							out = out[:remaining]
						}
						d.pendingText = append(d.pendingText, out...)
					}
				case tOp:
					d.opBuf = append(d.opBuf, out...)
				default:
					// ignore
				}
			}
			if done {
				if d.target == tOp {
					d.opKnown = true
					d.opIsFinal = string(d.opBuf) == "final"
					if !d.opIsFinal {
						// Not a final response; drop any buffered text.
						d.pendingText = nil
					} else {
						flushPending()
					}
				} else if d.target == tText {
					flushPending()
				}
				d.state = stSeekKey
			}

		case stSkipValue:
			if d.processSkipByte(ch, &i) {
				// processSkipByte moved state; reprocess current byte if requested.
				if d.state == stSeekKey || d.state == stDone {
					goto reprocess
				}
			}

		case stDone:
			continue
		}
	}

	if len(emitted) == 0 && len(d.utf8Tail) == 0 {
		return ""
	}
	combined := append([]byte(nil), d.utf8Tail...)
	combined = append(combined, emitted...)
	head, tail := splitValidUTF8Prefix(combined)
	d.utf8Tail = append(d.utf8Tail[:0], tail...)
	return string(head)
}

func (d *finalTextStreamDecoder) startSkipValue(first byte) {
	d.skipInStr = false
	d.skipEscape = false
	d.skipDepth = 0
	switch first {
	case '{', '[':
		d.skipDepth = 1
	case '"':
		// should not happen (string values handled elsewhere)
		d.skipInStr = true
	default:
		// primitive
	}
	// Note: first byte is already consumed; remaining bytes are handled in stSkipValue.
}

// processSkipByte updates skip state and returns true when it changes d.state.
// It may decrement *i to reprocess a separator in the next state.
func (d *finalTextStreamDecoder) processSkipByte(ch byte, i *int) bool {
	if d.skipInStr {
		if d.skipEscape {
			d.skipEscape = false
			return false
		}
		if ch == '\\' {
			d.skipEscape = true
			return false
		}
		if ch == '"' {
			d.skipInStr = false
		}
		return false
	}
	if ch == '"' {
		d.skipInStr = true
		return false
	}
	if d.skipDepth > 0 {
		switch ch {
		case '{', '[':
			d.skipDepth++
		case '}', ']':
			d.skipDepth--
		}
		if d.skipDepth == 0 {
			d.state = stSeekKey
			return true
		}
		return false
	}
	// primitive value: end on ',' or '}'.
	if ch == ',' {
		d.state = stSeekKey
		// Reprocess comma in stSeekKey.
		*i--
		return true
	}
	if ch == '}' {
		d.state = stSeekKey
		// Reprocess '}' in stSeekKey (it will transition to done).
		*i--
		return true
	}
	return false
}

// jsonStringParser incrementally decodes JSON string content (inside quotes) and returns decoded bytes.
// Step expects to be fed bytes after the opening quote (") and will return done=true when it consumes
// the closing quote.
type jsonStringParser struct {
	escape      bool
	unicodeN    int
	unicodeVal  int
	pendingHigh int
}

func (p *jsonStringParser) Reset() { *p = jsonStringParser{} }

func (p *jsonStringParser) Step(b byte) (done bool, out []byte) {
	if !p.escape && p.unicodeN == 0 && b == '"' {
		return true, nil
	}
	if p.unicodeN > 0 {
		if !isHex(b) {
			p.unicodeN = 0
			p.unicodeVal = 0
			p.pendingHigh = 0
			return false, []byte("\uFFFD")
		}
		p.unicodeVal = (p.unicodeVal << 4) | hexVal(b)
		p.unicodeN--
		if p.unicodeN > 0 {
			return false, nil
		}
		r := p.unicodeVal
		p.unicodeVal = 0
		// surrogate pair handling
		if p.pendingHigh != 0 {
			hi := p.pendingHigh
			p.pendingHigh = 0
			if r >= 0xDC00 && r <= 0xDFFF {
				code := 0x10000 + ((hi - 0xD800) << 10) + (r - 0xDC00)
				return false, runeBytes(rune(code))
			}
			// invalid pair
			out = append(out, []byte("\uFFFD")...)
			out = append(out, runeBytes(rune(r))...)
			return false, out
		}
		if r >= 0xD800 && r <= 0xDBFF {
			p.pendingHigh = r
			return false, nil
		}
		return false, runeBytes(rune(r))
	}
	if p.escape {
		p.escape = false
		switch b {
		case '"', '\\', '/':
			return false, []byte{b}
		case 'b':
			return false, []byte{'\b'}
		case 'f':
			return false, []byte{'\f'}
		case 'n':
			return false, []byte{'\n'}
		case 'r':
			return false, []byte{'\r'}
		case 't':
			return false, []byte{'\t'}
		case 'u':
			p.unicodeN = 4
			p.unicodeVal = 0
			return false, nil
		default:
			return false, []byte{b}
		}
	}
	if b == '\\' {
		p.escape = true
		return false, nil
	}
	return false, []byte{b}
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexVal(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return 10 + int(b-'a')
	case b >= 'A' && b <= 'F':
		return 10 + int(b-'A')
	default:
		return 0
	}
}

func runeBytes(r rune) []byte {
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	return append([]byte(nil), buf[:n]...)
}

func splitValidUTF8Prefix(b []byte) (head []byte, tail []byte) {
	if len(b) == 0 {
		return nil, nil
	}
	if utf8.Valid(b) {
		return b, nil
	}
	// Only the last few bytes can be an incomplete rune.
	start := len(b) - utf8.UTFMax
	if start < 0 {
		start = 0
	}
	for cut := len(b); cut >= start; cut-- {
		if utf8.Valid(b[:cut]) {
			return b[:cut], b[cut:]
		}
	}
	return nil, b
}
