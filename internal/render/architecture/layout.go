// Package architecture は architecture 図種のレンダラを実装する。
//
// LLM が決めた pos / size をそのまま尊重し、境界矩形・直交経路・ラベル位置・
// 凡例・viewBox をコードで決定論的に確定させる。座標計算はすべて計算時点で
// 丸め (shared.Round2) を適用し、検証と描画が同じ数値を見る。
package architecture

import (
	"fmt"
	"sort"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/geometry"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/render"
	"github.com/ZONO33LHD/archy-go/internal/render/shared"
)

// Renderer は architecture 図種のレンダラ。
type Renderer struct{}

// New はレンダラを生成する。
func New() *Renderer { return &Renderer{} }

// Type は図種名を返す。
func (r *Renderer) Type() string { return "architecture" }

// フォントサイズの既定値と下限。
const (
	nodeLabelFont    = 14.0
	nodeLabelMin     = 9.0
	nodeSubFont      = 11.0
	nodeSubMin       = 8.0
	tagFont          = 10.0
	tagMin           = 7.0
	edgeLabelFont    = 11.0
	boundaryFont     = 11.5
	boundaryMin      = 8.0
	boundaryPad      = 20.0
	boundaryLabelPad = 22.0
	viewBoxPad       = 24.0
	legendGapY       = 36.0
	legendFont       = 11.0
	portSpacing      = 18.0
	tagPillH         = 16.0
)

// typeOrder はノード種別の正準順 (凡例の並びに使う)。
var typeOrder = []string{"frontend", "backend", "database", "cloud", "security", "messagebus", "external"}

// typeLabels は凡例の表示名 (図の本文であり翻訳しない)。
var typeLabels = map[string]string{
	"frontend": "Frontend", "backend": "Backend", "database": "Database",
	"cloud": "Cloud", "security": "Security", "messagebus": "Message Bus",
	"external": "External",
}

var variantOrder = []string{"default", "emphasis", "security", "dashed"}

var variantLabels = map[string]string{
	"default": "Default link", "emphasis": "Emphasis link",
	"security": "Security link", "dashed": "Dashed link",
}

// Layout は IR から確定ジオメトリを構築する。
func (r *Renderer) Layout(doc *ir.Document) (*render.Layout, diag.List) {
	var ds diag.List
	l := &render.Layout{Title: doc.Meta.Title}

	nodeIdx := make(map[string]int, len(doc.Components))
	l.Nodes = make([]render.Node, len(doc.Components))
	for i, c := range doc.Components {
		rect := geometry.Rect{
			X: shared.Round2(c.Pos[0]), Y: shared.Round2(c.Pos[1]),
			W: shared.Round2(c.Size[0]), H: shared.Round2(c.Size[1]),
		}
		n := render.Node{
			ID: c.ID, Type: c.Type, Label: c.Label,
			Sublabel: c.Sublabel, Tag: c.Tag, Rect: rect,
		}
		layoutNodeText(&n, i, &ds)
		l.Nodes[i] = n
		nodeIdx[c.ID] = i
	}

	layoutBoundaries(doc, l, nodeIdx, &ds)
	layoutEdges(doc, l, nodeIdx, &ds)
	buildLegend(l)
	resolveViewBox(doc, l, &ds)
	return l, ds
}

