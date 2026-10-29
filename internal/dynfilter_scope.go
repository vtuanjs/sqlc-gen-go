package golang

import "strings"

// dynParenDeltas counts SQL parentheses per line, excluding quoted values,
// identifiers and comments. Scan the whole query so lexical state survives
// newlines. These counts describe scope only; the original SQL is not changed.
func dynParenDeltas(sql, engine string) []int {
	deltas := make([]int, strings.Count(sql, "\n")+1)
	line := 0
	pg := engine != "mysql" && engine != "sqlite"
	mysql := engine == "mysql"
	// PostgreSQL continues a string across whitespace containing a newline;
	// an E prefix applies to all of its continued pieces.
	var previousString, previousEscape, separatorNewline bool

	advance := func(start, end int) int {
		line += strings.Count(sql[start:end], "\n")
		return end
	}
	for i := 0; i < len(sql); {
		switch c := sql[i]; {
		case c == '\n':
			if previousString {
				separatorNewline = true
			}
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-' &&
			(!mysql || i+2 == len(sql) || sql[i+2] <= ' '):
			// Leave the newline for the next iteration.
			if n := strings.IndexByte(sql[i:], '\n'); n >= 0 {
				i += n
			} else {
				i = len(sql)
			}
		case mysql && c == '#':
			if n := strings.IndexByte(sql[i:], '\n'); n >= 0 {
				i += n
			} else {
				i = len(sql)
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			// PostgreSQL allows line comments, but not block comments, between
			// continued string pieces.
			previousString = false
			depth, j := 1, i+2
			for j < len(sql) && depth > 0 {
				switch {
				case pg && j+1 < len(sql) && sql[j:j+2] == "/*":
					depth++
					j += 2
				case j+1 < len(sql) && sql[j:j+2] == "*/":
					depth--
					j += 2
				default:
					j++
				}
			}
			i = advance(i, j)
		case c == '\'' || c == '"' || (!pg && c == '`'):
			backslash := mysql && c != '`' || pg && c == '\'' && previousString && previousEscape && separatorNewline
			previousString = false
			if pg && c == '\'' && i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') &&
				(i == 1 || !dynScopeIdentByte(sql[i-2])) {
				backslash = true
			}
			closed := false
			j := i + 1
			for j < len(sql) {
				if backslash && sql[j] == '\\' && j+1 < len(sql) {
					j += 2
					continue
				}
				if sql[j] == c {
					j++
					if j < len(sql) && sql[j] == c {
						j++
						continue
					}
					closed = true
					break
				}
				j++
			}
			i = advance(i, j)
			previousString = c == '\'' && closed
			previousEscape = backslash
			separatorNewline = false
		case engine == "sqlite" && c == '[':
			previousString = false
			j := len(sql)
			if n := strings.IndexByte(sql[i+1:], ']'); n >= 0 {
				j = i + 2 + n
			}
			i = advance(i, j)
		case pg && c == '$' && (i == 0 || !dynScopeIdentByte(sql[i-1])):
			previousString = false
			// Tags follow identifier rules, except that '$' closes the tag.
			j := i + 1
			for j < len(sql) && sql[j] != '$' &&
				(sql[j] == '_' || sql[j] >= 128 || sql[j] >= 'a' && sql[j] <= 'z' ||
					sql[j] >= 'A' && sql[j] <= 'Z' || j > i+1 && sql[j] >= '0' && sql[j] <= '9') {
				j++
			}
			if j < len(sql) && sql[j] == '$' {
				delim := sql[i : j+1]
				end := len(sql)
				if n := strings.Index(sql[j+1:], delim); n >= 0 {
					end = j + 1 + n + len(delim)
				}
				i = advance(i, end)
			} else {
				i++
			} // e.g. $1 is a bind parameter, not a quoted span
		case c == '(':
			previousString = false
			deltas[line]++
			i++
		case c == ')':
			previousString = false
			deltas[line]--
			i++
		default:
			previousString = false
			i++
		}
	}
	return deltas
}

func dynScopeIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= 128 ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
