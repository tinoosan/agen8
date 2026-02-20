package agent

import (
	"bytes"
	"encoding/xml"
	"strings"
)

type xmlAttribute struct {
	key   string
	value string
}

func buildXMLBlock(tag string, attrs []xmlAttribute, content string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n<")
	b.WriteString(tag)
	for _, a := range attrs {
		k := strings.TrimSpace(a.key)
		if k == "" {
			continue
		}
		b.WriteString(" ")
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(escapeXMLAttr(a.value))
		b.WriteString("\"")
	}
	b.WriteString(">\n")
	b.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">")
	return b.String()
}

func escapeXMLAttr(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