// layoutNodeText はノード内テキストのフォントサイズとタグピルを確定する。
func layoutNodeText(n *render.Node, idx int, ds *diag.List) {
	ptr := fmt.Sprintf("/components/%d", idx)
	font, ok := shared.FitFont(n.Label, n.Rect.W, nodeLabelFont, nodeLabelMin)
	n.LabelFont = font
	if !ok {
		diag.Append(ds, overflowDiag(ptr+"/label", "component", n.ID, "label", n.Label, n.Rect.W))
	}
	if n.Sublabel != "" {
		font, ok := shared.FitFont(n.Sublabel, n.Rect.W, nodeSubFont, nodeSubMin)
		n.SublabelFont = font
		if !ok {
			diag.Append(ds, overflowDiag(ptr+"/sublabel", "component", n.ID, "sublabel", n.Sublabel, n.Rect.W))
		}
	}
	if n.Tag != "" {
		// タグピルはテキスト幅に合わせるが、ノード幅 + 40px を上限とする。
		w := shared.Round2(shared.TextWidth(n.Tag, tagFont) + 12)
		maxW := n.Rect.W + 40
		if w > maxW {
			w = maxW
		}
		font, ok := shared.FitFont(n.Tag, w, tagFont, tagMin)
		n.TagFont = font
		if !ok {
			diag.Append(ds, overflowDiag(ptr+"/tag", "component", n.ID, "tag", n.Tag, w))
		}
		c := n.Rect.Center()
		n.HasTag = true
		n.TagRect = geometry.Rect{
			X: shared.Round2(c.X - w/2), Y: shared.Round2(n.Rect.Y - tagPillH - 4),
			W: w, H: tagPillH,
		}
	}
}

func overflowDiag(ptr, surface, id, field, text string, width float64) diag.Diagnostic {
	d := diag.Error("composition/text-overflow", ptr,
		fmt.Sprintf("%s が最小フォントサイズまで縮小してもボックス (幅 %spx) に収まりません", field, shared.Coord(width)),
		"ボックスの幅 (size) を広げる",
		"意味を保ったまま文言を短くする")
	d.Subject = &diag.Subject{Surface: surface, ID: id}
	d.Evidence = map[string]any{"boxWidth": width, "textUnits": shared.TextUnits(text)}
	return d
}

// layoutBoundaries は wraps 対象の bbox から境界矩形を確定する。
func layoutBoundaries(doc *ir.Document, l *render.Layout, nodeIdx map[string]int, ds *diag.List) {
	l.Boundaries = make([]render.BoundaryBox, 0, len(doc.Boundaries))
	for i, b := range doc.Boundaries {
		var rects []geometry.Rect
		for _, id := range b.Wraps {
			if idx, ok := nodeIdx[id]; ok {
				rects = append(rects, l.Nodes[idx].Rect)
				if l.Nodes[idx].HasTag {
					rects = append(rects, l.Nodes[idx].TagRect)
				}
			}
		}
		if len(rects) == 0 {
			continue // 参照エラーは検証層が報告済み
		}
		bb := geometry.BoundingBox(rects).Inflate(boundaryPad)
		// ラベル分の余白を上辺に足す。
		bb.Y = shared.Round2(bb.Y - boundaryLabelPad)
		bb.H = shared.Round2(bb.H + boundaryLabelPad)
		bb.X = shared.Round2(bb.X)
		bb.W = shared.Round2(bb.W)
		font, ok := shared.FitFont(b.Label, bb.W-8, boundaryFont, boundaryMin)
		if !ok {
			diag.Append(ds, overflowDiag(fmt.Sprintf("/boundaries/%d/label", i), "boundary", "", "label", b.Label, bb.W-8))
		}
		l.Boundaries = append(l.Boundaries, render.BoundaryBox{
			Kind: b.Kind, Label: b.Label, Wraps: b.Wraps, Rect: bb, LabelFont: font,
		})
	}
}

// portKey は (ノード, 辺) ごとのポートグループのキー。
type portKey struct {
	node string
	side geometry.Side
}

