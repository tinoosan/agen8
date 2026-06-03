package bytesutil

// TrimRightNewlines removes trailing '\n' and '\r' bytes from b.
// It returns a subslice of b (it does not allocate).
func TrimRightNewlines(b []byte) []byte {
	for len(b) > 0 {
		last := b[len(b)-1]
		if last != '\n' && last != '\r' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
