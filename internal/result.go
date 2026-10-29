package golang

import (
	"bufio"
	"fmt"
	"sort"
	"strings"

	"github.com/sqlc-dev/plugin-sdk-go/metadata"
	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"github.com/sqlc-dev/plugin-sdk-go/sdk"
	"github.com/vtuanjs/sqlc-gen-go/internal/inflection"
	"github.com/vtuanjs/sqlc-gen-go/internal/opts"
)

func buildEnums(req *plugin.GenerateRequest, options *opts.Options) []Enum {
	var enums []Enum
	for _, schema := range req.Catalog.Schemas {
		if schema.Name == "pg_catalog" || schema.Name == "information_schema" {
			continue
		}
		for _, enum := range schema.Enums {
			var enumName string
			if schema.Name == req.Catalog.DefaultSchema {
				enumName = enum.Name
			} else {
				enumName = schema.Name + "_" + enum.Name
			}

			e := Enum{
				Name:      StructName(enumName, options),
				Comment:   enum.Comment,
				NameTags:  map[string]string{},
				ValidTags: map[string]string{},
			}
			if options.EmitJsonTags {
				e.NameTags["json"] = JSONTagName(enumName, options)
				e.ValidTags["json"] = JSONTagName("valid", options)
			}

			seen := make(map[string]struct{}, len(enum.Vals))
			for i, v := range enum.Vals {
				value := EnumReplace(v)
				if _, found := seen[value]; found || value == "" {
					value = fmt.Sprintf("value_%d", i)
				}
				e.Constants = append(e.Constants, Constant{
					Name:  StructName(enumName+"_"+value, options),
					Value: v,
					Type:  e.Name,
				})
				seen[value] = struct{}{}
			}
			enums = append(enums, e)
		}
	}
	if len(enums) > 0 {
		sort.Slice(enums, func(i, j int) bool { return enums[i].Name < enums[j].Name })
	}
	return enums
}

func buildStructs(req *plugin.GenerateRequest, options *opts.Options) []Struct {
	var structs []Struct
	for _, schema := range req.Catalog.Schemas {
		if schema.Name == "pg_catalog" || schema.Name == "information_schema" {
			continue
		}
		for _, table := range schema.Tables {
			var tableName string
			if schema.Name == req.Catalog.DefaultSchema {
				tableName = table.Rel.Name
			} else {
				tableName = schema.Name + "_" + table.Rel.Name
			}
			structName := tableName
			if !options.EmitExactTableNames {
				structName = inflection.Singular(inflection.SingularParams{
					Name:       structName,
					Exclusions: options.InflectionExcludeTableNames,
				})
			}
			s := Struct{
				Table:   &plugin.Identifier{Schema: schema.Name, Name: table.Rel.Name},
				Name:    StructName(structName, options),
				Comment: table.Comment,
			}
			for _, column := range table.Columns {
				tags := map[string]string{}
				if options.EmitDbTags {
					tags["db"] = column.Name
				}
				if options.EmitJsonTags {
					tags["json"] = JSONTagName(column.Name, options)
				}
				addExtraGoStructTags(tags, req, options, column)
				s.Fields = append(s.Fields, Field{
					Name:    StructName(column.Name, options),
					Type:    goType(req, options, column),
					Tags:    tags,
					Comment: column.Comment,
				})
			}
			structs = append(structs, s)
		}
	}
	if len(structs) > 0 {
		sort.Slice(structs, func(i, j int) bool { return structs[i].Name < structs[j].Name })
	}
	return structs
}

type goColumn struct {
	id int
	*plugin.Column
	embed *goEmbed
}

type goEmbed struct {
	modelType string
	modelName string
	fields    []Field
}

