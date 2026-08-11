package golang

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

// ifAnnotationRe matches "-- :if @p1 [@p2 ...]" or "-- :if $p1 [$p2 ...]" at end of line.
var ifAnnotationRe = regexp.MustCompile(`--\s*:if\s+[@$]\w+(?:\s+[@$]\w+)*\s*$`)

// ifParamRe extracts individual @name or $name tokens from an annotation.
var ifParamRe = regexp.MustCompile(`[@$](\w+)`)

var sqlcSliceRe = regexp.MustCompile(`/\*SLICE:([^*]+)\*/\?`)

// parseIfNames returns all param names listed in a :if annotation string.
func parseIfNames(annotation string) []string {
	matches := ifParamRe.FindAllStringSubmatch(annotation, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

// DynFilterInfo holds the result of parsing :if annotations from a SQL query.
type DynFilterInfo struct {
	// AnnotatedSQL is the SQL with -- :if replaced by -- :dynif N markers.
	// N is the 0-based index into the full args slice passed to DynamicSQL.
	// For SQL params: N = paramNumber - 1  (matches $N position in args).
	// For flag-only params: N = numSQLParams + flagOffset.
	AnnotatedSQL string
	// ConditionalParamNumbers contains the $N numbers (1-based) of SQL params
	// that are conditional and should become pointer types.
	ConditionalParamNumbers []int
	// FlagParams are extra bool params that need to be added to the params struct
	// (params referenced in :if that are not actual SQL params).
	FlagParams []FlagParam
	// OrderedArgNames is the full ordered list of param names for the DynamicSQL
	// call, indexed by their :dynif N value.
	// Positions 0..numSQLParams-1 are SQL params (in $N order, all of them).
	// Positions numSQLParams.. are flag-only params (in appearance order).
	OrderedArgNames []string
}

// FlagParam represents a flag-only bool parameter (used in ORDER BY :if).
type FlagParam struct {
	// Name is the original @name from the :if annotation.
	Name string
	// GoName is the CamelCase Go field name.
	GoName string
}

// ParseDynFilter parses -- :if @param [... @paramN] annotations from SQL query text.
// params is the list of ALL SQL parameters (from sqlc).
//
// The :dynif N index assigned to each annotation equals paramNumber-1 for
// SQL params, so that the DynamicSQL runtime can directly use args[N] ↔ $N+1.
// Flag-only params (ORDER BY flags not in SQL) get indices starting at
// len(params), and are appended to the DynamicSQL args after the SQL params.
//
// Both inline and block syntax are supported:
//
//	AND b = $2 -- :if @b              (inline, single)
//	AND b = $2 -- :if @b @c           (inline, multi — line removed if any is inactive)
//	-- :if @b                          (block: applies to next line)
//	AND b = $2
func ParseDynFilter(sql string, params []*plugin.Parameter) (*DynFilterInfo, error) {
	// Build map: column name -> param number (1-based)
	paramByName := make(map[string]int32)
	for _, p := range params {
		if p.Column.Name != "" {
			paramByName[p.Column.Name] = p.Number
		}
	}

	sql = numberSqlcSlices(sql, paramByName)

	lines := strings.Split(sql, "\n")

	// First pass: collect all :if annotations to find which params are
	// conditional and which are flag-only.
	type refEntry struct {
		name        string
		isFlagOnly  bool
		paramNumber int32 // only valid if !isFlagOnly; equals the $N number
	}
	seenName := make(map[string]bool)
	var refs []refEntry

	for _, line := range lines {
		loc := ifAnnotationRe.FindStringIndex(line)
		if loc == nil {
			continue
		}
		for _, name := range parseIfNames(line[loc[0]:]) {
			if seenName[name] {
				continue
			}
			seenName[name] = true
			if paramNum, ok := paramByName[name]; ok {
				refs = append(refs, refEntry{name: name, isFlagOnly: false, paramNumber: paramNum})
			} else {
				refs = append(refs, refEntry{name: name, isFlagOnly: true})
			}
		}
	}

	if len(refs) == 0 {
		return nil, nil
	}

	// Build name -> :dynif index mapping.
	// SQL params: index = paramNumber - 1  (0-based, matches $N position in args)
	// Flag params: index = len(params) + flagOffset  (appended after SQL params)
	argIndexByName := make(map[string]int)
	conditionalParamNums := make(map[int32]bool)
	var flagParams []FlagParam
	flagOffset := 0
	for _, r := range refs {
		if !r.isFlagOnly {
			argIndexByName[r.name] = int(r.paramNumber) - 1
			conditionalParamNums[r.paramNumber] = true
		} else {
			argIndexByName[r.name] = len(params) + flagOffset
			flagParams = append(flagParams, FlagParam{
				Name:   r.name,
				GoName: structName(r.name),
			})
			flagOffset++
		}
	}

	// buildSuffix converts a list of param names into " -- :if $N [-- :if $M ...]".
	buildSuffix := func(names []string) (string, error) {
		var parts []string
		for _, name := range names {
			idx, ok := argIndexByName[name]
			if !ok {
				return "", fmt.Errorf("dynfilter: unknown param @%s", name)
			}
			parts = append(parts, fmt.Sprintf("-- :if $%d", idx+1))
		}
		return " " + strings.Join(parts, " "), nil
	}

	// Second pass: rewrite the SQL, replacing -- :if @name... with -- :if $N...
	var newLines []string
	// blockSuffix is the annotation suffix to propagate to lines inside a
	// multi-line paren block; "" means inactive.
	// blockDepth tracks net open parens: when it drops to ≤ 0 the block ends.
	blockSuffix := ""
	blockDepth := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// Propagate annotation to lines inside an open paren block.
		if blockSuffix != "" {
			blockDepth += strings.Count(line, "(") - strings.Count(line, ")")
			// If this line has its own inline annotation, convert it first so the
			// generated line carries both conditions.
			if loc := ifAnnotationRe.FindStringIndex(line); loc != nil {
				innerSuffix, err := buildSuffix(parseIfNames(line[loc[0]:]))
				if err != nil {
					return nil, err
				}
				line = strings.TrimRight(line[:loc[0]], " \t") + innerSuffix
			}
			newLines = append(newLines, strings.TrimRight(line, " \t")+blockSuffix)
			if blockDepth <= 0 {
				blockSuffix = ""
				blockDepth = 0
			}
			continue
		}

		loc := ifAnnotationRe.FindStringIndex(line)
		if loc == nil {
			newLines = append(newLines, line)
			continue
		}

		names := parseIfNames(line[loc[0]:])
		suffix, err := buildSuffix(names)
		if err != nil {
			return nil, err
		}

		prefix := strings.TrimSpace(line[:loc[0]])
		if prefix == "" {
			// Standalone annotation: emit the :if $N marker so dynCompile's
			// "skipNext" logic handles the immediately following line.
			newLines = append(newLines, strings.TrimSpace(suffix))
			// If the next line opens a paren block, also activate block
			// propagation so every interior line gets the same condition.
			// blockDepth starts at 0; the block propagation code will add the
			// net paren depth of the next line on its first iteration.
			if i+1 < len(lines) {
				nextLine := lines[i+1]
				nextDepth := strings.Count(nextLine, "(") - strings.Count(nextLine, ")")
				if nextDepth > 0 {
					blockSuffix = suffix
					blockDepth = 0
				}
			}
		} else {
			// Inline annotation
			content := strings.TrimRight(line[:loc[0]], " \t")
			newLines = append(newLines, content+suffix)
			// If the content opens a paren block, propagate to subsequent lines.
			depth := strings.Count(content, "(") - strings.Count(content, ")")
			if depth > 0 {
				blockSuffix = suffix
				blockDepth = depth
			}
		}
	}

	annotatedSQL := strings.Join(newLines, "\n")

	// Build ConditionalParamNumbers list
	var condNums []int
	for num := range conditionalParamNums {
		condNums = append(condNums, int(num))
	}
	sort.Ints(condNums)

	// Build OrderedArgNames: all SQL params in $N order (position 0..N-1),
	// then flag params in appearance order.
	type sqlParam struct {
		name   string
		number int32
	}
	var sqlParamsSorted []sqlParam
	for _, p := range params {
		if p.Column.Name != "" {
			sqlParamsSorted = append(sqlParamsSorted, sqlParam{name: p.Column.Name, number: p.Number})
		}
	}
	sort.Slice(sqlParamsSorted, func(i, j int) bool {
		return sqlParamsSorted[i].number < sqlParamsSorted[j].number
	})

	orderedArgNames := make([]string, len(params)+len(flagParams))
	for i, sp := range sqlParamsSorted {
		orderedArgNames[i] = sp.name
	}
	for i, fp := range flagParams {
		orderedArgNames[len(params)+i] = fp.Name
	}

	return &DynFilterInfo{
		AnnotatedSQL:            annotatedSQL,
		ConditionalParamNumbers: condNums,
		FlagParams:              flagParams,
		OrderedArgNames:         orderedArgNames,
	}, nil
}

func numberSqlcSlices(sql string, paramByName map[string]int32) string {
	return sqlcSliceRe.ReplaceAllStringFunc(sql, func(marker string) string {
		name := sqlcSliceRe.FindStringSubmatch(marker)[1]
		n, ok := paramByName[name]
		if !ok {
			// Unknown slice name: leave the marker untouched rather than
			// emitting an unresolvable ?0 placeholder.
			return marker
		}
		return fmt.Sprintf("/*SLICE:%s*/?%d", name, n)
	})
}

// structName converts snake_case to CamelCase (reuses the same logic as StructName but simplified).
func structName(name string) string {
	parts := strings.Split(name, "_")
	var out string
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out += strings.ToUpper(p[:1]) + p[1:]
	}
	return out
}
