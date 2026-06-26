package golang

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

type cteDefinition struct {
	name    string
	columns []string
}

var (
	returningStarRe = regexp.MustCompile(`(?i)\bRETURNING\s*\*`)
	selectStarRe    = regexp.MustCompile(`(?i)\bSELECT\s+(?:\w+\.)?\*`)
)

func validateCTEReferences(req *plugin.GenerateRequest) error {
	tableColumns := buildTableColumnMap(req)
	for _, query := range req.Queries {
		if query.Name == "" || query.Cmd == "" {
			continue
		}
		normalized := normalizeSQL(query.Text)
		lower := strings.ToLower(normalized)
		if strings.HasPrefix(lower, "with ") {
			if err := validateQueryCTEs(normalized, query.Name, tableColumns); err != nil {
				return err
			}
		} else {
			if err := validateQueryTables(normalized, query.Name, tableColumns); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildTableColumnMap(req *plugin.GenerateRequest) map[string]map[string]struct{} {
	tables := make(map[string]map[string]struct{})
	for _, schema := range req.Catalog.GetSchemas() {
		for _, table := range schema.GetTables() {
			cols := make(map[string]struct{})
			for _, col := range table.GetColumns() {
				cols[strings.ToLower(col.GetName())] = struct{}{}
			}
			tables[strings.ToLower(table.GetRel().GetName())] = cols
		}
	}
	return tables
}

func validateQueryCTEs(sql string, queryName string, tableColumns map[string]map[string]struct{}) error {
	sql = normalizeSQL(sql)
	ctes, cteBodies, mainQuery := parseCTEs(sql, tableColumns)

	cteMap := make(map[string]*cteDefinition)
	for i := range ctes {
		cteMap[ctes[i].name] = &ctes[i]
	}

	// Validate references inside CTE bodies against earlier CTEs + real tables
	earlierCTEs := make(map[string]*cteDefinition)
	for i, cte := range ctes {
		if err := checkTextReferences(cteBodies[i], queryName, earlierCTEs, tableColumns); err != nil {
			return err
		}
		earlierCTEs[cte.name] = &ctes[i]
	}

	return checkTextReferences(mainQuery, queryName, cteMap, tableColumns)
}

func validateQueryTables(sql string, queryName string, tableColumns map[string]map[string]struct{}) error {
	normalized := normalizeSQL(sql)
	return checkTextReferences(normalized, queryName, nil, tableColumns)
}

var (
	lineCommentRe  = regexp.MustCompile(`--[^\n]*`)
	blockCommentRe = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	whitespaceRe   = regexp.MustCompile(`\s+`)
)

func normalizeSQL(sql string) string {
	sql = lineCommentRe.ReplaceAllString(sql, "")
	sql = blockCommentRe.ReplaceAllString(sql, "")
	sql = whitespaceRe.ReplaceAllString(sql, " ")
	return strings.TrimSpace(sql)
}

func parseCTEs(sql string, tableColumns map[string]map[string]struct{}) ([]cteDefinition, []string, string) {
	lower := strings.ToLower(sql)
	if !strings.HasPrefix(lower, "with ") {
		return nil, nil, sql
	}

	withBody, mainQuery := splitWithClause(sql)
	if withBody == "" {
		return nil, nil, sql
	}

	var ctes []cteDefinition
	var bodies []string
	cteMap := make(map[string]*cteDefinition)

	parts := splitCTEDefinitions(withBody)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cte, body := parseSingleCTE(part, tableColumns, cteMap)
		if cte != nil {
			ctes = append(ctes, *cte)
			bodies = append(bodies, body)
			cteMap[cte.name] = cte
		}
	}

	return ctes, bodies, mainQuery
}

func splitWithClause(sql string) (string, string) {
	// Skip the "WITH " prefix (case-insensitive)
	rest := sql[5:]

	// Find the main query by tracking parenthesis depth.
	// The main query starts after the last CTE's closing paren,
	// followed by the next top-level SQL keyword.
	depth := 0
	lastCloseParen := -1
	for i, ch := range rest {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				lastCloseParen = i
			}
		}
	}

	if lastCloseParen == -1 {
		return "", sql
	}

	withBody := rest[:lastCloseParen+1]
	mainQuery := strings.TrimSpace(rest[lastCloseParen+1:])

	return withBody, mainQuery
}

