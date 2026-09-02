package ir

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ZONO33LHD/archy-go/internal/diag"
)

// minimalDoc は検証を通る最小の architecture IR。
const minimalDoc = `{
  "schema_version": 1,
  "diagram_type": "architecture",
  "meta": { "title": "t" },
  "components": [
    { "id": "a", "type": "backend", "label": "A", "pos": [0, 0], "size": [100, 50] }
  ]
}`

func hasCode(ds diag.List, code string) bool {
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestParseMinimal(t *testing.T) {
	doc, ds := Parse("architecture", []byte(minimalDoc))
	if doc == nil {
		t.Fatalf("Parse 失敗: %v", ds)
	}
	if doc.Meta.Title != "t" || len(doc.Components) != 1 {
		t.Errorf("デコード結果が想定と異なります: %+v", doc)
	}
}

// TestParseRejections は §13 リソース枯渇・検疫系の回帰テスト。
func TestParseRejections(t *testing.T) {
	deep := strings.Repeat("[", 10000) + strings.Repeat("]", 10000)
	huge := make([]byte, MaxInputBytes+1)
	for i := range huge {
		huge[i] = ' '
	}

	var manyComponents strings.Builder
	manyComponents.WriteString(`{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[`)
	for i := 0; i < 10000; i++ {
		if i > 0 {
			manyComponents.WriteString(",")
		}
		fmt.Fprintf(&manyComponents, `{"id":"n%d","type":"backend","label":"x","pos":[0,0],"size":[10,10]}`, i)
	}
	manyComponents.WriteString(`]}`)

	longLabel := strings.Replace(minimalDoc, `"label": "A"`, `"label": "`+strings.Repeat("x", 1000000)+`"`, 1)

	tests := []struct {
		name     string
		input    []byte
		wantCode string
	}{
		{"深さ10000のネスト", []byte(deep), "input/too-deep"},
		{"サイズ超過", huge, "input/too-large"},
		{"components 10000件", []byte(manyComponents.String()), "schema/items"},
		{"pos が 1e308", []byte(strings.Replace(minimalDoc, `"pos": [0, 0]`, `"pos": [1e308, 1e308]`, 1)), "schema/range"},
		{"NaN リテラル", []byte(strings.Replace(minimalDoc, `"pos": [0, 0]`, `"pos": [NaN, 0]`, 1)), "input/syntax"},
		{"長さ100万の label", []byte(longLabel), "schema/length"},
		{"重複キー", []byte(`{"schema_version":1,"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[]}`), "input/duplicate-key"},
		{"不正UTF-8", []byte(`{"schema_version":1,"diagram_type":"architecture","meta":{"title":"` + "\xff" + `"},"components":[]}`), "input/invalid-utf8"},
		{"未知フィールド", []byte(strings.Replace(minimalDoc, `"meta": { "title": "t" }`, `"meta": { "title": "t", "open": true }`, 1)), "schema/unknown-field"},
		{"meta.open 相当のフィールド", []byte(strings.Replace(minimalDoc, `"schema_version": 1,`, `"schema_version": 1, "open": true,`, 1)), "schema/unknown-field"},
		{"図種の不一致", []byte(strings.Replace(minimalDoc, `"architecture"`, `"workflow"`, 1)), "input/type-mismatch"},
		{"配列のルート", []byte(`[]`), "schema/type"},
		{"構文エラー", []byte(`{`), "input/syntax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, ds := Parse("architecture", tt.input)
			if doc != nil {
				t.Fatalf("拒否されるべき入力が受理されました")
			}
			if !hasCode(ds, tt.wantCode) {
				t.Errorf("診断コード %s が見つかりません。実際: %+v", tt.wantCode, codesOf(ds))
			}
		})
	}
}

func codesOf(ds diag.List) []string {
	var cs []string
	for _, d := range ds {
		cs = append(cs, d.Code)
	}
	return cs
}

// TestParseDuplicateKeyIsV2Default は encoding/json/v2 の既定挙動 (重複キー拒否)
// に依存していることをテストで固定する (§13)。
func TestParseDuplicateKeyIsV2Default(t *testing.T) {
	in := []byte(`{"schema_version":1,"diagram_type":"architecture","meta":{"title":"a","title":"b"},"components":[]}`)
	doc, ds := Parse("architecture", in)
	if doc != nil {
		t.Fatal("重複キーが受理されました (v2 の既定が変わった可能性)")
	}
	if !hasCode(ds, "input/duplicate-key") {
		t.Errorf("input/duplicate-key が返りません: %v", codesOf(ds))
	}
}

func TestParsePointerAccuracy(t *testing.T) {
	in := strings.Replace(minimalDoc, `"type": "backend"`, `"type": "mainframe"`, 1)
	_, ds := Parse("architecture", []byte(in))
	found := false
	for _, d := range ds {
		if d.Code == "schema/enum" && d.Pointer == "/components/0/type" {
			found = true
		}
	}
	if !found {
		t.Errorf("JSON Pointer /components/0/type の診断が見つかりません: %+v", ds)
	}
}

func TestReadLimited(t *testing.T) {
	big := bytes.Repeat([]byte("a"), MaxInputBytes+1)
	_, ds := ReadLimited(bytes.NewReader(big))
	if !hasCode(ds, "input/too-large") {
		t.Errorf("サイズ上限が強制されていません: %v", codesOf(ds))
	}
	data, ds := ReadLimited(strings.NewReader("{}"))
	if len(ds) != 0 || string(data) != "{}" {
		t.Errorf("通常読み込みが失敗: %v", ds)
	}
}

// FuzzParseIR: 不変条件 = パニックしない、必ず有限時間で診断か結果を返す (§13)。
func FuzzParseIR(f *testing.F) {
	f.Add([]byte(minimalDoc))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[[[[[[`))
	f.Add([]byte(`{"a":`))
	f.Add([]byte("\xff\xfe"))
	f.Add([]byte(`{"schema_version":1e999}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, ds := Parse("architecture", data)
		if doc == nil && len(ds) == 0 {
			t.Error("失敗時に診断がありません")
		}
	})
}
