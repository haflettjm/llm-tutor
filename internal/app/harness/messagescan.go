package harness

import (
	"strconv"
	"strings"
)

const messagePrefix = `"message":"`

type messageScanner struct {
	prefix        string
	inValue       bool
	done          bool
	escaped       bool
	unicode       string
	unicodeActive bool
}

// Feed returns newly decoded text from the structured response's message value.
func (s *messageScanner) Feed(fragment string) string {
	if s.done {
		return ""
	}
	if !s.inValue {
		s.prefix += fragment
		index := strings.Index(s.prefix, messagePrefix)
		if index < 0 {
			if len(s.prefix) >= len(messagePrefix) {
				s.prefix = s.prefix[len(s.prefix)-len(messagePrefix)+1:]
			}
			return ""
		}
		s.inValue = true
		fragment = s.prefix[index+len(messagePrefix):]
		s.prefix = ""
	}

	var text strings.Builder
	for i := 0; i < len(fragment); i++ {
		ch := fragment[i]
		if s.unicodeActive {
			s.unicode += string(ch)
			if len(s.unicode) == 4 {
				decoded, err := strconv.Unquote(`"\u` + s.unicode + `"`)
				if err == nil {
					text.WriteString(decoded)
				}
				s.unicode = ""
				s.unicodeActive = false
			}
			continue
		}
		if s.escaped {
			s.escaped = false
			switch ch {
			case 'b':
				text.WriteByte('\b')
			case 'f':
				text.WriteByte('\f')
			case 'n':
				text.WriteByte('\n')
			case 'r':
				text.WriteByte('\r')
			case 't':
				text.WriteByte('\t')
			case 'u':
				s.unicode = ""
				s.unicodeActive = true
			default:
				text.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '\\':
			s.escaped = true
		case '"':
			s.done = true
			return text.String()
		default:
			text.WriteByte(ch)
		}
	}
	return text.String()
}