func splitCTEDefinitions(withBody string) []string {
	var parts []string
	depth := 0
	start := 0

	for i, ch := range withBody {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				parts = append(parts, withBody[start:i+1])
				start = i + 1
				// Skip whitespace and comma to next CTE
				for start < len(withBody) && (withBody[start] == ' ' || withBody[start] == '\t' || withBody[start] == '\n' || withBody[start] == '\r' || withBody[start] == ',') {
					start++
				}
			}
		}
	}

	return parts
}

var cteNameRe = regexp.MustCompile(`(?i)^(\w+)\s+AS\s*\(`)

func parseSingleCTE(part string, tableColumns map[string]map[string]struct{}, knownCTEs map[string]*cteDefinition) (*cteDefinition, string) {
	m := cteNameRe.FindStringSubmatchIndex(part)
	if m == nil {
		return nil, ""
	}

	name := strings.ToLower(part[m[2]:m[3]])
	// m[1] is end of full match (position after the opening paren)
	body := part[m[1] : len(part)-1]

	columns := extractCTEColumns(body, tableColumns, knownCTEs)
	return &cteDefinition{name: name, columns: columns}, body
}

func extractCTEColumns(body string, tableColumns map[string]map[string]struct{}, knownCTEs map[string]*cteDefinition) []string {
	bodyLower := strings.ToLower(strings.TrimSpace(body))

	// INSERT/UPDATE/DELETE ... RETURNING *
	if returningStarRe.MatchString(bodyLower) {
		tableName := extractDMLTable(bodyLower)
		if tableName != "" {
			if cols, ok := tableColumns[tableName]; ok {
				var result []string
				for col := range cols {
					result = append(result, col)
				}
				return result
			}
		}
		return nil
	}

	if !strings.HasPrefix(bodyLower, "select") {
		return nil
	}

	// SELECT ... FROM ...
	if selectStarRe.MatchString(bodyLower) {
		return extractSelectStarColumns(bodyLower, tableColumns, knownCTEs)
	}

	return extractSelectListColumns(body)
}

var (
	insertIntoRe = regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(\w+)`)
	updateRe     = regexp.MustCompile(`(?i)\bUPDATE\s+(\w+)`)
	deleteFromRe = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(\w+)`)
	fromSourceRe = regexp.MustCompile(`(?i)\bFROM\s+(\w+)`)
)

func extractDMLTable(bodyLower string) string {
	if m := insertIntoRe.FindStringSubmatch(bodyLower); m != nil {
		return m[1]
	}
	if m := updateRe.FindStringSubmatch(bodyLower); m != nil {
		return m[1]
	}
	if m := deleteFromRe.FindStringSubmatch(bodyLower); m != nil {
		return m[1]
	}
	return ""
}

func extractSelectStarColumns(bodyLower string, tableColumns map[string]map[string]struct{}, knownCTEs map[string]*cteDefinition) []string {
	// SELECT * FROM source or SELECT alias.* FROM source alias
	m := fromSourceRe.FindStringSubmatch(bodyLower)
	if m == nil {
		return nil
	}
	source := strings.ToLower(m[1])

	if cte, ok := knownCTEs[source]; ok {
		cols := make([]string, len(cte.columns))
		copy(cols, cte.columns)
		return cols
	}
	if cols, ok := tableColumns[source]; ok {
		var result []string
		for col := range cols {
			result = append(result, col)
		}
		return result
	}
	return nil
}

func extractSelectListColumns(body string) []string {
	selectBody := extractSelectClause(body)
	if selectBody == "" {
		return nil
	}

	cols := splitSelectColumns(selectBody)
	var result []string
	for _, col := range cols {
		name := resolveColumnName(col)
		if name != "" {
			result = append(result, strings.ToLower(name))
		}
	}
	return result
}

