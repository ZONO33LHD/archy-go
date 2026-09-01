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

// CheckName は receipt に載せる構図検査の名前一覧 (実行順)。
var CheckNames = []string{
	"node-overlap",
	"text-overflow",
	"boundary-containment",
	"viewbox-fit",
	"edge-through-node",
	"label-collision",
	"path-quality",
	"port-spacing",
	"legend-consistency",
}

// standardChecks は standard プロファイルで実行する基本チェック。
var standardChecks = map[string]bool{
	"node-overlap":         true,
	"text-overflow":        true,
	"boundary-containment": true,
	"viewbox-fit":          true,
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
	var results []CheckResult

	run := func(name string, fn func(*diag.List)) {
		res := CheckResult{Name: name}
		if showcase || standardChecks[name] {
			res.Ran = true
			before := len(ds)
			fn(&ds)
			res.Issues = len(ds) - before
		}
		results = append(results, res)
	}

	run("node-overlap", func(out *diag.List) { checkNodeOverlap(l, out) })
	run("text-overflow", func(out *diag.List) {
		for _, d := range layoutDiags {
			if d.Code == "composition/text-overflow" {
				*out = append(*out, d)
			}
		}
	})
	run("boundary-containment", func(out *diag.List) { checkBoundaryContainment(l, out) })
	run("viewbox-fit", func(out *diag.List) { checkViewBoxFit(l, out) })
	run("edge-through-node", func(out *diag.List) { checkEdgeThroughNode(l, out) })
	run("label-collision", func(out *diag.List) { checkLabelCollision(l, out) })
	run("path-quality", func(out *diag.List) { checkPathQuality(l, out) })
	run("port-spacing", func(out *diag.List) { checkPortSpacing(l, out) })
	run("legend-consistency", func(out *diag.List) { checkLegendConsistency(l, out) })

	// text-overflow 以外のレイアウト診断 (route-truncated 等) も合流させる。
	for _, d := range layoutDiags {
		if d.Code != "composition/text-overflow" {
			ds = append(ds, d)
		}
	}
	return ds, results
}

// checkNodeOverlap はコンポーネント矩形同士の重なりを検査する。
func checkNodeOverlap(l *render.Layout, ds *diag.List) {
	for i := 0; i < len(l.Nodes); i++ {
		if ds.AtCap() {
			return
		}
		for j := i + 1; j < len(l.Nodes); j++ {
			a, b := l.Nodes[i], l.Nodes[j]
			if a.Rect.Overlaps(b.Rect, 0) {
				d := diag.Error("composition/node-overlap", fmt.Sprintf("/components/%d", j),
					fmt.Sprintf("component '%s' と '%s' の矩形が重なっています", a.ID, b.ID),
					"pos を調整して重なりを解消する",
					"size を縮小して間隔を確保する")
				d.Subject = &diag.Subject{Surface: "component", ID: b.ID}
				d.Evidence = map[string]any{
					"a": rectEvidence(a.Rect), "b": rectEvidence(b.Rect), "conflictsWith": a.ID,
				}
				diag.Append(ds, d)
			}
		}
	}
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

// checkEdgeThroughNode は辺が無関係な不透明ノードを貫通していないかを検査する。
func checkEdgeThroughNode(l *render.Layout, ds *diag.List) {
	for _, e := range l.Edges {
		if ds.AtCap() {
			return
		}
		for _, n := range l.Nodes {
			if n.ID == e.From || n.ID == e.To {
				continue
			}
			for i := 1; i < len(e.Points); i++ {
				if geometry.SegIntersectsRect(e.Points[i-1], e.Points[i], n.Rect, 0) {
					d := diag.Error("composition/edge-through-node", "",
						fmt.Sprintf("接続 '%s' が無関係な component '%s' を貫通しています", e.ID, n.ID),
						"via で経路を明示して迂回させる",
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
