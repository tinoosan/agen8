package kit

import "strings"

// CommandPalette manages the state of a slash-command autocomplete palette.
type CommandPalette struct {
	Open     bool
	Matches  []string
	Selected int
}

// Update recomputes palette state from the given input and command list.
// isExact returns true if a token exactly matches a known command.
// Returns true if any state changed (caller may need to re-layout).
func (p *CommandPalette) Update(input string, commands []string, isExact func(string) bool) bool {
	prevOpen, prevLen, prevSel := p.Open, len(p.Matches), p.Selected

	fields := strings.Fields(input)
	firstToken := strings.TrimSpace(input)
	if len(fields) > 0 {
		firstToken = fields[0]
	}

	if !strings.HasPrefix(firstToken, "/") ||
		(isExact(firstToken) && strings.ContainsAny(input, " \t\n")) {
		p.Open, p.Matches, p.Selected = false, nil, 0
	} else {
		var matches []string
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, firstToken) {
				matches = append(matches, cmd)
			}
		}
		if len(matches) > 0 {
			p.Open = true
			p.Matches = matches
			if p.Selected < 0 || p.Selected >= len(matches) {
				p.Selected = 0
			}
		} else {
			p.Open, p.Matches, p.Selected = false, nil, 0
		}
	}
	return p.Open != prevOpen || len(p.Matches) != prevLen || p.Selected != prevSel
}

// Navigate moves the selection by delta, clamped to valid range.
func (p *CommandPalette) Navigate(delta int) {
	if len(p.Matches) == 0 {
		return
	}
	p.Selected = clampInt(p.Selected+delta, 0, len(p.Matches)-1)
}

// Autocomplete returns the input with the first token replaced by the selected command.
// If trailingSpace is true, a trailing space is appended.
func (p *CommandPalette) Autocomplete(input string, trailingSpace bool) (string, bool) {
	if !p.Open || len(p.Matches) == 0 {
		return input, false
	}
	if p.Selected < 0 || p.Selected >= len(p.Matches) {
		return input, false
	}
	selected := p.Matches[p.Selected]
	fields := strings.Fields(input)
	var newVal string
	if len(fields) == 0 {
		newVal = selected
	} else {
		rest := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
		if rest != "" {
			newVal = selected + " " + rest
		} else {
			newVal = selected
		}
	}
	if trailingSpace && !strings.HasSuffix(newVal, " ") {
		newVal += " "
	}
	return newVal, true
}

// Reset closes the palette and clears state.
func (p *CommandPalette) Reset() {
	p.Open = false
	p.Matches = nil
	p.Selected = 0
}