func extractSelectClause(body string) string {
	lower := strings.ToLower(body)

	// Find SELECT keyword (skip leading spaces)
	idx := strings.Index(lower, "select")
	if idx == -1 {
		return ""
	}
	rest := body[idx+6:]
	restLower := strings.ToLower(rest)

	// Skip DISTINCT/ALL
	trimmed := strings.TrimSpace(restLower)
	if strings.HasPrefix(trimmed, "distinct ") {
		offset := strings.Index(restLower, "distinct") + 8
		rest = rest[offset:]
		restLower = restLower[offset:]
	} else if strings.HasPrefix(trimmed, "all ") {
		offset := strings.Index(restLower, "all") + 3
		rest = rest[offset:]
		restLower = restLower[offset:]
	}

	// Find FROM at depth 0
	depth := 0
	for i, ch := range restLower {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && i+5 <= len(restLower) {
				word := restLower[i : i+5]
				if (word == "from " || word == "from\t") && (i == 0 || restLower[i-1] == ' ' || restLower[i-1] == '\t' || restLower[i-1] == ')') {
					return strings.TrimSpace(rest[:i])
				}
			}
		}
	}

	return strings.TrimSpace(rest)
}

func splitSelectColumns(selectBody string) []string {
	var cols []string
	depth := 0
	start := 0

	for i, ch := range selectBody {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				cols = append(cols, strings.TrimSpace(selectBody[start:i]))
				start = i + 1
			}
		}
	}

	last := strings.TrimSpace(selectBody[start:])
	if last != "" {
		cols = append(cols, last)
	}

	return cols
}

var aliasRe = regexp.MustCompile(`(?i)\bAS\s+(\w+)\s*$`)
var qualifiedColRe = regexp.MustCompile(`(?i)^(\w+)\.(\w+)$`)
var simpleColRe = regexp.MustCompile(`(?i)^(\w+)$`)

func resolveColumnName(expr string) string {
	expr = strings.TrimSpace(expr)

	// Check for "expr AS alias"
	if m := aliasRe.FindStringSubmatch(expr); m != nil {
		return m[1]
	}

	// Check for "table.column"
	if m := qualifiedColRe.FindStringSubmatch(expr); m != nil {
		return m[2]
	}

	// Simple column name
	if m := simpleColRe.FindStringSubmatch(expr); m != nil {
		return m[1]
	}

	return ""
}

type columnSource struct {
	name    string
	columns map[string]struct{}
	isCTE   bool
}

