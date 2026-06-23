package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
)

const maxTableCellWidth = 40

type Format string

const (
	FormatJSON   Format = "json"
	FormatNDJSON Format = "ndjson"
	FormatTable  Format = "table"
	FormatCSV    Format = "csv"
)

type Options struct {
	Format  string
	Machine bool
	JQ      string
}

func ParseFormat(value string) (Format, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "json":
		return FormatJSON, true
	case "ndjson":
		return FormatNDJSON, true
	case "table":
		return FormatTable, true
	case "csv":
		return FormatCSV, true
	default:
		return FormatJSON, false
	}
}

func Write(w io.Writer, value any, opts Options, preferredColumns []string) error {
	var err error
	if opts.JQ != "" {
		value, err = applyJQ(value, opts.JQ)
		if err != nil {
			return err
		}
	}
	format, ok := ParseFormat(opts.Format)
	if !ok {
		return fmt.Errorf("unsupported format %q", opts.Format)
	}
	value, err = toGeneric(value)
	if err != nil {
		return err
	}
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	case FormatNDJSON:
		return WriteNDJSON(w, extractData(value))
	case FormatCSV:
		return writeCSV(w, extractData(value), preferredColumns)
	case FormatTable:
		return writeTable(w, extractData(value), preferredColumns)
	default:
		return fmt.Errorf("unsupported format %q", opts.Format)
	}
}

// WriteNDJSON writes each list item as one line and writes non-list values once.
func WriteNDJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	if values, ok := value.([]any); ok {
		for _, item := range values {
			if err := enc.Encode(item); err != nil {
				return err
			}
		}
		return nil
	}
	return enc.Encode(value)
}

func applyJQ(value any, expr string) (any, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &value); err != nil {
		return nil, err
	}
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, err
	}
	iter := query.Run(value)
	var out []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 1 {
		return out[0], nil
	}
	return out, nil
}

func ApplyJQ(value any, expr string) (any, error) {
	return applyJQ(value, expr)
}

func toGeneric(value any) (any, error) {
	switch value.(type) {
	case nil, bool, string, json.Number, float64:
		return value, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	return generic, nil
}

func extractData(value any) any {
	if fields, ok := value.(map[string]any); ok {
		if data, exists := fields["data"]; exists {
			return data
		}
	}
	return value
}

type tabularData struct {
	rows     []map[string]any
	isList   bool
	isObject bool
}

func asTabular(value any) tabularData {
	switch typed := value.(type) {
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if fields, ok := item.(map[string]any); ok {
				rows = append(rows, fields)
			} else {
				rows = append(rows, map[string]any{"value": item})
			}
		}
		return tabularData{rows: rows, isList: true}
	case map[string]any:
		return tabularData{rows: []map[string]any{typed}, isObject: true}
	default:
		return tabularData{rows: []map[string]any{{"value": typed}}, isObject: true}
	}
}

func normalizeColumns(rows []map[string]any, preferred []string) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			seen[key] = true
		}
	}
	columns := make([]string, 0, len(seen)+len(preferred))
	added := map[string]bool{}
	for _, column := range preferred {
		if column == "" || added[column] {
			continue
		}
		if len(seen) == 0 || seen[column] {
			columns = append(columns, column)
			added[column] = true
		}
	}
	remaining := make([]string, 0, len(seen))
	for column := range seen {
		if !added[column] {
			remaining = append(remaining, column)
		}
	}
	sort.Strings(remaining)
	return append(columns, remaining...)
}

func writeCSV(w io.Writer, value any, preferredColumns []string) error {
	data := asTabular(value)
	columns := normalizeColumns(data.rows, preferredColumns)
	if len(columns) == 0 {
		return nil
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(columns); err != nil {
		return err
	}
	for _, row := range data.rows {
		record := make([]string, len(columns))
		for index, column := range columns {
			record[index] = stringify(row[column])
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeTable(w io.Writer, value any, preferredColumns []string) error {
	data := asTabular(value)
	columns := normalizeColumns(data.rows, preferredColumns)
	if len(data.rows) == 0 || len(columns) == 0 {
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	}
	if data.isObject {
		return writeKeyValueTable(w, data.rows[0], columns)
	}
	widths := make([]int, len(columns))
	lines := make([][]string, len(data.rows))
	for index, column := range columns {
		widths[index] = min(displayWidth(column), maxTableCellWidth)
		if widths[index] == 0 {
			widths[index] = 1
		}
	}
	for rowIndex, row := range data.rows {
		lines[rowIndex] = make([]string, len(columns))
		for columnIndex, column := range columns {
			value := cleanCell(stringify(row[column]))
			lines[rowIndex][columnIndex] = truncateDisplay(value, maxTableCellWidth)
			widths[columnIndex] = max(widths[columnIndex], displayWidth(lines[rowIndex][columnIndex]))
		}
	}
	printRow := func(values []string) error {
		for index, value := range values {
			if index > 0 {
				if _, err := fmt.Fprint(w, "  "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(w, padDisplay(truncateDisplay(value, widths[index]), widths[index])); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	if err := printRow(columns); err != nil {
		return err
	}
	separators := make([]string, len(columns))
	for index := range columns {
		separators[index] = strings.Repeat("-", widths[index])
	}
	if err := printRow(separators); err != nil {
		return err
	}
	for _, line := range lines {
		if err := printRow(line); err != nil {
			return err
		}
	}
	return nil
}

func writeKeyValueTable(w io.Writer, row map[string]any, columns []string) error {
	keyWidth := 1
	for _, column := range columns {
		keyWidth = max(keyWidth, min(displayWidth(column), maxTableCellWidth))
	}
	for _, column := range columns {
		key := padDisplay(truncateDisplay(column, keyWidth), keyWidth)
		value := truncateDisplay(cleanCell(stringify(row[column])), maxTableCellWidth)
		if _, err := fmt.Fprintf(w, "%s  %s\n", key, value); err != nil {
			return err
		}
	}
	return nil
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any, []any:
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
	}
	return fmt.Sprint(value)
}

func cleanCell(value string) string {
	value = strings.ReplaceAll(value, "\r", "\\r")
	return strings.ReplaceAll(value, "\n", "\\n")
}

func runeDisplayWidth(r rune) int {
	if r == 0 || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
		(r >= 0xFF01 && r <= 0xFF60) || (r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1FAFF) {
		return 2
	}
	return 1
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		width += runeDisplayWidth(r)
	}
	return width
}

func truncateDisplay(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if displayWidth(value) <= maxWidth {
		return value
	}
	target := maxWidth - 1
	if target <= 0 {
		return "…"
	}
	width := 0
	var builder strings.Builder
	for _, r := range value {
		runeWidth := runeDisplayWidth(r)
		if width+runeWidth > target {
			break
		}
		builder.WriteRune(r)
		width += runeWidth
	}
	return builder.String() + "…"
}

func padDisplay(value string, width int) string {
	padding := width - displayWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}
