package validate

import (
	"strings"
	"testing"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/render/architecture"
)

// baseDoc はテスト用のベース IR を組み立てる。
func baseDoc() *ir.Document {
	return &ir.Document{
		SchemaVersion: 1,
		DiagramType:   "architecture",
		Meta:          ir.Meta{Title: "t"},
		Components: []ir.Component{
			{ID: "a", Type: "backend", Label: "A", Pos: [2]float64{0, 0}, Size: [2]float64{120, 60}},
			{ID: "b", Type: "database", Label: "B", Pos: [2]float64{300, 0}, Size: [2]float64{120, 60}},
		},
		Connections: []ir.Connection{
			{ID: "ab", From: "a", To: "b"},
		},
	}
}

func runComposition(t *testing.T, doc *ir.Document, profile string) diag.List {
	t.Helper()
	r := architecture.New()
	layout, lds := r.Layout(doc)
	ds, _ := Composition(layout, lds, profile, false)
	return ds
}

func codes(ds diag.List) string {
	var cs []string
	for _, d := range ds {
		cs = append(cs, d.Code)
	}
	return strings.Join(cs, " ")
}

func TestNodeOverlap(t *testing.T) {
	doc := baseDoc()
	doc.Components[1].Pos = [2]float64{50, 20} // a と重なる
	ds := runComposition(t, doc, "standard")
	if !strings.Contains(codes(ds), "composition/node-overlap") {
		t.Errorf("node-overlap が検出されません: %s", codes(ds))
	}
}

func TestTextOverflow(t *testing.T) {
	doc := baseDoc()
	doc.Components[0].Label = strings.Repeat("非常に長いラベル", 10)
	ds := runComposition(t, doc, "standard")
	if !strings.Contains(codes(ds), "composition/text-overflow") {
		t.Errorf("text-overflow が検出されません: %s", codes(ds))
	}
}

func TestBoundaryContainment(t *testing.T) {
	doc := baseDoc()
	// a のみを wraps するが、b が境界内に食い込む位置に置く。
	doc.Components[1].Pos = [2]float64{100, 0}
	doc.Boundaries = []ir.Boundary{{Kind: "region", Label: "R", Wraps: []string{"a"}}}
	ds := runComposition(t, doc, "standard")
	if !strings.Contains(codes(ds), "composition/boundary-containment") {
		t.Errorf("boundary-containment が検出されません: %s", codes(ds))
	}
}

func TestViewBoxOverflow(t *testing.T) {
	doc := baseDoc()
	doc.Meta.ViewBox = []float64{0, 0, 100, 100} // 明示 viewBox が小さすぎる
	ds := runComposition(t, doc, "standard")
	if !strings.Contains(codes(ds), "composition/viewbox-overflow") {
		t.Errorf("viewbox-overflow が検出されません: %s", codes(ds))
	}
}

func TestLabelCollision(t *testing.T) {
	doc := baseDoc()
	// 2 本の接続のラベルを labelDy で同じ場所に寄せる。
	dy1 := 0.0
	dy2 := -18.0
	doc.Connections = []ir.Connection{
		{ID: "ab", From: "a", To: "b", Label: "same-place-label", LabelDy: &dy1},
		{ID: "ab2", From: "a", To: "b", Label: "same-place-label", LabelDy: &dy2},
	}
	ds := runComposition(t, doc, "showcase")
	if !strings.Contains(codes(ds), "composition/label-collision") {
		t.Errorf("label-collision が検出されません: %s", codes(ds))
	}
}

func TestEdgeThroughNode(t *testing.T) {
	doc := baseDoc()
	// a と b の間に障害物 c を置き、via で強制的に貫通させる。
	doc.Components = append(doc.Components,
		ir.Component{ID: "c", Type: "security", Label: "C", Pos: [2]float64{180, 0}, Size: [2]float64{60, 60}},
	)
	doc.Connections = []ir.Connection{
		{ID: "ab", From: "a", To: "b", FromSide: "right", ToSide: "left",
			Via: [][2]float64{{210, 30}}},
	}
	ds := runComposition(t, doc, "showcase")
	if !strings.Contains(codes(ds), "composition/edge-through-node") {
		t.Errorf("edge-through-node が検出されません: %s", codes(ds))
	}
}

func TestPathQualityWarning(t *testing.T) {
	doc := baseDoc()
	// 5px しか離れていない via で短セグメントを作る。
	doc.Connections = []ir.Connection{
		{ID: "ab", From: "a", To: "b", FromSide: "right", ToSide: "left",
			Via: [][2]float64{{125, 30}, {125, 35}, {200, 35}}},
	}
	ds := runComposition(t, doc, "showcase")
	if !strings.Contains(codes(ds), "composition/segment-too-short") &&
		!strings.Contains(codes(ds), "composition/tight-turn") {
		t.Errorf("経路品質の警告が検出されません: %s", codes(ds))
	}
	// standard では経路品質チェックは実行されない。
	ds = runComposition(t, doc, "standard")
	if strings.Contains(codes(ds), "composition/segment-too-short") {
		t.Errorf("standard で品質チェックが実行されています: %s", codes(ds))
	}
}

// TestShowcaseWarningsFail: showcase では警告 1 つでも不合格 (§5)。
// プロファイル判定自体は cli 層で行うため、ここでは警告が出ることのみ確認する。
func TestReferenceChecks(t *testing.T) {
	doc := baseDoc()
	doc.Connections = append(doc.Connections,
		ir.Connection{ID: "bad", From: "a", To: "ghost"},
		ir.Connection{ID: "self", From: "a", To: "a"},
		ir.Connection{ID: "ab", From: "a", To: "b"}, // 重複 id
	)
	doc.Boundaries = []ir.Boundary{{Kind: "region", Label: "R", Wraps: []string{"ghost2", "a", "a"}}}
	ds := Document(doc)
	for _, want := range []string{"ref/unknown-component", "ref/self-loop", "ref/duplicate-id", "ref/duplicate-wrap"} {
		if !strings.Contains(codes(ds), want) {
			t.Errorf("%s が検出されません: %s", want, codes(ds))
		}
	}
}

func TestMetaOutputConstraint(t *testing.T) {
	doc := baseDoc()
	for _, bad := range []string{"../../../etc/passwd", "/tmp/x.html", "C:foo.html", "evil.html:stream", "CON.html"} {
		doc.Meta.Output = bad
		ds := Document(doc)
		if !strings.Contains(codes(ds), "outpath/meta-output") {
			t.Errorf("meta.output=%q が拒否されません: %s", bad, codes(ds))
		}
	}
	doc.Meta.Output = "ok.html"
	if ds := Document(doc); strings.Contains(codes(ds), "outpath/meta-output") {
		t.Errorf("正当な meta.output が拒否されました: %s", codes(ds))
	}
}