var (
	qualifiedRefRe = regexp.MustCompile(`(?i)\b(\w+)\.(\w+)\b`)
	aliasPatternRe = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(\w+)\s+(?:AS\s+)?(\w+)\b`)
)

func buildSourceMap(text string, cteMap map[string]*cteDefinition, tableColumns map[string]map[string]struct{}) map[string]*columnSource {
	sources := make(map[string]*columnSource)

	for name, cols := range tableColumns {
		sources[name] = &columnSource{name: name, columns: cols, isCTE: false}
	}

	for name, cte := range cteMap {
		colSet := make(map[string]struct{}, len(cte.columns))
		for _, c := range cte.columns {
			colSet[c] = struct{}{}
		}
		sources[name] = &columnSource{name: name, columns: colSet, isCTE: true}
	}

	matches := aliasPatternRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		sourceName := strings.ToLower(m[1])
		aliasName := strings.ToLower(m[2])
		switch aliasName {
		case "on", "where", "inner", "left", "right", "outer", "cross",
			"full", "natural", "using", "set", "order", "group", "having",
			"limit", "offset", "union", "intersect", "except", "returning",
			"as", "and", "or", "not", "in", "between", "like", "is", "null",
			"true", "false", "case", "when", "then", "else", "end", "select":
			continue
		}
		if src, ok := sources[sourceName]; ok {
			sources[aliasName] = src
		}
	}
	return sources
}

// checkTextReferences validates column references in text, which the caller must
// already have passed through normalizeSQL.
func checkTextReferences(text string, queryName string, cteMap map[string]*cteDefinition, tableColumns map[string]map[string]struct{}) error {
	normalized := text
	sources := buildSourceMap(normalized, cteMap, tableColumns)

	// Check qualified references (table.column)
	matches := qualifiedRefRe.FindAllStringSubmatch(normalized, -1)
	for _, match := range matches {
		refName := strings.ToLower(match[1])
		colName := strings.ToLower(match[2])

		src, ok := sources[refName]
		if !ok {
			continue
		}

		if len(src.columns) == 0 {
			continue
		}

		if _, found := src.columns[colName]; !found {
			kind := "table"
			if src.isCTE {
				kind = "CTE"
			}
			colList := make([]string, 0, len(src.columns))
			for c := range src.columns {
				colList = append(colList, c)
			}
			return fmt.Errorf(
				"query %q: column %q not found in %s %q (available columns: %s)",
				queryName, match[2], kind, src.name, strings.Join(colList, ", "),
			)
		}
	}

	// Check unqualified references in subqueries with a single FROM table
	if err := checkSubqueryUnqualifiedRefs(normalized, queryName, sources, tableColumns); err != nil {
		return err
	}

	return nil
}

var (
	// Matches FROM table or JOIN table at depth 0 within a clause
	fromTableRe = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(\w+)`)

	// operandRefRe extracts identifiers that appear in comparison operand positions.
	// Each alternative captures into a distinct group; extractOperandRefs collects
	// the non-empty group from each match.
	//
	// Using operand position instead of "all identifiers minus a keyword blocklist"
	// means SQL keywords (ESCAPE, NULLS, FILTER, …) are never extracted in the
	// first place — they don't sit next to comparison operators — so no blocklist
	// is needed and the approach is stable as SQL dialects evolve.
	operandRefRe = regexp.MustCompile(
		`(?i)` +
			// Group 1: identifier on the left of a comparison operator
			`\b([a-z_]\w*)\s*(?:=|!=|<>|<=|>=|<|>)` +
			`|` +
			// Group 2: identifier on the right of a comparison operator
			`(?:=|!=|<>|<=|>=|<|>)\s*([a-z_]\w*)\b` +
			`|` +
			// Group 3: identifier before LIKE / ILIKE / IS / IN / BETWEEN
			`\b([a-z_]\w*)\s+(?:like|ilike|is|in|between)\b` +
			`|` +
			// Group 4: identifier after NOT (boolean column negation: NOT col).
			// NOT LIKE / NOT IN / NOT BETWEEN / NOT ILIKE / NOT EXISTS also match here,
			// but the captured keywords are filtered out via valueKeywords below.
			`\bnot\s+([a-z_]\w*)\b`,
	)
)

// valueKeywords are non-column identifiers that can appear in operand positions
// (adjacent to comparison operators, or captured by the NOT rule). The list is
// kept minimal: only values/functions that truly cannot be column names, plus the
// small set that operators like AT TIME ZONE or COLLATE inject into operand slots.
var valueKeywords = map[string]struct{}{
	// SQL boolean/null literals
	"null": {}, "true": {}, "false": {}, "unknown": {},
	// Logical operators that can land adjacent to a comparison operator after
	// qualified refs are stripped (e.g. "col > X AND col < Y" → "… AND  < Y").
	"and": {}, "or": {},
	// SQL keywords captured by G3/G4 when NOT precedes them:
	// "NOT LIKE", "NOT ILIKE", "NOT IN", "NOT BETWEEN", "NOT EXISTS", "NOT IS"
	// — G3 captures "not" (identifier before like/ilike/is/in/between),
	//   G4 captures the keyword after NOT.
	"not": {}, "like": {}, "ilike": {}, "in": {}, "between": {}, "exists": {}, "is": {},
	// COLLATE appears on the right of = in "col = 'x' COLLATE …"
	"collate": {},
	// AT TIME ZONE pushes "zone" into the left-of-operator position
	"zone": {},
	// Zero-argument datetime functions used as comparison values (no parentheses in SQL)
	"current_date": {}, "current_timestamp": {}, "current_time": {},
	"localtime": {}, "localtimestamp": {},
	"current_user": {}, "session_user": {}, "current_catalog": {}, "current_schema": {},
	// CASE expression keywords: "END = x" puts "end" (and "when" once the qualified
	// ref before it is stripped) into an operand slot.
	"case": {}, "when": {}, "then": {}, "else": {}, "end": {},
	// Type names in typed-literal syntax "TYPE 'string'": after the string literal
	// is stripped, the type name sits next to the comparison operator
	// (e.g. "col = timestamp '…'" → "col =  timestamp").
	"timestamp": {}, "timestamptz": {}, "date": {}, "time": {}, "interval": {},
	"numeric": {}, "decimal": {}, "boolean": {}, "bool": {},
	"int": {}, "integer": {}, "bigint": {}, "smallint": {},
	"real": {}, "double": {}, "char": {}, "varchar": {}, "text": {},
	"bytea": {}, "uuid": {}, "json": {}, "jsonb": {},
}