// look through all the structs and attempt to find a matching one to embed
// We need the name of the struct and its field names.
func newGoEmbed(embed *plugin.Identifier, structs []Struct, defaultSchema string) *goEmbed {
	if embed == nil {
		return nil
	}

	for _, s := range structs {
		embedSchema := defaultSchema
		if embed.Schema != "" {
			embedSchema = embed.Schema
		}

		// compare the other attributes
		if embed.Catalog != s.Table.Catalog || embed.Name != s.Table.Name || embedSchema != s.Table.Schema {
			continue
		}

		fields := make([]Field, len(s.Fields))
		copy(fields, s.Fields)

		return &goEmbed{
			modelType: s.Name,
			modelName: s.Name,
			fields:    fields,
		}
	}

	return nil
}

func columnName(c *plugin.Column, pos int) string {
	if c.Name != "" {
		return c.Name
	}
	return fmt.Sprintf("column_%d", pos+1)
}

func paramName(p *plugin.Parameter) string {
	if p.Column.Name != "" {
		return argName(p.Column.Name)
	}
	return fmt.Sprintf("dollar_%d", p.Number)
}

func argName(name string) string {
	out := ""
	for i, p := range strings.Split(name, "_") {
		if i == 0 {
			out += strings.ToLower(p)
		} else if p == "id" {
			out += "ID"
		} else {
			out += strings.Title(p)
		}
	}
	return out
}

func buildQueries(req *plugin.GenerateRequest, options *opts.Options, structs []Struct) ([]Query, error) {
	qs := make([]Query, 0, len(req.Queries))
	for _, query := range req.Queries {
		if query.Name == "" {
			continue
		}
		if query.Cmd == "" {
			continue
		}

		var constantName string
		if options.EmitExportedQueries {
			constantName = sdk.Title(query.Name)
		} else {
			constantName = sdk.LowerTitle(query.Name)
		}

		comments := query.Comments
		if options.EmitSqlAsComment {
			if len(comments) == 0 {
				comments = append(comments, query.Name)
			}
			comments = append(comments, " ")
			scanner := bufio.NewScanner(strings.NewReader(query.Text))
			for scanner.Scan() {
				line := scanner.Text()
				comments = append(comments, "  "+line)
			}
			if err := scanner.Err(); err != nil {
				return nil, err
			}
		}

		gq := Query{
			Cmd:          query.Cmd,
			ConstantName: constantName,
			FieldName:    sdk.LowerTitle(query.Name) + "Stmt",
			MethodName:   query.Name,
			SourceName:   query.Filename,
			SQL:          query.Text,
			Comments:     comments,
			Table:        query.InsertIntoTable,
		}
		sqlpkg := parseDriver(options.SqlPackage)

		qpl := int(*options.QueryParameterLimit)

		if len(query.Params) == 1 && qpl != 0 {
			p := query.Params[0]
			gq.Arg = QueryValue{
				Name:      escape(paramName(p)),
				DBName:    p.Column.GetName(),
				Typ:       goType(req, options, p.Column),
				SQLDriver: sqlpkg,
				Column:    p.Column,
			}
		} else if len(query.Params) >= 1 {
			var cols []goColumn
			for _, p := range query.Params {
				cols = append(cols, goColumn{
					id:     int(p.Number),
					Column: p.Column,
				})
			}
			s, err := columnsToStruct(req, options, gq.MethodName+"Params", cols, false)
			if err != nil {
				return nil, err
			}
			gq.Arg = QueryValue{
				Emit:        true,
				Name:        "arg",
				Struct:      s,
				SQLDriver:   sqlpkg,
				EmitPointer: options.EmitParamsStructPointers,
			}

			// if query params is 2, and query params limit is 4 AND this is a copyfrom, we still want to emit the query's model
			// otherwise we end up with a copyfrom using a struct without the struct definition
			if len(query.Params) <= qpl && query.Cmd != ":copyfrom" {
				gq.Arg.Emit = false
			}
		}

		if len(query.Columns) == 1 && query.Columns[0].EmbedTable == nil {
			c := query.Columns[0]
			name := columnName(c, 0)
			name = strings.Replace(name, "$", "_", -1)
			retName := escape(name)
			// For :one queries the scan destination lives in the same scope as
			// the query parameters, so reusing a parameter's name would cause
			// Scan to overwrite the input and leak it back to the caller on
			// sql.ErrNoRows (see sqlc-dev/sqlc#4354). Rename the return
			// variable when it would collide.
			if query.Cmd == metadata.CmdOne {
				argNames := map[string]struct{}{}
				for _, p := range gq.Arg.Pairs() {
					argNames[p.Name] = struct{}{}
				}
				for {
					if _, conflict := argNames[retName]; !conflict {
						break
					}
					retName += "_2"
				}
			}
			gq.Ret = QueryValue{
				Name:      retName,
				DBName:    name,
				Typ:       goType(req, options, c),
				SQLDriver: sqlpkg,
			}
		} else if putOutColumns(query) {
			var gs *Struct
			var emit bool

			for _, s := range structs {
				if len(s.Fields) != len(query.Columns) {
					continue
				}
				same := true
				for i, f := range s.Fields {
					c := query.Columns[i]
					sameName := f.Name == StructName(columnName(c, i), options)
					sameType := f.Type == goType(req, options, c)
					sameTable := sdk.SameTableName(c.Table, s.Table, req.Catalog.DefaultSchema)
					if !sameName || !sameType || !sameTable {
						same = false
					}
				}
				if same {
					gs = &s
					break
				}
			}

			if gs == nil {
				var columns []goColumn
				for i, c := range query.Columns {
					columns = append(columns, goColumn{
						id:     i,
						Column: c,
						embed:  newGoEmbed(c.EmbedTable, structs, req.Catalog.DefaultSchema),
					})
				}
				var err error
				gs, err = columnsToStruct(req, options, gq.MethodName+"Row", columns, true)
				if err != nil {
					return nil, err
				}
				emit = true
			}
			gq.Ret = QueryValue{
				Emit:        emit,
				Name:        "i",
				Struct:      gs,
				SQLDriver:   sqlpkg,
				EmitPointer: options.EmitResultStructPointers,

				DisableSlicePointer: options.DisableResultSlicePointers,
			}
		}

		if options.EmitDynamicFilter {
			if err := applyDynFilter(req, options, &gq, query.Params, sqlpkg); err != nil {
				return nil, err
			}
		}

		qs = append(qs, gq)
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].MethodName < qs[j].MethodName })
	return qs, nil
}

