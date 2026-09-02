// composition.go は構図検証 (composition checks) を実装する (§5)。
//
// スキーマ検証をパスしても図としては壊れていることがある。
// ここでは確定済み Layout (検証と描画が同じ数値を見る) に対して幾何検査を行う。
package validate

import (
	"fmt"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/geometry"
	"github.com/ZONO33LHD/archy-go/internal/render"
)

// checkSpec は 1 つの構図検査の定義。名前・standard プロファイルで走るか・検査関数を束ねる。
// これが検査一覧と実行順の単一の真実の源であり、名前と実行の二重管理を避ける。
type checkSpec struct {
	name     string
	standard bool // standard プロファイルでも実行するか (false は showcase 専用)
	fn       func(l *render.Layout, layoutDiags diag.List, ds *diag.List)
}

// checkSpecs は全構図検査 (実行順)。
var checkSpecs = []checkSpec{
	{"node-overlap", true, func(l *render.Layout, _ diag.List, ds *diag.List) { checkNodeOverlap(l, ds) }},
	{"text-overflow", true, func(_ *render.Layout, layoutDiags diag.List, ds *diag.List) {
		for _, d := range layoutDiags {
			if d.Code == "composition/text-overflow" {
				diag.Append(ds, d)
			}
		}
	}},
	{"boundary-containment", true, func(l *render.Layout, _ diag.List, ds *diag.List) { checkBoundaryContainment(l, ds) }},
	{"viewbox-fit", true, func(l *render.Layout, _ diag.List, ds *diag.List) { checkViewBoxFit(l, ds) }},
	{"edge-through-node", false, func(l *render.Layout, _ diag.List, ds *diag.List) { checkEdgeThroughNode(l, ds) }},
	{"edge-crossing", false, func(l *render.Layout, _ diag.List, ds *diag.List) { checkEdgeCrossing(l, ds) }},
	{"label-collision", false, func(l *render.Layout, _ diag.List, ds *diag.List) { checkLabelCollision(l, ds) }},
	{"path-quality", false, func(l *render.Layout, _ diag.List, ds *diag.List) { checkPathQuality(l, ds) }},
	{"port-spacing", false, func(l *render.Layout, _ diag.List, ds *diag.List) { checkPortSpacing(l, ds) }},
	{"legend-consistency", false, func(l *render.Layout, _ diag.List, ds *diag.List) { checkLegendConsistency(l, ds) }},
}

// CheckResult は 1 チェックの実行結果。
type CheckResult struct {
	Name string `json:"name"`
	// Ran は実行されたか (standard プロファイルでは一部スキップされる)。
	Ran bool `json:"ran"`
	// Issues は検出された診断数。
	Issues int `json:"issues"`
}

// Composition は Layout に対する構図検証を実行する。
// layoutDiags はレンダラの Layout() が返した診断 (text-overflow 等) で、
// text-overflow チェックの結果集計に取り込む。
// runAll が真なら profile にかかわらず全チェックを実行する (deliver は常に全チェック §12)。
func Composition(l *render.Layout, layoutDiags diag.List, profile string, runAll bool) (diag.List, []CheckResult) {
	showcase := profile == "showcase" || runAll
	var ds diag.List
	results := make([]CheckResult, 0, len(checkSpecs))

	for _, spec := range checkSpecs {
		res := CheckResult{Name: spec.name}
		if showcase || spec.standard {
			res.Ran = true
			before := len(ds)
			spec.fn(l, layoutDiags, &ds)
			res.Issues = len(ds) - before
		}
		results = append(results, res)
	}

	// text-overflow 以外のレイアウト診断 (route-truncated 等) も合流させる。
	for _, d := range layoutDiags {
		if d.Code != "composition/text-overflow" {
			ds = append(ds, d)
		}
	}
	return ds, results
}

