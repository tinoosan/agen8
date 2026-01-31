package store

import (
	"strings"
	"unicode"
)

type MemoryChunk struct {
	Index   int
	Content string
	Start   int
	End     int
}

type ChunkStrategy interface {
	Chunk(content string) []MemoryChunk
}

// ParagraphChunker splits by double newlines and then enforces a max size.
type ParagraphChunker struct {
	MaxChunkSize int
}

func (c *ParagraphChunker) Chunk(content string) []MemoryChunk {
	max := c.MaxChunkSize
	if max <= 0 {
		max = 1000
	}
	out := []MemoryChunk{}

	offset := 0
	for offset < len(content) {
		next := strings.Index(content[offset:], "\n\n")
		paraStart := offset
		paraEnd := len(content)
		if next >= 0 {
			paraEnd = offset + next
		}
		raw := content[paraStart:paraEnd]
		offset = paraEnd
		if next >= 0 {
			offset += 2
		}

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		leading := strings.Index(raw, trimmed)
		if leading < 0 {
			leading = 0
		}
		baseStart := paraStart + leading

		if len(trimmed) <= max {
			out = append(out, MemoryChunk{
				Content: trimmed,
				Start:   baseStart,
				End:     baseStart + len(trimmed),
			})
			continue
		}

		subChunks := chunkByWordsWithOffsets(trimmed, max)
		for _, sub := range subChunks {
			out = append(out, MemoryChunk{
				Content: sub.Content,
				Start:   baseStart + sub.Start,
				End:     baseStart + sub.End,
			})
		}
	}

	for i := range out {
		out[i].Index = i
	}
	return out
}

// TimeBasedChunker splits by timestamp-like lines, with a size cap fallback.
type TimeBasedChunker struct {
	MaxChunkSize int
}

func (c *TimeBasedChunker) Chunk(content string) []MemoryChunk {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}
	max := c.MaxChunkSize
	if max <= 0 {
		max = 1000
	}

	type seg struct {
		Start int
		End   int
		Text  string
	}
	segments := []seg{}

	offset := 0
	segStart := 0
	for i, line := range lines {
		lineStart := offset
		lineEnd := lineStart + len(line)
		offset = lineEnd + 1

		if i > 0 && isTimeStampLine(line) {
			segments = append(segments, seg{
				Start: segStart,
				End:   lineStart - 1,
				Text:  content[segStart : lineStart-1],
			})
			segStart = lineStart
		}
	}
	if segStart < len(content) {
		segments = append(segments, seg{
			Start: segStart,
			End:   len(content),
			Text:  content[segStart:],
		})
	}

	out := []MemoryChunk{}
	for _, s := range segments {
		trimmed := strings.TrimSpace(s.Text)
		if trimmed == "" {
			continue
		}
		leading := strings.Index(s.Text, trimmed)
		if leading < 0 {
			leading = 0
		}
		baseStart := s.Start + leading

		if len(trimmed) <= max {
			out = append(out, MemoryChunk{
				Content: trimmed,
				Start:   baseStart,
				End:     baseStart + len(trimmed),
			})
			continue
		}

		subChunks := chunkByWordsWithOffsets(trimmed, max)
		for _, sub := range subChunks {
			out = append(out, MemoryChunk{
				Content: sub.Content,
				Start:   baseStart + sub.Start,
				End:     baseStart + sub.End,
			})
		}
	}

	for i := range out {
		out[i].Index = i
	}
	return out
}

func isTimeStampLine(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) < 5 || line[2] != ':' {
		return false
	}
	h1, h2 := line[0], line[1]
	m1, m2 := line[3], line[4]
	if !isDigit(h1) || !isDigit(h2) || !isDigit(m1) || !isDigit(m2) {
		return false
	}
	return true
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

type subChunk struct {
	Content string
	Start   int
	End     int
}

func chunkByWordsWithOffsets(text string, max int) []subChunk {
	if max <= 0 || len(text) <= max {
		return []subChunk{{Content: strings.TrimSpace(text), Start: 0, End: len(strings.TrimSpace(text))}}
	}

	out := []subChunk{}
	i := 0
	for i < len(text) {
		for i < len(text) && unicode.IsSpace(rune(text[i])) {
			i++
		}
		if i >= len(text) {
			break
		}
		chunkStart := i
		chunkEnd := i
		lastWordEnd := i

		for i < len(text) {
			wordStart := i
			for i < len(text) && !unicode.IsSpace(rune(text[i])) {
				i++
			}
			wordEnd := i
			if wordEnd-chunkStart > max && lastWordEnd > chunkStart {
				break
			}
			lastWordEnd = wordEnd
			chunkEnd = wordEnd
			for i < len(text) && unicode.IsSpace(rune(text[i])) {
				i++
			}
			if chunkEnd-chunkStart >= max {
				break
			}
			if i >= len(text) {
				break
			}
			if i-wordStart > max {
				break
			}
		}

		if lastWordEnd == chunkStart {
			lastWordEnd = minInt(chunkStart+max, len(text))
		}
		out = append(out, subChunk{
			Content: strings.TrimSpace(text[chunkStart:lastWordEnd]),
			Start:   chunkStart,
			End:     lastWordEnd,
		})
		i = lastWordEnd
	}

	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