var cmdReturnsData = map[string]struct{}{
	metadata.CmdBatchMany: {},
	metadata.CmdBatchOne:  {},
	metadata.CmdMany:      {},
	metadata.CmdOne:       {},
}

func putOutColumns(query *plugin.Query) bool {
	_, found := cmdReturnsData[query.Cmd]
	return found
}

// It's possible that this method will generate duplicate JSON tag values
//
//	Columns: count, count,   count_2
//	 Fields: Count, Count_2, Count2
//
// JSON tags: count, count_2, count_2
//
// This is unlikely to happen, so don't fix it yet
func columnsToStruct(req *plugin.GenerateRequest, options *opts.Options, name string, columns []goColumn, useID bool) (*Struct, error) {
	gs := Struct{
		Name: name,
	}
	seen := map[string][]int{}
	suffixes := map[int]int{}
	for i, c := range columns {
		colName := columnName(c.Column, i)
		tagName := colName

		// override col/tag with expected model name
		if c.embed != nil {
			colName = c.embed.modelName
			tagName = SetCaseStyle(colName, "snake")
		}

		fieldName := StructName(colName, options)
		baseFieldName := fieldName
		// Track suffixes by the ID of the column, so that columns referring to the same numbered parameter can be
		// reused.
		suffix := 0
		if o, ok := suffixes[c.id]; ok && useID {
			suffix = o
		} else if v := len(seen[fieldName]); v > 0 && !c.IsNamedParam {
			suffix = v + 1
		}
		suffixes[c.id] = suffix
		if suffix > 0 {
			tagName = fmt.Sprintf("%s_%d", tagName, suffix)
			fieldName = fmt.Sprintf("%s_%d", fieldName, suffix)
		}
		tags := map[string]string{}
		if options.EmitDbTags {
			tags["db"] = tagName
		}
		if options.EmitJsonTags {
			tags["json"] = JSONTagName(tagName, options)
		}
		addExtraGoStructTags(tags, req, options, c.Column)
		f := Field{
			Name:   fieldName,
			DBName: colName,
			Tags:   tags,
			Column: c.Column,
		}
		if c.embed == nil {
			f.Type = goType(req, options, c.Column)
		} else {
			f.Type = c.embed.modelType
			f.EmbedFields = c.embed.fields
		}

		gs.Fields = append(gs.Fields, f)
		if _, found := seen[baseFieldName]; !found {
			seen[baseFieldName] = []int{i}
		} else {
			seen[baseFieldName] = append(seen[baseFieldName], i)
		}
	}

	// If a field does not have a known type, but another
	// field with the same name has a known type, assign
	// the known type to the field without a known type
	for i, field := range gs.Fields {
		if len(seen[field.Name]) > 1 && field.Type == "interface{}" {
			for _, j := range seen[field.Name] {
				if i == j {
					continue
				}
				otherField := gs.Fields[j]
				if otherField.Type != field.Type {
					field.Type = otherField.Type
				}
				gs.Fields[i] = field
			}
		}
	}

	err := checkIncompatibleFieldTypes(gs.Fields)
	if err != nil {
		return nil, err
	}

	return &gs, nil
}

