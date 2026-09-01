// Package render はレンダラ共通の Layout モデルと HTML 組み立てを提供する。
//
// Layout は「検証と描画が同じ数値を見る」ための確定済みジオメトリである。
// 図種ごとのレンダラ (render/architecture など) が IR から Layout を構築し、
// internal/validate が Layout を検査し、同じ Layout から SVG を生成する。
package render

import (
	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/geometry"
	"github.com/ZONO33LHD/archy-go/internal/ir"
)

// Renderer は図種ごとのレンダラが実装するインターフェース。
// 第 2 段階の図種 (workflow など) も同じインターフェースに乗せる。
type Renderer interface {
	// Type は担当する図種名。
	Type() string
	// Layout は IR から確定ジオメトリを構築する。テキスト溢れ等の
	// レイアウト時に確定する診断も返す。
	Layout(doc *ir.Document) (*Layout, diag.List)
	// SVG は Layout から SVG 文字列を生成する。ここに非決定性はない。
	SVG(l *Layout) string
}

// Layout は確定済みジオメトリの全体。
type Layout struct {
	Title   string
	ViewBox [4]float64
	// ViewBoxExplicit は meta.viewBox の明示指定か (収まり検査のエラー文言に使う)。
	ViewBoxExplicit bool
	Nodes           []Node
	Edges           []Edge
	Boundaries      []BoundaryBox
	Legend          []LegendEntry
	LegendRect      geometry.Rect
	// Ports は (ノードID, 辺) ごとの確定ポート位置。ポート間隔検査に使う。
	Ports []Port
}

// Node は配置済みコンポーネント。
type Node struct {
	ID       string
	Type     string
	Label    string
	Sublabel string
	Tag      string
	Rect     geometry.Rect
	// LabelFont / SublabelFont / TagFont は収まるよう縮小済みのフォントサイズ。
	LabelFont    float64
	SublabelFont float64
	TagFont      float64
	// HasTag が真のとき TagRect はノード上部のタグピルの矩形。
	HasTag  bool
	TagRect geometry.Rect
}

// Edge はルーティング済みの辺。
type Edge struct {
	ID      string
	From    string
	To      string
	Label   string
	Variant string
	Points  []geometry.Point
	// HasLabel が真のとき LabelRect / LabelFont が有効。
	HasLabel  bool
	LabelRect geometry.Rect
	LabelFont float64
	// Truncated はルーティング探索が反復上限で打ち切られたか。
	Truncated bool
}

// BoundaryBox は配置済み境界。
type BoundaryBox struct {
	Kind      string
	Label     string
	Wraps     []string
	Rect      geometry.Rect
	LabelFont float64
}

// LegendEntry は凡例の 1 エントリ。
type LegendEntry struct {
	// Kind はノード種別名または辺 variant 名。
	Kind string
	// Label は表示名 (図の本文であり翻訳しない)。
	Label string
	// IsEdge は辺 variant の凡例か。
	IsEdge bool
}

// Port は確定ポート位置。
type Port struct {
	NodeID string
	Side   geometry.Side
	Point  geometry.Point
	EdgeID string
}