// paintedNodeRect は実際に描画されるノード関連の矩形 (本体 + ノード外に出るタグピル)。
type paintedNodeRect struct {
	ownerID string
	kind    string // "box" | "tag"
	rect    geometry.Rect
}

// paintedNodeRects は全ノードの本体矩形とタグピル矩形を列挙する。
// タグはノードの外 (上方) に描かれるため、重なり検査では本体と同格に扱う必要がある。
func paintedNodeRects(l *render.Layout) []paintedNodeRect {
	out := make([]paintedNodeRect, 0, len(l.Nodes))
	for _, n := range l.Nodes {
		out = append(out, paintedNodeRect{ownerID: n.ID, kind: "box", rect: n.Rect})
		if n.HasTag {
			out = append(out, paintedNodeRect{ownerID: n.ID, kind: "tag", rect: n.TagRect})
		}
	}
	return out
}

// checkNodeOverlap はコンポーネントの描画矩形同士の重なりを検査する。
// 本体だけでなくノード外に出るタグピルも対象に含める (別ノードのタグ・本体との重なりを見逃さない)。
func checkNodeOverlap(l *render.Layout, ds *diag.List) {
	painted := paintedNodeRects(l)
	for i := range painted {
		if ds.AtCap() {
			return
		}
		for j := i + 1; j < len(painted); j++ {
			a, b := painted[i], painted[j]
			if a.ownerID == b.ownerID {
				continue // 同一ノードの本体とタグは重なってよい
			}
			if a.rect.Overlaps(b.rect, 0) {
				d := diag.Error("composition/node-overlap", "",
					fmt.Sprintf("component '%s' の%sと '%s' の%sが重なっています",
						a.ownerID, paintKindLabel(a.kind), b.ownerID, paintKindLabel(b.kind)),
					"pos を調整して重なりを解消する",
					"size を縮小して間隔を確保する",
					"タグ (tag) の文言を短くする")
				d.Subject = &diag.Subject{Surface: "component", ID: b.ownerID}
				d.Evidence = map[string]any{
					"a": rectEvidence(a.rect), "b": rectEvidence(b.rect), "conflictsWith": a.ownerID,
				}
				diag.Append(ds, d)
			}
		}
	}
}

func paintKindLabel(kind string) string {
	if kind == "tag" {
		return "タグ"
	}
	return "矩形"
}

func rectEvidence(r geometry.Rect) []float64 {
	return []float64{r.X, r.Y, r.W, r.H}
}

// checkBoundaryContainment は境界の整合性を検査する。
// wraps 対象が境界内に収まっているか (構築上の不変条件) に加えて、
// wraps に含まれないノードが境界内に入り込んでいないかを検査する。
func checkBoundaryContainment(l *render.Layout, ds *diag.List) {
	for bi, b := range l.Boundaries {
		if ds.AtCap() {
			return
		}
		wrapped := map[string]bool{}
		for _, id := range b.Wraps {
			wrapped[id] = true
		}
		for _, n := range l.Nodes {
			if wrapped[n.ID] {
				if !b.Rect.ContainsRect(n.Rect, 0) {
					d := diag.Error("composition/boundary-containment", fmt.Sprintf("/boundaries/%d", bi),
						fmt.Sprintf("境界 '%s' が wraps 対象 '%s' を囲めていません", b.Label, n.ID),
						"境界の wraps を見直す")
					d.Subject = &diag.Subject{Surface: "boundary"}
					diag.Append(ds, d)
				}
				continue
			}
			if n.Rect.Overlaps(b.Rect, 0) {
				d := diag.Error("composition/boundary-containment", fmt.Sprintf("/boundaries/%d", bi),
					fmt.Sprintf("wraps に含まれない component '%s' が境界 '%s' と重なっています", n.ID, b.Label),
					"component を境界の外へ移動する",
					"意図して含めるなら wraps に追加する")
				d.Subject = &diag.Subject{Surface: "component", ID: n.ID}
				d.Evidence = map[string]any{"boundary": rectEvidence(b.Rect), "node": rectEvidence(n.Rect)}
				diag.Append(ds, d)
			}
		}
	}
}