// layoutEdges は辺のサイド解決 → ポートスプレッド → ルーティング → ラベル配置を行う。
func layoutEdges(doc *ir.Document, l *render.Layout, nodeIdx map[string]int, ds *diag.List) {
	type endpoint struct {
		connIdx int
		isFrom  bool
		peer    geometry.Rect
	}
	sides := make([][2]geometry.Side, len(doc.Connections))
	groups := make(map[portKey][]endpoint)
	var groupOrder []portKey // マップの反復順に依存させない

	for i, c := range doc.Connections {
		fi, fok := nodeIdx[c.From]
		ti, tok := nodeIdx[c.To]
		if !fok || !tok {
			continue // 参照エラーは検証層が報告済み
		}
		from, to := l.Nodes[fi].Rect, l.Nodes[ti].Rect
		fs, ts := geometry.AutoSides(from, to)
		if c.FromSide != "" {
			fs = geometry.Side(c.FromSide)
		}
		if c.ToSide != "" {
			ts = geometry.Side(c.ToSide)
		}
		sides[i] = [2]geometry.Side{fs, ts}

		// 経路を明示指定 (via / channelX / channelY) している辺はスプレッド対象外。
		// labelAt はラベル位置だけを動かす指定でポート配置とは無関係なので除外しない。
		if len(c.Via) > 0 || c.ChannelX != nil || c.ChannelY != nil {
			continue
		}
		for _, ep := range []endpoint{
			{connIdx: i, isFrom: true, peer: to},
			{connIdx: i, isFrom: false, peer: from},
		} {
			var key portKey
			if ep.isFrom {
				key = portKey{doc.Connections[i].From, fs}
			} else {
				key = portKey{doc.Connections[i].To, ts}
			}
			if _, seen := groups[key]; !seen {
				groupOrder = append(groupOrder, key)
			}
			groups[key] = append(groups[key], ep)
		}
	}

	// ポートスプレッド: 同一 (ノード, 辺) の複数端点にオフセットを与える。
	offsets := make(map[[2]int]float64) // (connIdx, 0=from/1=to) → offset
	for _, key := range groupOrder {
		eps := groups[key]
		if len(eps) < 2 {
			continue // 辺が 1 本だけなら分散しない
		}
		rect := l.Nodes[nodeIdx[key.node]].Rect
		sideLen := geometry.SideLength(rect, key.side)
		spacing := portSpacing
		if s := sideLen / float64(len(eps)+1); s < spacing {
			spacing = shared.Round2(s)
		}
		// 交差を減らすため相手ノードの位置 (辺に沿った軸) で並べ、同値は接続順。
		sorted := append([]endpoint{}, eps...)
		sort.SliceStable(sorted, func(a, b int) bool {
			var ka, kb float64
			if key.side.Horizontal() {
				ka, kb = sorted[a].peer.Center().Y, sorted[b].peer.Center().Y
			} else {
				ka, kb = sorted[a].peer.Center().X, sorted[b].peer.Center().X
			}
			if ka != kb {
				return ka < kb
			}
			return sorted[a].connIdx < sorted[b].connIdx
		})
		for i, ep := range sorted {
			off := shared.Round2((float64(i) - float64(len(sorted)-1)/2) * spacing)
			which := 1
			if ep.isFrom {
				which = 0
			}
			offsets[[2]int{ep.connIdx, which}] = off
		}
	}

	// 対向ポートの軸揃え: オフセット差が 16px 未満なら同一軸に寄せる
	// (両端が角のクリアランス 8px を保てる場合のみ)。
	alignOpposite(doc, l, nodeIdx, sides, offsets)

	// ルーティング本体。処理順は connections の配列順に固定する。
	l.Edges = make([]render.Edge, 0, len(doc.Connections))
	for i, c := range doc.Connections {
		fi, fok := nodeIdx[c.From]
		ti, tok := nodeIdx[c.To]
		if !fok || !tok {
			continue
		}
		fs, ts := sides[i][0], sides[i][1]
		var obstacles []geometry.Rect
		for j, n := range l.Nodes {
			if j == fi || j == ti {
				continue
			}
			obstacles = append(obstacles, n.Rect)
		}
		var via []geometry.Point
		for _, v := range c.Via {
			via = append(via, geometry.Point{X: shared.Round2(v[0]), Y: shared.Round2(v[1])})
		}
		res := geometry.Route(geometry.RouteRequest{
			From: l.Nodes[fi].Rect, To: l.Nodes[ti].Rect,
			FromSide: fs, ToSide: ts,
			FromOffset: offsets[[2]int{i, 0}], ToOffset: offsets[[2]int{i, 1}],
			Via: via, ChannelX: c.ChannelX, ChannelY: c.ChannelY,
			Obstacles: obstacles,
		})
		variant := c.Variant
		if variant == "" {
			variant = "default"
		}
		e := render.Edge{
			ID: c.ID, From: c.From, To: c.To, Label: c.Label,
			Variant: variant, Points: res.Points, Truncated: res.Truncated,
		}
		if res.Truncated {
			d := diag.Warning("composition/route-truncated", fmt.Sprintf("/connections/%d", i),
				"経路探索が反復上限に達したため最良候補で打ち切りました",
				"via で経路を明示する", "ノード配置を見直して障害物を減らす")
			d.Subject = &diag.Subject{Surface: "connection", ID: c.ID}
			diag.Append(ds, d)
		}
		layoutEdgeLabel(&e, c)
		l.Edges = append(l.Edges, e)
		if len(res.Points) >= 2 {
			l.Ports = append(l.Ports,
				render.Port{NodeID: c.From, Side: fs, Point: res.Points[0], EdgeID: c.ID},
				render.Port{NodeID: c.To, Side: ts, Point: res.Points[len(res.Points)-1], EdgeID: c.ID},
			)
		}
	}
}

