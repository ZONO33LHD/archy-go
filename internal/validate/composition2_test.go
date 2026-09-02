package validate

import (
	"strings"
	"testing"

	"github.com/ZONO33LHD/archy-go/internal/geometry"
	"github.com/ZONO33LHD/archy-go/internal/ir"
)

// TestTagOverlapDetected はノード外に出るタグピル同士の重なりが検出されることを検証する
// (レビュー指摘: showcase を通過する図で TagRect の重なりを見逃していた)。
func TestTagOverlapDetected(t *testing.T) {
	doc := baseDoc()
	// 近接する 2 ノードに長いタグを付けると、ノード外のタグピルが重なる。
	doc.Components[0].Pos = [2]float64{0, 100}
	doc.Components[0].Tag = "a long protected tag value here"
	doc.Components[1].Pos = [2]float64{130, 100}
	doc.Components[1].Tag = "another long protected tag value"
	ds := runComposition(t, doc, "showcase")
	if !strings.Contains(codes(ds), "composition/node-overlap") {
		t.Errorf("タグ重なりが検出されません: %s", codes(ds))
	}
}

// TestEndpointReentryDetected は via が端点ノードへ逆流する経路が検出されることを検証する
// (レビュー指摘: 端点ノードを一律に交差検査から除外していた)。
func TestEndpointReentryDetected(t *testing.T) {
	doc := baseDoc()
	doc.Components[0].Pos = [2]float64{200, 100}
	doc.Components[0].Size = [2]float64{100, 50}
	doc.Components[1].Pos = [2]float64{200, 300}
	doc.Components[1].Size = [2]float64{100, 50}
	via := [][2]float64{{100, 225}}
	doc.Connections = []ir.Connection{
		{ID: "ab", From: "a", To: "b", FromSide: "right", ToSide: "right", Via: via},
	}
	ds := runComposition(t, doc, "showcase")
	if !strings.Contains(codes(ds), "composition/edge-through-node") {
		t.Errorf("端点再侵入が検出されません: %s", codes(ds))
	}
}

// TestOrthoSegCross は直交セグメント交差判定の単体テスト。
func TestOrthoSegCross(t *testing.T) {
	// 水平 (0,100)-(200,100) と 垂直 (100,0)-(100,200) は (100,100) で交差。
	pt, ok := orthoSegCross(
		geometry.Point{X: 0, Y: 100}, geometry.Point{X: 200, Y: 100},
		geometry.Point{X: 100, Y: 0}, geometry.Point{X: 100, Y: 200})
	if !ok || pt.X != 100 || pt.Y != 100 {
		t.Errorf("交差点 = %v ok=%v, want (100,100) true", pt, ok)
	}
	// T 字接触 (端点で触れる) は交差としない。
	if _, ok := orthoSegCross(
		geometry.Point{X: 0, Y: 100}, geometry.Point{X: 100, Y: 100},
		geometry.Point{X: 100, Y: 0}, geometry.Point{X: 100, Y: 200}); ok {
		t.Error("端点接触が交差と判定されました")
	}
	// 平行 (両方水平) は判定しない。
	if _, ok := orthoSegCross(
		geometry.Point{X: 0, Y: 100}, geometry.Point{X: 200, Y: 100},
		geometry.Point{X: 0, Y: 100}, geometry.Point{X: 200, Y: 100}); ok {
		t.Error("平行セグメントが交差と判定されました")
	}
}

// TestEdgeCrossingDetected は無関係な辺同士の交差が showcase warning になることを検証する。
func TestEdgeCrossingDetected(t *testing.T) {
	doc := baseDoc()
	// 4 ノードを配置し、a→d と c→b が中央で直交交差するよう via を与える。
	doc.Components = []ir.Component{
		{ID: "a", Type: "backend", Label: "A", Pos: [2]float64{0, 0}, Size: [2]float64{80, 40}},
		{ID: "b", Type: "database", Label: "B", Pos: [2]float64{300, 0}, Size: [2]float64{80, 40}},
		{ID: "c", Type: "cloud", Label: "C", Pos: [2]float64{0, 200}, Size: [2]float64{80, 40}},
		{ID: "d", Type: "external", Label: "D", Pos: [2]float64{300, 200}, Size: [2]float64{80, 40}},
	}
	// a(右)→d(左): 中央縦チャネル x=190 を下る。c(右)→b(左): 中央横チャネル y=110 を通る。
	doc.Connections = []ir.Connection{
		{ID: "ad", From: "a", To: "d", FromSide: "right", ToSide: "left", ChannelX: f64(190)},
		{ID: "cb", From: "c", To: "b", FromSide: "right", ToSide: "left", ChannelY: f64(110)},
	}
	ds := runComposition(t, doc, "showcase")
	if !strings.Contains(codes(ds), "composition/edge-crossing") {
		t.Errorf("辺の交差が検出されません: %s", codes(ds))
	}
}

func f64(v float64) *float64 { return &v }