// checkViewBoxFit は全要素が viewBox 内に収まっているかを検査する。
func checkViewBoxFit(l *render.Layout, ds *diag.List) {
	vb := geometry.Rect{X: l.ViewBox[0], Y: l.ViewBox[1], W: l.ViewBox[2], H: l.ViewBox[3]}
	const tol = 0.5
	report := func(kind, id string, r geometry.Rect) {
		fix := "viewBox を広げるか要素を内側へ移動する"
		if !l.ViewBoxExplicit {
			fix = "内部エラーの可能性があります (自動 viewBox が要素を含んでいません)"
		}
		d := diag.Error("composition/viewbox-overflow", "",
			fmt.Sprintf("%s '%s' が viewBox の外にはみ出しています", kind, id), fix)
		d.Subject = &diag.Subject{Surface: kind, ID: id}
		d.Evidence = map[string]any{"viewBox": []float64{vb.X, vb.Y, vb.W, vb.H}, "element": rectEvidence(r)}
		diag.Append(ds, d)
	}
	inside := func(r geometry.Rect) bool {
		return r.X >= vb.X-tol && r.Y >= vb.Y-tol && r.MaxX() <= vb.MaxX()+tol && r.MaxY() <= vb.MaxY()+tol
	}
	for _, n := range l.Nodes {
		if !inside(n.Rect) {
			report("component", n.ID, n.Rect)
		}
		if n.HasTag && !inside(n.TagRect) {
			report("component", n.ID, n.TagRect)
		}
	}
	for _, b := range l.Boundaries {
		if !inside(b.Rect) {
			report("boundary", b.Label, b.Rect)
		}
	}
	for _, e := range l.Edges {
		for _, p := range e.Points {
			if !inside(geometry.Rect{X: p.X, Y: p.Y}) {
				report("connection", e.ID, geometry.Rect{X: p.X, Y: p.Y})
				break
			}
		}
		if e.HasLabel && !inside(e.LabelRect) {
			report("connection", e.ID, e.LabelRect)
		}
	}
	if len(l.Legend) > 0 && !inside(l.LegendRect) {
		report("legend", "legend", l.LegendRect)
	}
}

// checkEdgeThroughNode は辺がノードを不正に貫通していないかを検査する。
//
//   - 無関係なノード: どのセグメントも貫通してはならない。
//   - 端点ノード (from / to): 最初と最後のセグメント (ポートから出る/入るスタブ) 以外は
//     貫通してはならない。via/channel が経路を端点ノードへ逆流させると、経路がノードの下へ
//     潜って反対側から再出現する破綻が起きる。従来は端点ノードを一律除外していたため
//     showcase でも見逃していた (レビュー指摘)。
func checkEdgeThroughNode(l *render.Layout, ds *diag.List) {
	nodeByID := make(map[string]geometry.Rect, len(l.Nodes))
	for _, n := range l.Nodes {
		nodeByID[n.ID] = n.Rect
	}
	for _, e := range l.Edges {
		if ds.AtCap() {
			return
		}
		endpointRects := map[string]geometry.Rect{}
		if r, ok := nodeByID[e.From]; ok {
			endpointRects[e.From] = r
		}
		if r, ok := nodeByID[e.To]; ok {
			endpointRects[e.To] = r
		}
		nSeg := len(e.Points) - 1
		for _, n := range l.Nodes {
			isEndpoint := n.ID == e.From || n.ID == e.To
			for i := 1; i < len(e.Points); i++ {
				// 端点ノードでは最初 (i==1) と最後 (i==nSeg) のスタブは正当な出入りなので除外する。
				if isEndpoint && (i == 1 || i == nSeg) {
					continue
				}
				if geometry.SegIntersectsRect(e.Points[i-1], e.Points[i], n.Rect, 0) {
					msg := fmt.Sprintf("接続 '%s' が無関係な component '%s' を貫通しています", e.ID, n.ID)
					if isEndpoint {
						msg = fmt.Sprintf("接続 '%s' の経路が端点 component '%s' に再侵入しています", e.ID, n.ID)
					}
					d := diag.Error("composition/edge-through-node", "", msg,
						"via で経路を明示して迂回させる",
						"via の中継点が端点ノードの内側や逆側に来ないよう調整する",
						"component の pos を移動して経路を空ける")
					d.Subject = &diag.Subject{Surface: "connection", ID: e.ID}
					d.Evidence = map[string]any{"node": rectEvidence(n.Rect), "conflictsWith": n.ID}
					diag.Append(ds, d)
					break
				}
			}
		}
	}
}

