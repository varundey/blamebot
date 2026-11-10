package main

import "strings"

const (
	boldRed = "\x1b[1;31m"
	yellow  = "\x1b[33m"
	reset   = "\x1b[0m"
)

// highlightVerdict makes the verdict line of a reply stand out: a line that
// begins with "BLAME:" is printed bold red, and one that begins with
// "NO ONE TO BLAME" is printed yellow. Everything else is left alone, so a
// BLAME: that appears mid-line (say, inside quoted evidence) is not touched.
func highlightVerdict(reply string) string {
	lines := strings.Split(reply, "\n")
	for i, line := range lines {
		switch trimmed := strings.TrimLeft(line, " "); {
		case strings.HasPrefix(trimmed, "BLAME:"):
			lines[i] = boldRed + "🚨 " + trimmed + reset
		case strings.HasPrefix(trimmed, "NO ONE TO BLAME"):
			lines[i] = yellow + trimmed + reset
		}
	}
	return strings.Join(lines, "\n")
}