// applyDynFilter parses :if annotations in gq.SQL, adjusts the params struct
// (pointer types for conditional SQL params, bool fields for flag-only params),
// and sets gq.HasDynFilter / gq.DynFilterArgs.
func applyDynFilter(req *plugin.GenerateRequest, options *opts.Options, gq *Query, sqlParams []*plugin.Parameter, sqlpkg opts.SQLDriver) error {
	info, err := parseDynFilter(gq.SQL, sqlParams, req.Settings.Engine)
	if err != nil {
		return fmt.Errorf("query %s: %w", gq.MethodName, err)
	}
	if info == nil {
		return nil
	}

	gq.HasDynFilter = true
	gq.SQL = info.AnnotatedSQL

	// Build a set of param numbers that are conditional (need pointer types)
	conditionalNums := make(map[int]bool)
	for _, n := range info.ConditionalParamNumbers {
		conditionalNums[n] = true
	}

	// If arg is a struct, update the field types for conditional params and
	// append bool fields for flag-only params.
	if gq.Arg.Struct != nil {
		// Make conditional SQL param fields into pointer types
		for i, f := range gq.Arg.Struct.Fields {
			if f.Column == nil {
				continue
			}
			// Find the param number for this field
			for _, p := range sqlParams {
				if p.Column.Name == f.DBName && conditionalNums[int(p.Number)] {
					if !strings.HasPrefix(f.Type, "*") && !strings.HasPrefix(f.Type, "[]") {
						gq.Arg.Struct.Fields[i].Type = "*" + f.Type
					}
					break
				}
			}
		}
		// Append bool fields for flag-only params
		for _, fp := range info.FlagParams {
			gq.Arg.Struct.Fields = append(gq.Arg.Struct.Fields, Field{
				Name:   fp.GoName,
				DBName: fp.Name,
				Type:   "bool",
			})
		}
		// Force emission of the struct
		gq.Arg.Emit = true
	} else if !gq.Arg.isEmpty() {
		// Single param (not a struct) that is conditional: make it a pointer.
		// Slices are already nil-able, so skip wrapping for []T types.
		if conditionalNums[1] && !strings.HasPrefix(gq.Arg.Typ, "*") && !strings.HasPrefix(gq.Arg.Typ, "[]") {
			gq.Arg.Typ = "*" + gq.Arg.Typ
		}
		// If there are flag params, we need to upgrade to a struct
		if len(info.FlagParams) > 0 {
			// Rebuild as a struct with the original param + flag params
			fields := []Field{
				{
					Name:   StructName(gq.Arg.Name, options),
					DBName: gq.Arg.DBName,
					Type:   gq.Arg.Typ,
					Column: gq.Arg.Column,
				},
			}
			for _, fp := range info.FlagParams {
				fields = append(fields, Field{
					Name:   fp.GoName,
					DBName: fp.Name,
					Type:   "bool",
				})
			}
			gq.Arg = QueryValue{
				Emit:      true,
				Name:      "arg",
				Struct:    &Struct{Name: gq.MethodName + "Params", Fields: fields},
				SQLDriver: sqlpkg,
			}
		}
	} else if len(info.FlagParams) > 0 {
		// No SQL params, only flag params
		var fields []Field
		for _, fp := range info.FlagParams {
			fields = append(fields, Field{
				Name:   fp.GoName,
				DBName: fp.Name,
				Type:   "bool",
			})
		}
		gq.Arg = QueryValue{
			Emit:      true,
			Name:      "arg",
			Struct:    &Struct{Name: gq.MethodName + "Params", Fields: fields},
			SQLDriver: sqlpkg,
		}
	}

	// Build DynFilterArgs: the ordered list of field expressions for Build.
	// Ordering must match the :dynif N indices produced by ParseDynFilter.
	// ParseDynFilter assigns indices in order of first appearance in the SQL.
	// We need to reconstruct that order here.
	//
	// For SQL params: expression is "arg.FieldName" (or just "paramName" if single)
	// For flag params: expression is "arg.FlagName"
	gq.DynFilterArgs = buildDynFilterArgs(gq, sqlParams, info)
	gq.DynQueryVar = "_" + sdk.LowerTitle(gq.MethodName) + "DynQ"

	return nil
}

