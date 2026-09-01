package cli

import (
	"strings"
	"testing"

	"github.com/ZONO33LHD/archy-go/internal/diag"
)

// TestDiagnosticCap はスキーマ違反の巨大入力でも診断件数が上限で頭打ちになることを検証する。
// (レビュー指摘: 4MiB の巨大配列から数百万件の診断で OOM を起こせる)
func TestDiagnosticCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t"},"components":[`)
	for i := range 5000 {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('0') // 各要素は object 期待に対し数値 = 型不正
	}
	sb.WriteString(`]}`)

	res := build("architecture", []byte(sb.String()), "", false)
	if res.OK {
		t.Fatal("不正入力が受理されました")
	}
	if len(res.Diags) > diag.MaxDiagnostics+1 {
		t.Errorf("診断件数が上限を超えています: %d (上限 %d)", len(res.Diags), diag.MaxDiagnostics)
	}
	if !hasDiagCode(res, "diagnostics/truncated") {
		t.Errorf("上限到達時の diagnostics/truncated がありません: %v", diagCodes(res))
	}
}

// TestComposition2DCap は正当な最大件数でも O(n²) 検査の診断が上限で頭打ちになることを検証する。
// (レビュー指摘: 2000 接続の総当たりで約 200 万件の診断)
func TestCompositionQuadraticCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"schema_version":1,"diagram_type":"architecture","meta":{"title":"t","quality_profile":"showcase"},"components":[`)
	// 全ノードを同一座標に重ねる → node-overlap が総当たりで大量発生する。
	n := 300
	for i := range n {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"id":"n`)
		sb.WriteString(itoa(i))
		sb.WriteString(`","type":"backend","label":"x","pos":[0,0],"size":[100,50]}`)
	}
	sb.WriteString(`]}`)

	res := build("architecture", []byte(sb.String()), "showcase", true)
	if res.OK {
		t.Fatal("重なりだらけの図が受理されました")
	}
	if len(res.Diags) > diag.MaxDiagnostics+1 {
		t.Errorf("診断件数が上限を超えています: %d", len(res.Diags))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestMultilineLabelRejected は単一行フィールドの改行が SVG を壊す前に拒否されることを検証する。
// (レビュー指摘: 改行を含むラベルが検証を通り、tspan が無制限に縦へ積まれる)
// JSON 文字列内の \n (エスケープシーケンス) は正当な JSON で、改行文字 1 個を表す。
func TestMultilineLabelRejected(t *testing.T) {
	doc := `{
	  "schema_version": 1, "diagram_type": "architecture",
	  "meta": { "title": "t" },
	  "components": [
	    { "id": "a", "type": "backend", "label": "line1\nline2\nline3", "pos": [0,0], "size": [100,50] }
	  ]
	}`
	res := build("architecture", []byte(doc), "", false)
	if res.OK {
		t.Fatal("改行を含む label が受理されました")
	}
	if !hasDiagCode(res, "sanitize/multiline") {
		t.Errorf("sanitize/multiline が返りません: %v", diagCodes(res))
	}
}

// TestRouteConflictRejected は via と channelX の同時指定が拒否されることを検証する。
func TestRouteConflictRejected(t *testing.T) {
	doc := `{
	  "schema_version": 1, "diagram_type": "architecture",
	  "meta": { "title": "t" },
	  "components": [
	    { "id": "a", "type": "backend", "label": "A", "pos": [0,0], "size": [100,50] },
	    { "id": "b", "type": "database", "label": "B", "pos": [300,0], "size": [100,50] }
	  ],
	  "connections": [
	    { "id": "ab", "from": "a", "to": "b", "via": [[150,25]], "channelX": 150 }
	  ]
	}`
	res := build("architecture", []byte(doc), "", false)
	if res.OK {
		t.Fatal("via と channelX の同時指定が受理されました")
	}
	if !hasDiagCode(res, "ref/route-conflict") {
		t.Errorf("ref/route-conflict が返りません: %v", diagCodes(res))
	}
}