// alignOpposite は対向辺 (left/right または top/bottom) で近接する両端ポートを同一軸に揃える。
func alignOpposite(doc *ir.Document, l *render.Layout, nodeIdx map[string]int,
	sides [][2]geometry.Side, offsets map[[2]int]float64) {
	const cornerClearance = 8.0
	for i, c := range doc.Connections {
		fi, fok := nodeIdx[c.From]
		ti, tok := nodeIdx[c.To]
		if !fok || !tok {
			continue
		}
		fs, ts := sides[i][0], sides[i][1]
		opposite := (fs == geometry.SideLeft && ts == geometry.SideRight) ||
			(fs == geometry.SideRight && ts == geometry.SideLeft) ||
			(fs == geometry.SideTop && ts == geometry.SideBottom) ||
			(fs == geometry.SideBottom && ts == geometry.SideTop)
		if !opposite {
			continue
		}
		from, to := l.Nodes[fi].Rect, l.Nodes[ti].Rect
		pA := geometry.PortPoint(from, fs, offsets[[2]int{i, 0}])
		pB := geometry.PortPoint(to, ts, offsets[[2]int{i, 1}])
		var gap float64
		if fs.Horizontal() {
			gap = pB.Y - pA.Y
		} else {
			gap = pB.X - pA.X
		}
		if gap == 0 || gap >= 16 || gap <= -16 {
			continue
		}
		// 中間値に揃える。両端が辺の角クリアランスを保てる場合のみ適用する。
		var axisA, axisB, target float64
		if fs.Horizontal() {
			axisA, axisB = pA.Y, pB.Y
		} else {
			axisA, axisB = pA.X, pB.X
		}
		target = shared.Round2((axisA + axisB) / 2)
		offA := shared.Round2(offsets[[2]int{i, 0}] + (target - axisA))
		offB := shared.Round2(offsets[[2]int{i, 1}] + (target - axisB))
		if withinSide(from, fs, offA, cornerClearance) && withinSide(to, ts, offB, cornerClearance) {
			offsets[[2]int{i, 0}] = offA
			offsets[[2]int{i, 1}] = offB
		}
	}
}

// withinSide はオフセットが辺の範囲 (角クリアランス込み) に収まるか。
func withinSide(r geometry.Rect, s geometry.Side, offset, clearance float64) bool {
	half := geometry.SideLength(r, s)/2 - clearance
	return offset >= -half && offset <= half
}

// layoutEdgeLabel は辺ラベルの位置とマスク矩形を確定する。
func layoutEdgeLabel(e *render.Edge, c ir.Connection) {
	if e.Label == "" || len(e.Points) < 2 {
		return
	}
	t := 0.5
	if c.LabelAt != nil {
		t = *c.LabelAt
	}
	anchor := geometry.PointAt(e.Points, t)
	dy := -12.0 // 既定は経路の上に置く
	if c.LabelDy != nil {
		dy = *c.LabelDy
	}
	w := shared.Round2(shared.TextWidth(e.Label, edgeLabelFont) + 10)
	h := shared.Round2(edgeLabelFont + 7)
	e.HasLabel = true
	e.LabelFont = edgeLabelFont
	e.LabelRect = geometry.Rect{
		X: shared.Round2(anchor.X - w/2),
		Y: shared.Round2(anchor.Y + dy - h/2),
		W: w, H: h,
	}
}