// checkEdgeCrossing は無関係な辺同士の交差・重複を検査する (showcase warning)。
// 共有ポート (同じノードに接続する辺) は正当に近接しうるため、端点ノードを共有する
// ペアは対象外とする。交差は bridge/jump 表現を持たないため、可能なら避けるべき。
func checkEdgeCrossing(l *render.Layout, ds *diag.List) {
	for i := 0; i < len(l.Edges); i++ {
		if ds.AtCap() {
			return
		}
		for j := i + 1; j < len(l.Edges); j++ {
			a, b := l.Edges[i], l.Edges[j]
			if sharesEndpoint(a, b) {
				continue
			}
			if pt, ok := firstSegmentCrossing(a.Points, b.Points); ok {
				d := diag.Warning("composition/edge-crossing", "",
					fmt.Sprintf("接続 '%s' と '%s' が交差しています (交点付近で線の追跡が困難になります)", a.ID, b.ID),
					"どちらかを via で迂回させて交差を避ける",
					"ノード配置を見直して経路が重ならないようにする")
				d.Subject = &diag.Subject{Surface: "connection", ID: a.ID}
				d.Evidence = map[string]any{"at": []float64{pt.X, pt.Y}, "conflictsWith": b.ID}
				diag.Append(ds, d)
			}
		}
	}
}

// sharesEndpoint は 2 辺が端点ノードを共有するか。
func sharesEndpoint(a, b render.Edge) bool {
	return a.From == b.From || a.From == b.To || a.To == b.From || a.To == b.To
}

// firstSegmentCrossing は 2 つの折れ線が交差する最初の点を返す (軸平行セグメント前提)。
func firstSegmentCrossing(a, b []geometry.Point) (geometry.Point, bool) {
	for i := 1; i < len(a); i++ {
		for j := 1; j < len(b); j++ {
			if pt, ok := orthoSegCross(a[i-1], a[i], b[j-1], b[j]); ok {
				return pt, true
			}
		}
	}
	return geometry.Point{}, false
}

// orthoSegCross は軸平行な 2 セグメント (一方が水平、一方が垂直) の交点を返す。
// 平行なセグメント同士は交差なし扱い (端点接触は交差としない)。
func orthoSegCross(a1, a2, b1, b2 geometry.Point) (geometry.Point, bool) {
	aH := a1.Y == a2.Y
	bH := b1.Y == b2.Y
	if aH == bH {
		return geometry.Point{}, false // 同方向は判定しない (重複は稀で描画上も許容)
	}
	// a を水平、b を垂直に正規化する。
	h1, h2, v1, v2 := a1, a2, b1, b2
	if !aH {
		h1, h2, v1, v2 = b1, b2, a1, a2
	}
	y := h1.Y
	x := v1.X
	minHX, maxHX := min(h1.X, h2.X), max(h1.X, h2.X)
	minVY, maxVY := min(v1.Y, v2.Y), max(v1.Y, v2.Y)
	// 端点での接触 (T 字・角) は交差としない。厳密内部での交差のみ。
	if x > minHX && x < maxHX && y > minVY && y < maxVY {
		return geometry.Point{X: x, Y: y}, true
	}
	return geometry.Point{}, false
}

