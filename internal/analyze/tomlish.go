package analyze

import (
	"strings"
)

// tomlSections is a pragmatic line-based TOML reader. It only understands
// what detection needs: [section] headers and key = "value" pairs.
type tomlSections map[string]map[string]string

// parseTOMLish parses content into section -> key -> value. Keys outside any
// section land under "".
func parseTOMLish(content string) tomlSections {
	out := tomlSections{}
	current := ""
	out[current] = map[string]string{}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.Trim(line, "[]")
			current = strings.Trim(current, "\"'")
			if _, ok := out[current]; !ok {
				out[current] = map[string]string{}
			}
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = stripInlineComment(val)
		val = strings.Trim(val, "\"'")
		out[current][key] = val
	}
	return out
}

// stripInlineComment removes a trailing " # comment" outside quotes.
func stripInlineComment(v string) string {
	inStr := false
	var quote byte
	for i := 0; i < len(v); i++ {
		c := v[i]
		if inStr {
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			quote = c
		case '#':
			return strings.TrimSpace(v[:i])
		}
	}
	return v
}

// yamlKeysFromList extracts `- item` list entries under a `key:` line
// (used for pnpm-workspace.yaml packages).
func yamlKeysFromList(content, key string) []string {
	var out []string
	inKey := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, key+":") {
			inKey = true
			continue
		}
		if inKey {
			if strings.HasPrefix(line, "- ") {
				out = append(out, strings.Trim(strings.TrimSpace(line[2:]), "'\""))
				continue
			}
			if line != "" && !strings.HasPrefix(line, "#") {
				inKey = false
			}
		}
	}
	return out
}
