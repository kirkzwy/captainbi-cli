package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
)

type Options struct {
	Format  string
	Machine bool
	JQ      string
}

func Write(w io.Writer, value any, opts Options, columns []string) error {
	var err error
	if opts.JQ != "" {
		value, err = applyJQ(value, opts.JQ)
		if err != nil {
			return err
		}
	}
	format := opts.Format
	if format == "" {
		format = "json"
	}
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	case "ndjson":
		rows := rowsFrom(value)
		enc := json.NewEncoder(w)
		for _, row := range rows {
			if err := enc.Encode(row); err != nil {
				return err
			}
		}
		return nil
	case "csv":
		return writeCSV(w, rowsFrom(value), columns)
	case "table":
		return writeTable(w, rowsFrom(value), columns)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
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

func rowsFrom(value any) []map[string]any {
	if m, ok := value.(map[string]any); ok {
		if data, ok := m["data"]; ok {
			return rowsFrom(data)
		}
		return []map[string]any{m}
	}
	if arr, ok := value.([]any); ok {
		rows := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
		return rows
	}
	return nil
}

func normalizeColumns(rows []map[string]any, columns []string) []string {
	if len(columns) > 0 {
		return columns
	}
	seen := map[string]bool{}
	for _, row := range rows {
		for k := range row {
			seen[k] = true
		}
	}
	for k := range seen {
		columns = append(columns, k)
	}
	sort.Strings(columns)
	return columns
}

func writeCSV(w io.Writer, rows []map[string]any, columns []string) error {
	columns = normalizeColumns(rows, columns)
	cw := csv.NewWriter(w)
	if err := cw.Write(columns); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, len(columns))
		for i, c := range columns {
			record[i] = fmt.Sprint(row[c])
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeTable(w io.Writer, rows []map[string]any, columns []string) error {
	columns = normalizeColumns(rows, columns)
	if len(columns) == 0 {
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	}
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = len(c)
	}
	lines := make([][]string, len(rows))
	for r, row := range rows {
		lines[r] = make([]string, len(columns))
		for i, c := range columns {
			v := fmt.Sprint(row[c])
			lines[r][i] = v
			if len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}
	printRow := func(vals []string) {
		for i, v := range vals {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprintf(w, "%-*s", widths[i], v)
		}
		fmt.Fprintln(w)
	}
	printRow(columns)
	sep := make([]string, len(columns))
	for i := range columns {
		sep[i] = strings.Repeat("-", widths[i])
	}
	printRow(sep)
	for _, line := range lines {
		printRow(line)
	}
	return nil
}