// checkLabelCollision は関係ラベルのマスク矩形の衝突を検査する。
//   - ラベル同士
//   - ラベルと他の経路 (自身の経路は除く)
//   - ラベルとノード矩形
func checkLabelCollision(l *render.Layout, ds *diag.List) {
	labeled := make([]render.Edge, 0, len(l.Edges))
	for _, e := range l.Edges {
		if e.HasLabel {
			labeled = append(labeled, e)
		}
	}
	for i := 0; i < len(labeled); i++ {
		if ds.AtCap() {
			return
		}
		for j := i + 1; j < len(labeled); j++ {
			if labeled[i].LabelRect.Overlaps(labeled[j].LabelRect, 0) {
				d := diag.Error("composition/label-collision", "",
					fmt.Sprintf("関係ラベル '%s' と '%s' が重なっています", labeled[i].ID, labeled[j].ID),
					"labelDy を調整してラベルを離す",
					"labelAt でラベル位置を経路上の別の場所に移す",
					"意味を保ったままラベルの文言を短くする")
				d.Subject = &diag.Subject{Surface: "connection", ID: labeled[i].ID}
				d.Evidence = map[string]any{
					"labelBox": rectEvidence(labeled[i].LabelRect), "conflictsWith": labeled[j].ID,
				}
				diag.Append(ds, d)
			}
		}
	}
	for _, le := range labeled {
		if ds.AtCap() {
			return
		}
		for _, e := range l.Edges {
			if e.ID == le.ID {
				continue
			}
			for i := 1; i < len(e.Points); i++ {
				if geometry.SegIntersectsRectClosed(e.Points[i-1], e.Points[i], le.LabelRect) {
					d := diag.Error("composition/label-collision", "",
						fmt.Sprintf("関係ラベル '%s' が経路 '%s' と重なっています", le.ID, e.ID),
						"labelDy を調整してラベルを経路から離す",
						"経路の間隔を広げる",
						"意味を保ったままラベルの文言を短くする")
					d.Subject = &diag.Subject{Surface: "connection", ID: le.ID}
					d.Evidence = map[string]any{
						"labelBox": rectEvidence(le.LabelRect), "conflictsWith": e.ID,
					}
					diag.Append(ds, d)
					break
				}
			}
		}
		for _, n := range l.Nodes {
			if le.LabelRect.Overlaps(n.Rect, 0) {
				d := diag.Error("composition/label-collision", "",
					fmt.Sprintf("関係ラベル '%s' が component '%s' と重なっています", le.ID, n.ID),
					"labelDy / labelAt を調整してラベルをノードから離す",
					"component の pos を移動する")
				d.Subject = &diag.Subject{Surface: "connection", ID: le.ID}
				d.Evidence = map[string]any{
					"labelBox": rectEvidence(le.LabelRect), "conflictsWith": n.ID,
				}
				diag.Append(ds, d)
			}
		}
	}
}

// checkPathQuality は 8px 未満のセグメント・16px 未満の内角ターンを検査する。
func checkPathQuality(l *render.Layout, ds *diag.List) {
	for _, e := range l.Edges {
		for _, issue := range geometry.PathQuality(e.Points) {
			var d diag.Diagnostic
			switch issue.Kind {
			case "short-segment":
				d = diag.Warning("composition/segment-too-short", "",
					fmt.Sprintf("接続 '%s' に %.1fpx のセグメントがあります (最小 %.0fpx)", e.ID, issue.Length, geometry.MinSegment),
					"via の中継点を間引くか位置を調整する",
					"ポート位置 (fromSide / toSide) を見直す")
			default:
				d = diag.Warning("composition/tight-turn", "",
					fmt.Sprintf("接続 '%s' に内角クリアランス %.1fpx のターンがあります (最小 %.0fpx)", e.ID, issue.Length, geometry.MinTurnClearance),
					"via の中継点を調整して曲がりを緩くする",
					"経路のチャネル (channelX / channelY) を外側に移す")
			}
			d.Subject = &diag.Subject{Surface: "connection", ID: e.ID}
			d.Evidence = map[string]any{"at": []float64{issue.At.X, issue.At.Y}, "length": issue.Length}
			diag.Append(ds, d)
		}
	}
}

