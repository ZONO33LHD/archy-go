package schema

import (
	json "encoding/json/v2"
	"testing"

	"github.com/ZONO33LHD/archy-go/assets"
	"github.com/ZONO33LHD/archy-go/internal/diag"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func find(ds diag.List, code, pointer string) bool {
	for _, d := range ds {
		if d.Code == code && d.Pointer == pointer {
			return true
		}
	}
	return false
}

// TestGeneratedValidator は生成バリデータの診断コードと JSON Pointer の正確性を検証する。
func TestGeneratedValidator(t *testing.T) {
	tests := []struct {
		name, doc, code, pointer string
	}{
		{"型違い", `{"schema_version":"x","diagram_type":"architecture","meta":{"title":"t"},"components":[]}`,
			"schema/type", "/schema_version"},
		{"必須欠落", `{"schema_version":1,"diagram_type":"architecture","components":[]}`,
			"schema/required", ""},
		{"enum違反", `{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[{"id":"a","type":"quantum","label":"x","pos":[0,0],"size":[10,10]}]}`,
			"schema/enum", "/components/0/type"},
		{"pattern違反", `{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[{"id":"-bad","type":"backend","label":"x","pos":[0,0],"size":[10,10]}]}`,
			"schema/pattern", "/components/0/id"},
		{"range違反", `{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[{"id":"a","type":"backend","label":"x","pos":[0,-2000000],"size":[10,10]}]}`,
			"schema/range", "/components/0/pos/1"},
		{"items違反", `{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[{"id":"a","type":"backend","label":"x","pos":[0],"size":[10,10]}]}`,
			"schema/items", "/components/0/pos"},
		{"未知フィールド", `{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[],"extra":1}`,
			"schema/unknown-field", "/extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := ValidateArchitecture(decode(t, tt.doc))
			if !find(ds, tt.code, tt.pointer) {
				t.Errorf("%s (%s) が見つかりません: %+v", tt.code, tt.pointer, ds)
			}
		})
	}
}

// TestSchemaAssetInSync は埋め込みスキーマが生成バリデータの前提を満たすことの smoke test。
// 生成物とスキーマの完全同期は CI の `go generate` 差分検証が保証する。
func TestSchemaAssetInSync(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(assets.MustRead("schemas/architecture.schema.json"), &s); err != nil {
		t.Fatalf("スキーマがパースできません: %v", err)
	}
	if s["type"] != "object" {
		t.Error("ルートは object であるべきです")
	}
}
