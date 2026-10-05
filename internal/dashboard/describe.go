package dashboard

import (
	"bufio"
	"os"
	"strings"
)

// DescribeEntry is the type and human-readable description of a metric
// family, as listed by "systemd-report describe".
type DescribeEntry struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ParseDescribe reads the fixed-width table produced by
// "systemd-report describe" (a header line with FAMILY/TYPE/DESCRIPTION
// columns, followed by one row per metric family) into a map keyed by
// family name.
func ParseDescribe(path string) (map[string]DescribeEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]DescribeEntry)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return out, scanner.Err()
	}

	header := []rune(scanner.Text())
	typeCol := runeIndex(header, "TYPE")
	descCol := runeIndex(header, "DESCRIPTION")
	if typeCol < 0 || descCol < 0 {
		return out, scanner.Err()
	}

	for scanner.Scan() {
		line := []rune(scanner.Text())
		if strings.TrimSpace(string(line)) == "" {
			continue
		}
		if len(line) > 0 && strings.TrimSpace(string(line[0])) == "" {
			continue
		}

		family := strings.TrimSpace(sliceRunes(line, 0, typeCol))
		if family == "" {
			continue
		}
		typ := strings.TrimSpace(sliceRunes(line, typeCol, descCol))
		desc := strings.TrimSpace(sliceRunes(line, descCol, len(line)))
		out[family] = DescribeEntry{Type: typ, Description: desc}
	}

	return out, scanner.Err()
}

// runeIndex returns the index of the first occurrence of substr in s (both
// measured in runes), or -1 if not found.
func runeIndex(s []rune, substr string) int {
	sub := []rune(substr)
	if len(sub) == 0 || len(sub) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// sliceRunes returns s[start:end], clamped to s's bounds, mirroring Python's
// tolerant slice semantics for out-of-range indices.
func sliceRunes(s []rune, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	if start >= end {
		return ""
	}
	return string(s[start:end])
}