// buildDynFilterArgs builds the args expression for the DynamicSQL call.
// The order matches info.OrderedArgNames (first-appearance order of :if annotations).
func buildDynFilterArgs(gq *Query, sqlParams []*plugin.Parameter, info *DynFilterInfo) string {
	if len(info.OrderedArgNames) == 0 {
		return ""
	}

	// Build param-name -> expression mapping
	paramExpr := make(map[string]string)

	if gq.Arg.Struct != nil {
		for _, p := range sqlParams {
			if p.Column.Name == "" {
				continue
			}
			for _, f := range gq.Arg.Struct.Fields {
				if f.DBName == p.Column.Name {
					paramExpr[p.Column.Name] = "arg." + f.Name
					break
				}
			}
		}
		for _, fp := range info.FlagParams {
			paramExpr[fp.Name] = "arg." + fp.GoName
		}
	} else if !gq.Arg.isEmpty() {
		if len(sqlParams) == 1 && sqlParams[0].Column.Name != "" {
			paramExpr[sqlParams[0].Column.Name] = escape(gq.Arg.Name)
		}
		for _, fp := range info.FlagParams {
			paramExpr[fp.Name] = "arg." + fp.GoName
		}
	}

	var parts []string
	for _, name := range info.OrderedArgNames {
		expr, ok := paramExpr[name]
		if !ok {
			expr = "nil"
		}
		parts = append(parts, expr)
	}
	return strings.Join(parts, ", ")
}

func checkIncompatibleFieldTypes(fields []Field) error {
	fieldTypes := map[string]string{}
	for _, field := range fields {
		if fieldType, found := fieldTypes[field.Name]; !found {
			fieldTypes[field.Name] = field.Type
		} else if field.Type != fieldType {
			return fmt.Errorf("named param %s has incompatible types: %s, %s", field.Name, field.Type, fieldType)
		}
	}
	return nil
}