// checkPortSpacing は同一辺のポートが重なっていないかを検査する。
func checkPortSpacing(l *render.Layout, ds *diag.List) {
	const minGap = 6.0
	for i := 0; i < len(l.Ports); i++ {
		if ds.AtCap() {
			return
		}
		for j := i + 1; j < len(l.Ports); j++ {
			a, b := l.Ports[i], l.Ports[j]
			if a.NodeID != b.NodeID || a.Side != b.Side {
				continue
			}
			if geometry.Dist(a.Point, b.Point) < minGap {
				d := diag.Error("composition/port-overlap", "",
					fmt.Sprintf("component '%s' の %s 辺で接続 '%s' と '%s' のポートが重なっています", a.NodeID, a.Side, a.EdgeID, b.EdgeID),
					"fromSide / toSide を別の辺に振り分ける",
					"via で経路を分離する")
				d.Subject = &diag.Subject{Surface: "component", ID: a.NodeID}
				d.Evidence = map[string]any{
					"a": []float64{a.Point.X, a.Point.Y}, "b": []float64{b.Point.X, b.Point.Y},
				}
				diag.Append(ds, d)
			}
		}
	}
}

// checkLegendConsistency は凡例エントリが実使用と一致しているかを検査する。
// 凡例は使用種別から生成されるため常に一致するはずであり、これは不変条件の検査。
func checkLegendConsistency(l *render.Layout, ds *diag.List) {
	usedTypes := map[string]bool{}
	for _, n := range l.Nodes {
		usedTypes[n.Type] = true
	}
	usedVariants := map[string]bool{}
	for _, e := range l.Edges {
		usedVariants[e.Variant] = true
	}
	legendTypes := map[string]bool{}
	legendVariants := map[string]bool{}
	for _, en := range l.Legend {
		if en.IsEdge {
			legendVariants[en.Kind] = true
		} else {
			legendTypes[en.Kind] = true
		}
	}
	for _, n := range l.Nodes {
		if !legendTypes[n.Type] {
			diag.Append(ds, diag.Error("composition/legend-mismatch", "",
				fmt.Sprintf("使用中のノード種別 '%s' が凡例にありません", n.Type),
				"レンダラの凡例生成を修正する (内部不変条件の違反)"))
			legendTypes[n.Type] = true // 同一種別の重複報告を避ける
		}
	}
	for _, en := range l.Legend {
		if en.IsEdge && !usedVariants[en.Kind] {
			diag.Append(ds, diag.Error("composition/legend-mismatch", "",
				fmt.Sprintf("凡例の variant '%s' は使用されていません", en.Kind),
				"レンダラの凡例生成を修正する (内部不変条件の違反)"))
		}
		if !en.IsEdge && !usedTypes[en.Kind] {
			diag.Append(ds, diag.Error("composition/legend-mismatch", "",
				fmt.Sprintf("凡例のノード種別 '%s' は使用されていません", en.Kind),
				"レンダラの凡例生成を修正する (内部不変条件の違反)"))
		}
	}
	for _, e := range l.Edges {
		if !legendVariants[e.Variant] {
			diag.Append(ds, diag.Error("composition/legend-mismatch", "",
				fmt.Sprintf("使用中の variant '%s' が凡例にありません", e.Variant),
				"レンダラの凡例生成を修正する (内部不変条件の違反)"))
			legendVariants[e.Variant] = true
		}
	}
}
