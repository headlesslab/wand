package proto

import (
	"regexp"
	"strings"
)

// PatternToReg FetchRequestPattern.URLPattern to regular expression, matching
// what the browser matches: a run of wildcards matches any run of characters
// when it has a "*" in it and up to one character per "?" otherwise (the
// browser's "?" matches no character as well as one), the character after a
// backslash matches itself, a backslash at the end matches nothing, and every
// other character matches itself.
func PatternToReg(pattern string) string {
	if pattern == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(`\A`)
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*', '?':
			j := i
			for j < len(pattern) && (pattern[j] == '*' || pattern[j] == '?') {
				j++
			}
			if strings.Contains(pattern[i:j], "*") {
				b.WriteString(".*")
			} else {
				b.WriteString(strings.Repeat(".?", j-i))
			}
			i = j - 1
		case '\\':
			if i+1 < len(pattern) {
				i++
				b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			}
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString(`\z`)
	return b.String()
}
