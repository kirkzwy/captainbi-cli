package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  Format
		ok    bool
	}{
		{"", FormatJSON, true},
		{"JSON", FormatJSON, true},
		{"NdJsOn", FormatNDJSON, true},
		{"TABLE", FormatTable, true},
		{"csv", FormatCSV, true},
		{"pretty", FormatJSON, false},
	} {
		got, ok := ParseFormat(tc.input)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ParseFormat(%q) = %q, %v; want %q, %v", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCSVIncludesAllFieldsWithStableOrdering(t *testing.T) {
	value := map[string]any{"data": []any{
		map[string]any{"z": 2, "id": 1, "nested": map[string]any{"enabled": true}},
		map[string]any{"extra": "x", "id": 2},
	}}
	var out bytes.Buffer
	if err := Write(&out, value, Options{Format: "csv"}, []string{"id", "missing"}); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"id", "extra", "nested", "z"}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header = %#v, want %#v", records[0], wantHeader)
	}
	if records[1][2] != `{"enabled":true}` || records[2][1] != "x" {
		t.Fatalf("nested or sparse values were not encoded predictably: %#v", records)
	}
}

func TestCSVEmptyResultUsesKnownColumns(t *testing.T) {
	var out bytes.Buffer
	if err := Write(&out, map[string]any{"data": []any{}}, Options{Format: "csv"}, []string{"id", "sku"}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "id,sku\n" {
		t.Fatalf("empty CSV = %q", out.String())
	}
}

func TestCSVScalarValuesUseValueColumn(t *testing.T) {
	var out bytes.Buffer
	if err := Write(&out, []string{"one", "two"}, Options{Format: "csv"}, nil); err != nil {
		t.Fatal(err)
	}
	if out.String() != "value\none\ntwo\n" {
		t.Fatalf("scalar CSV = %q", out.String())
	}
}

func TestNDJSONSupportsObjectsAndScalars(t *testing.T) {
	var out bytes.Buffer
	value := map[string]any{"data": []any{map[string]any{"id": 1}, "plain", 3}}
	if err := Write(&out, value, Options{Format: "ndjson"}, nil); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || lines[0] != `{"id":1}` || lines[1] != `"plain"` || lines[2] != "3" {
		t.Fatalf("NDJSON lines = %#v", lines)
	}
}

func TestTableObjectUsesKeyValueAndCompactNestedJSON(t *testing.T) {
	var out bytes.Buffer
	value := map[string]any{"data": map[string]any{"name": "中文", "settings": map[string]any{"a": 1}}}
	if err := Write(&out, value, Options{Format: "table"}, []string{"name", "settings"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "name") || !strings.Contains(out.String(), "中文") || !strings.Contains(out.String(), `{"a":1}`) {
		t.Fatalf("table object = %q", out.String())
	}
}

func TestTableUsesUnicodeWidthAndTruncatesLongCells(t *testing.T) {
	if got := displayWidth("A中文"); got != 5 {
		t.Fatalf("display width = %d, want 5", got)
	}
	truncated := truncateDisplay(strings.Repeat("中", 30), maxTableCellWidth)
	if displayWidth(truncated) > maxTableCellWidth || !strings.HasSuffix(truncated, "…") {
		t.Fatalf("invalid truncation %q width=%d", truncated, displayWidth(truncated))
	}
	var out bytes.Buffer
	value := map[string]any{"data": []any{map[string]any{"id": 1, "description": strings.Repeat("中", 30)}}}
	if err := Write(&out, value, Options{Format: "table"}, []string{"id"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "description") || !strings.Contains(out.String(), "…") {
		t.Fatalf("table should include all fields and truncate long values: %q", out.String())
	}
}

func TestJSONNormalizesTypedValues(t *testing.T) {
	var out bytes.Buffer
	value := struct {
		Count int `json:"count"`
	}{Count: 2}
	if err := Write(&out, value, Options{Format: "json"}, nil); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || decoded["count"] != float64(2) {
		t.Fatalf("typed JSON was not normalized: %#v err=%v", decoded, err)
	}
}

func TestUnsupportedFormatFails(t *testing.T) {
	if err := Write(&bytes.Buffer{}, map[string]any{}, Options{Format: "pretty"}, nil); err == nil {
		t.Fatal("unsupported format should fail")
	}
}