// buildLegend は使用されている種別・variant から凡例を構築する。
func buildLegend(l *render.Layout) {
	usedTypes := map[string]bool{}
	for _, n := range l.Nodes {
		usedTypes[n.Type] = true
	}
	usedVariants := map[string]bool{}
	for _, e := range l.Edges {
		usedVariants[e.Variant] = true
	}
	for _, t := range typeOrder {
		if usedTypes[t] {
			l.Legend = append(l.Legend, render.LegendEntry{Kind: t, Label: typeLabels[t]})
		}
	}
	for _, v := range variantOrder {
		if usedVariants[v] {
			l.Legend = append(l.Legend, render.LegendEntry{Kind: v, Label: variantLabels[v], IsEdge: true})
		}
	}

	// 凡例をコンテンツ bbox の下に置き、本体幅を上限に折り返してエントリ位置を確定する。
	// 折り返すことで、多数の種別・variant を使っても凡例が本体より横に広がって
	// viewBox を引き伸ばす (本体が縮む) のを防ぐ。
	content := contentBBox(l)
	const rowHeight = 22.0
	maxWidth := content.W
	if maxWidth < 200 {
		maxWidth = 200 // 本体が極端に狭い場合の下限
	}
	startX := shared.Round2(content.X)
	startY := shared.Round2(content.MaxY() + legendGapY)

	x, y := startX, startY
	maxRowRight := startX
	for i := range l.Legend {
		entryW := shared.Round2(18 + shared.TextWidth(l.Legend[i].Label, legendFont) + 20)
		if x > startX && x+entryW-startX > maxWidth {
			x = startX // 行を折り返す
			y = shared.Round2(y + rowHeight)
		}
		l.Legend[i].X = x
		l.Legend[i].Y = y
		if x+entryW > maxRowRight {
			maxRowRight = x + entryW
		}
		x = shared.Round2(x + entryW)
	}
	l.LegendRect = geometry.Rect{
		X: startX, Y: startY,
		W: shared.Round2(maxRowRight - startX),
		H: shared.Round2(y + rowHeight - startY),
	}
}

// contentBBox は凡例を除く全要素の bbox。
func contentBBox(l *render.Layout) geometry.Rect {
	var rects []geometry.Rect
	for _, n := range l.Nodes {
		rects = append(rects, n.Rect)
		if n.HasTag {
			rects = append(rects, n.TagRect)
		}
	}
	for _, b := range l.Boundaries {
		rects = append(rects, b.Rect)
	}
	for _, e := range l.Edges {
		for _, p := range e.Points {
			rects = append(rects, geometry.Rect{X: p.X, Y: p.Y})
		}
		if e.HasLabel {
			rects = append(rects, e.LabelRect)
		}
	}
	return geometry.BoundingBox(rects)
}

// resolveViewBox は meta.viewBox の明示値、または全要素からの自動算出で viewBox を確定する。
func resolveViewBox(doc *ir.Document, l *render.Layout, ds *diag.List) {
	if len(doc.Meta.ViewBox) == 4 {
		l.ViewBoxExplicit = true
		for i, v := range doc.Meta.ViewBox {
			l.ViewBox[i] = shared.Round2(v)
		}
		if l.ViewBox[2] <= 0 || l.ViewBox[3] <= 0 {
			diag.Append(ds, diag.Error("composition/viewbox-invalid", "/meta/viewBox",
				"viewBox の幅・高さは正の値でなければなりません",
				"viewBox の第 3・第 4 要素を正の値にする"))
		}
		return
	}
	bb := contentBBox(l).Union(l.LegendRect).Inflate(viewBoxPad)
	l.ViewBox = [4]float64{
		shared.Round2(bb.X), shared.Round2(bb.Y),
		shared.Round2(bb.W), shared.Round2(bb.H),
	}
}