// listClauseKeywords are the only non-column identifiers that appear in
// ORDER BY / GROUP BY clause bodies.
var listClauseKeywords = map[string]struct{}{
	"asc": {}, "desc": {}, "nulls": {}, "first": {}, "last": {},
	"null": {}, "true": {}, "false": {},
	// COLLATE and USING appear in "ORDER BY col COLLATE …" / "ORDER BY col USING <"
	"collate": {}, "using": {},
	// CASE expression keywords: "ORDER BY CASE WHEN … THEN … ELSE … END"
	"case": {}, "when": {}, "then": {}, "else": {}, "end": {},
}

var listRefRe = regexp.MustCompile(`(?i)\b([a-z_]\w*)\b`)

func extractListRefs(cond string) []string {
	var refs []string
	for _, ref := range listRefRe.FindAllString(cond, -1) {
		if _, skip := listClauseKeywords[strings.ToLower(ref)]; !skip {
			refs = append(refs, ref)
		}
	}
	return refs
}

var (
	// stringLiteralRe matches single-quoted string literals.
	stringLiteralRe = regexp.MustCompile(`'[^']*'`)
	// doubleQuotedRe matches double-quoted identifiers (e.g. COLLATE "en-US").
	doubleQuotedRe = regexp.MustCompile(`"[^"]*"`)
	// castTypeRe matches a "::type" cast, including schema-qualified and array types
	// (e.g. ::int, ::order_status, ::pg_catalog.int4, ::text[]).
	castTypeRe = regexp.MustCompile(`(?i)::\s*"?[\w.]+"?(?:\s*\[\s*\])*`)
	// funcCallRe matches a function-call name (identifier immediately followed by "(").
	funcCallRe = regexp.MustCompile(`(?i)\b[a-z_]\w*\s*\(`)
)

// stripNonColumnTokens removes tokens from a condition clause that contain
// identifiers which are not column references — nested subqueries, string
// literals, "::type" casts, qualified references (already validated separately),
// and function-call names — so the remaining bare identifiers can be checked as
// column names of the current scope.
func stripNonColumnTokens(cond string) string {
	cond = stripSubqueries(cond)
	cond = stringLiteralRe.ReplaceAllString(cond, "")
	cond = doubleQuotedRe.ReplaceAllString(cond, "")
	cond = castTypeRe.ReplaceAllString(cond, "")
	cond = qualifiedRefRe.ReplaceAllString(cond, "")
	cond = funcCallRe.ReplaceAllString(cond, "")
	return cond
}

// stripSubqueries removes balanced parenthesized groups whose contents are a
// SELECT (e.g. EXISTS/IN/scalar subqueries). Their identifiers belong to an
// inner scope and are validated separately, so they must not be checked against
// the current scope's columns. Grouping parens (e.g. "(a OR b)") are preserved.
func stripSubqueries(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '(' {
			if end := findMatchingParen(s, i); end != -1 {
				if strings.HasPrefix(strings.TrimSpace(s[i+1:end]), "select") {
					i = end + 1
					continue
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func checkSubqueryUnqualifiedRefs(text string, queryName string, sources map[string]*columnSource, tableColumns map[string]map[string]struct{}) error {
	lower := strings.ToLower(text)

	// Validate the top-level statement's own WHERE/ON/HAVING conditions. This
	// covers the WHERE clause of a CTE body or main query, which is not wrapped
	// in parentheses and so isn't picked up by the subquery scan below.
	if trimmed := strings.TrimSpace(lower); strings.HasPrefix(trimmed, "select") {
		if err := checkScopeUnqualifiedRefs(trimmed, queryName, sources, tableColumns); err != nil {
			return err
		}
	}

	// Validate each parenthesized subquery in its own scope.
	for i := 0; i < len(lower); i++ {
		if lower[i] != '(' {
			continue
		}

		closeIdx := findMatchingParen(lower, i)
		if closeIdx == -1 {
			break
		}

		innerLower := strings.ToLower(strings.TrimSpace(text[i+1 : closeIdx]))
		if !strings.HasPrefix(innerLower, "select") {
			continue
		}

		if err := checkScopeUnqualifiedRefs(innerLower, queryName, sources, tableColumns); err != nil {
			return err
		}
	}
	return nil
}

// checkScopeUnqualifiedRefs validates the unqualified identifiers in the
// WHERE/ON/HAVING clauses of a single SELECT scope against the columns of the
// tables in that scope's FROM/JOIN list. scopeLower must be lower-cased.
func checkScopeUnqualifiedRefs(scopeLower string, queryName string, sources map[string]*columnSource, tableColumns map[string]map[string]struct{}) error {
	// Find all tables in this scope's FROM/JOIN (depth 0 only)
	scopeTables := extractScopeTables(scopeLower, sources, tableColumns)
	if len(scopeTables) == 0 {
		return nil
	}

	// Build merged column set for all tables in scope
	allCols := make(map[string]struct{})
	var scopeTableNames []string
	for _, src := range scopeTables {
		for col := range src.columns {
			allCols[col] = struct{}{}
		}
		scopeTableNames = append(scopeTableNames, src.name)
	}

	// SELECT-list output names/aliases are valid references in GROUP BY, ORDER BY
	// and (in some dialects) HAVING, so treat them as in-scope columns too.
	for _, out := range extractSelectListColumns(scopeLower) {
		allCols[out] = struct{}{}
	}

	checkRef := func(ref string) error {
		refLower := strings.ToLower(ref)
		if _, isSource := sources[refLower]; isSource {
			return nil
		}
		if _, found := allCols[refLower]; !found {
			return fmt.Errorf(
				"query %q: column %q not found in any table in scope (%s)",
				queryName, ref, strings.Join(scopeTableNames, ", "),
			)
		}
		return nil
	}

	// WHERE/ON/HAVING: only extract identifiers in comparison operand positions.
	// This avoids needing a keyword blocklist — SQL keywords (ESCAPE, NULLS, …)
	// don't appear next to comparison operators.
	condText := extractConditionClauses(scopeLower)
	if condText != "" {
		condClean := stripNonColumnTokens(condText)
		for _, ref := range extractOperandRefs(condClean) {
			if _, isValue := valueKeywords[strings.ToLower(ref)]; isValue {
				continue
			}
			if err := checkRef(ref); err != nil {
				return err
			}
		}
	}

	// ORDER BY / GROUP BY: bare identifiers here are directly column references,
	// so use simple identifier extraction with a tiny exclusion list.
	listText := extractListClauses(scopeLower)
	if listText != "" {
		listClean := stripNonColumnTokens(listText)
		for _, ref := range extractListRefs(listClean) {
			if err := checkRef(ref); err != nil {
				return err
			}
		}
	}

	return nil
}

// extractOperandRefs returns all identifiers captured by operandRefRe from cond.
func extractOperandRefs(cond string) []string {
	matches := operandRefRe.FindAllStringSubmatch(cond, -1)
	var refs []string
	for _, m := range matches {
		for _, g := range m[1:] {
			if g != "" {
				refs = append(refs, g)
			}
		}
	}
	return refs
}

func findMatchingParen(text string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parenDepths returns, for each byte index i in s, the parenthesis nesting depth
// after consuming s[i] (count of '(' minus ')' in s[0:i+1]). Identifiers and
// keywords contain no parens, so depths[i] at such a byte equals the depth of the
// clause it belongs to. Computing this once lets the scanners below run in O(n)
// instead of re-running a regex over the tail of the string at every position.
func parenDepths(s string) []int {
	depths := make([]int, len(s))
	d := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			d++
		case ')':
			d--
		}
		depths[i] = d
	}
	return depths
}

func extractScopeTables(subqueryLower string, sources map[string]*columnSource, tableColumns map[string]map[string]struct{}) []*columnSource {
	// Find FROM/JOIN tables at depth 0 within this subquery.
	var result []*columnSource
	seen := make(map[string]bool)
	depths := parenDepths(subqueryLower)

	for _, m := range fromTableRe.FindAllStringSubmatchIndex(subqueryLower, -1) {
		if depths[m[0]] != 0 {
			continue
		}
		tableName := subqueryLower[m[2]:m[3]]
		if seen[tableName] {
			continue
		}
		seen[tableName] = true
		if src, ok := sources[tableName]; ok {
			result = append(result, src)
		} else if cols, ok := tableColumns[tableName]; ok {
			result = append(result, &columnSource{name: tableName, columns: cols, isCTE: false})
		}
	}

	return result
}

var (
	// comparisonClauseRe matches WHERE/ON/HAVING — clauses where column references
	// appear as operands of comparison operators.
	comparisonClauseRe = regexp.MustCompile(`(?i)\b(?:WHERE|ON|HAVING)\b`)
	// listClauseRe matches ORDER BY / GROUP BY — clauses where bare identifiers
	// are directly column references (not operands of comparisons).
	listClauseRe = regexp.MustCompile(`(?i)\b(?:ORDER\s+BY|GROUP\s+BY)\b`)
)

// extractConditionClauses returns the bodies of WHERE/ON/HAVING clauses joined.
func extractConditionClauses(subqueryLower string) string {
	return extractClauseBodies(subqueryLower, comparisonClauseRe)
}

// extractListClauses returns the bodies of ORDER BY / GROUP BY clauses joined.
func extractListClauses(subqueryLower string) string {
	return extractClauseBodies(subqueryLower, listClauseRe)
}

func extractClauseBodies(subqueryLower string, re *regexp.Regexp) string {
	depths := parenDepths(subqueryLower)
	clauseEnds := clauseEndRe.FindAllStringIndex(subqueryLower, -1)

	var parts []string
	for _, cm := range re.FindAllStringIndex(subqueryLower, -1) {
		if depths[cm[0]] != 0 {
			continue
		}
		clauseStart := cm[1]
		clauseEnd := findClauseEnd(subqueryLower, clauseStart, depths, clauseEnds)
		parts = append(parts, subqueryLower[clauseStart:clauseEnd])
	}

	return strings.Join(parts, " ")
}

var clauseEndRe = regexp.MustCompile(`(?i)\b(?:GROUP|ORDER|LIMIT|OFFSET|UNION|INTERSECT|EXCEPT|RETURNING|HAVING|SELECT|FROM|WHERE)\b`)

// findClauseEnd returns the end index of a clause body that begins at start. The
// clause ends at the first clause-boundary keyword at the same depth, or where a
// ')' closes a paren opened before the clause, whichever comes first. depths and
// clauseEnds are precomputed over the whole string by the caller.
func findClauseEnd(text string, start int, depths []int, clauseEnds [][]int) int {
	base := 0
	if start > 0 {
		base = depths[start-1]
	}

	end := len(text)
	// A ')' that drops below the clause's base depth closes the enclosing scope.
	for i := start; i < len(text); i++ {
		if depths[i] < base {
			end = i
			break
		}
	}
	// The first clause-boundary keyword at the base depth ends the clause earlier.
	for _, m := range clauseEnds {
		if m[0] < start {
			continue
		}
		if m[0] >= end {
			break
		}
		if depths[m[0]] == base {
			return m[0]
		}
	}
	return end
}
