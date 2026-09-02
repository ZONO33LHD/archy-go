// route.go は辺の直交経路 (orthogonal routing) を計算する (§7)。
//
//   - 端点の辺 (side) は方向の契約: 最初のセグメントは fromSide に垂直に出て、
//     最後のセグメントは toSide に垂直に入る。
//   - via が指定されていればそれを尊重し、自動計算しない。
//   - 自動時は L 字 / Z 字 / コの字候補から他ノードを貫通しないものを選ぶ。
//   - 探索には反復回数の上限があり、上限到達時は最良候補を返し Truncated を立てる。
package geometry

import "math"

const (
	// StubLength はポートから最初の曲がりまでの最小突き出し長。
	StubLength = 20.0
	// MinSegment はこれ未満のセグメントを品質違反とする閾値 (px)。
	MinSegment = 8.0
	// MinTurnClearance は曲がり角の内角に要求する最小セグメント長 (px)。
	MinTurnClearance = 16.0
	// maxRouteIterations は候補評価の反復上限。無限ループの可能性を残さない。
	maxRouteIterations = 128
)

// RouteRequest は 1 本の辺のルーティング要求。
type RouteRequest struct {
	From, To         Rect
	FromSide, ToSide Side
	// FromOffset / ToOffset はポートスプレッドによる辺中心からのオフセット。
	FromOffset, ToOffset float64
	// Via は LLM が明示した中継点。非 nil なら自動計算しない。
	Via []Point
	// ChannelX / ChannelY は明示チャネル (垂直 / 水平)。
	ChannelX, ChannelY *float64
	// Obstacles は貫通してはならない不透明ノード矩形 (両端点のノードは含めない)。
	Obstacles []Rect
}

// RouteResult はルーティング結果。
type RouteResult struct {
	// Points は折れ線の頂点列 (ポート点を含む)。丸め済み。
	Points []Point
	// Truncated は反復上限に達し最良候補で打ち切ったか。
	Truncated bool
	// Crossings は最終経路が障害物と交差している数 (0 が正常)。
	Crossings int
}

// Route は直交経路を計算する。
func Route(req RouteRequest) RouteResult {
	pA := PortPoint(req.From, req.FromSide, req.FromOffset)
	pB := PortPoint(req.To, req.ToSide, req.ToOffset)

	var pts []Point
	truncated := false
	switch {
	case len(req.Via) > 0:
		pts = routeVia(pA, pB, req)
	case req.ChannelX != nil || req.ChannelY != nil:
		pts = routeChannel(pA, pB, req)
	default:
		pts, truncated = routeAuto(pA, pB, req)
	}

	pts = cleanPath(pts)
	for i := range pts {
		pts[i].X = round2(pts[i].X)
		pts[i].Y = round2(pts[i].Y)
	}
	return RouteResult{
		Points:    pts,
		Truncated: truncated,
		Crossings: countCrossings(pts, req.Obstacles),
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// routeVia は明示中継点を尊重しつつ、方向の契約 (最初は fromSide 外向き、
// 最後は toSide に垂直進入) を必ず守る。
//
// ポートから最初の曲がりまでのスタブ (a1 / b1) を両端に強制挿入することで、
// via がポートと同じ座標やノード内側にあっても「辺に垂直に出入りする」契約を保証する。
// スタブを入れない素朴な実装だと、fromSide:right なのに最初の via が左にある場合に
// 最初のセグメントがノード内部を横切ってしまう (端点ノードは衝突検査から除外されるため見逃される)。
func routeVia(pA, pB Point, req RouteRequest) []Point {
	a1 := stubOut(pA, req.FromSide)
	b1 := stubOut(pB, req.ToSide)
	pts := []Point{pA, a1}
	cur := a1
	horizontal := req.FromSide.Horizontal() // pA→a1 の軸

	// via 群のあと、必ず toSide スタブ b1 を経由してから pB へ入る。
	mids := append(append([]Point{}, req.Via...), b1)
	for _, t := range mids {
		if cur.X != t.X && cur.Y != t.Y {
			// 直前の軸を継続してから曲がる (直交性を保つ)。
			if horizontal {
				pts = append(pts, Point{t.X, cur.Y})
			} else {
				pts = append(pts, Point{cur.X, t.Y})
			}
			horizontal = !horizontal
		} else if cur.X == t.X && cur.Y != t.Y {
			horizontal = false
		} else if cur.Y == t.Y && cur.X != t.X {
			horizontal = true
		}
		pts = append(pts, t)
		cur = t
	}
	pts = append(pts, pB) // b1→pB は toSide に垂直
	return pts
}

// routeChannel は channelX / channelY 明示時の経路。
func routeChannel(pA, pB Point, req RouteRequest) []Point {
	a1 := stubOut(pA, req.FromSide)
	b1 := stubOut(pB, req.ToSide)
	if req.ChannelX != nil {
		cx := *req.ChannelX
		return []Point{pA, a1, {cx, a1.Y}, {cx, b1.Y}, b1, pB}
	}
	cy := *req.ChannelY
	return []Point{pA, a1, {a1.X, cy}, {b1.X, cy}, b1, pB}
}

func stubOut(p Point, s Side) Point {
	d := s.Outward()
	return Point{p.X + d.X*StubLength, p.Y + d.Y*StubLength}
}

// routeAuto は候補列挙 + スコアリングで自動ルーティングする。
func routeAuto(pA, pB Point, req RouteRequest) ([]Point, bool) {
	a1 := stubOut(pA, req.FromSide)
	b1 := stubOut(pB, req.ToSide)

	// 候補の中間点列を列挙する。順序は固定 (決定論)。
	var middles [][]Point
	// L 字 2 種
	middles = append(middles, []Point{{a1.X, b1.Y}})
	middles = append(middles, []Point{{b1.X, a1.Y}})
	// Z 字 (水平中間チャネル / 垂直中間チャネル)
	midX := round2((a1.X + b1.X) / 2)
	midY := round2((a1.Y + b1.Y) / 2)
	middles = append(middles, []Point{{midX, a1.Y}, {midX, b1.Y}})
	middles = append(middles, []Point{{a1.X, midY}, {b1.X, midY}})
	// コの字 (外周チャネル)。障害物と両端点を含む全体 bbox の外側を回る。
	all := append([]Rect{req.From, req.To}, req.Obstacles...)
	bb := BoundingBox(all)
	for _, off := range []float64{32, 64, 96} {
		top := bb.Y - off
		bottom := bb.MaxY() + off
		left := bb.X - off
		right := bb.MaxX() + off
		middles = append(middles,
			[]Point{{a1.X, top}, {b1.X, top}},
			[]Point{{a1.X, bottom}, {b1.X, bottom}},
			[]Point{{left, a1.Y}, {left, b1.Y}},
			[]Point{{right, a1.Y}, {right, b1.Y}},
		)
	}

	best := []Point(nil)
	bestScore := math.Inf(1)
	iterations := 0
	truncated := false
	for _, mid := range middles {
		iterations++
		if iterations > maxRouteIterations {
			truncated = true
			break
		}
		cand := buildCandidate(pA, a1, mid, b1, pB, req)
		if cand == nil {
			continue
		}
		score := scoreCandidate(cand, req.Obstacles)
		if score < bestScore {
			bestScore = score
			best = cand
		}
	}
	if best == nil {
		// すべて不正 (理論上起きないが保険): 最小 L 字を返す。
		best = cleanPath([]Point{pA, a1, {a1.X, b1.Y}, b1, pB})
	}
	return best, truncated
}

// buildCandidate は候補経路を組み立て、直交性が破れていれば nil を返す。
func buildCandidate(pA, a1 Point, mid []Point, b1, pB Point, req RouteRequest) []Point {
	pts := append([]Point{pA, a1}, mid...)
	pts = append(pts, b1, pB)
	pts = cleanPath(pts)
	for i := 1; i < len(pts); i++ {
		if pts[i-1].X != pts[i].X && pts[i-1].Y != pts[i].Y {
			return nil // 斜めセグメントが生じる候補は破棄
		}
	}
	// 方向の契約: 最初のセグメントは fromSide の外向き、最後は toSide の内向き。
	if len(pts) >= 2 {
		if !segFollowsDir(pts[0], pts[1], req.FromSide.Outward()) {
			return nil
		}
		out := req.ToSide.Outward()
		if !segFollowsDir(pts[len(pts)-1], pts[len(pts)-2], out) {
			return nil
		}
	}
	return pts
}

func segFollowsDir(from, to Point, dir Point) bool {
	dx, dy := to.X-from.X, to.Y-from.Y
	if dir.X != 0 {
		return dy == 0 && dx*dir.X > 0
	}
	return dx == 0 && dy*dir.Y > 0
}

// scoreCandidate は候補のスコア (小さいほど良い)。
// 交差 1000 / ターン 10 / 短セグメント 50 / タイトターン 20 / 長さ 0.01。
func scoreCandidate(pts []Point, obstacles []Rect) float64 {
	score := float64(countCrossings(pts, obstacles)) * 1000
	turns := 0
	for i := 2; i < len(pts); i++ {
		if turnAt(pts[i-2], pts[i-1], pts[i]) {
			turns++
			if Dist(pts[i-2], pts[i-1]) < MinTurnClearance || Dist(pts[i-1], pts[i]) < MinTurnClearance {
				score += 20
			}
		}
	}
	score += float64(turns) * 10
	for i := 1; i < len(pts); i++ {
		if d := Dist(pts[i-1], pts[i]); d > 0 && d < MinSegment {
			score += 50
		}
	}
	score += PathLength(pts) * 0.01
	return score
}

func turnAt(a, b, c Point) bool {
	abH := a.Y == b.Y
	bcH := b.Y == c.Y
	return abH != bcH
}

// countCrossings は経路が障害物矩形を貫通する数を数える。
func countCrossings(pts []Point, obstacles []Rect) int {
	n := 0
	for i := 1; i < len(pts); i++ {
		for _, ob := range obstacles {
			if SegIntersectsRect(pts[i-1], pts[i], ob, 0) {
				n++
			}
		}
	}
	return n
}

// cleanPath は連続重複点と同一直線上の中間点を除去する。
func cleanPath(pts []Point) []Point {
	if len(pts) == 0 {
		return pts
	}
	out := []Point{pts[0]}
	for _, p := range pts[1:] {
		if p == out[len(out)-1] {
			continue
		}
		out = append(out, p)
	}
	// 同一直線上の中間点を除去する。ただし単調な (折り返さない) 共線のみ。
	// 折り返し点 (方向が反転する角) を除去すると、ポートスタブが潰れて方向契約が破れるため残す。
	cleaned := []Point{out[0]}
	for i := 1; i < len(out); i++ {
		if i+1 < len(out) {
			a, b, c := cleaned[len(cleaned)-1], out[i], out[i+1]
			if a.Y == b.Y && b.Y == c.Y && (b.X-a.X)*(c.X-b.X) >= 0 {
				continue // 水平の単調共線 → 中間点を除去
			}
			if a.X == b.X && b.X == c.X && (b.Y-a.Y)*(c.Y-b.Y) >= 0 {
				continue // 垂直の単調共線 → 中間点を除去
			}
		}
		cleaned = append(cleaned, out[i])
	}
	return cleaned
}

// QualityIssue は経路品質の違反。
type QualityIssue struct {
	// Kind は "short-segment" | "tight-turn"。
	Kind string
	// Length は問題のセグメント長。
	Length float64
	// At は問題の位置 (セグメント始点 / 曲がり角)。
	At Point
}

// PathQuality は経路の品質違反 (8px 未満のセグメント、16px 未満の内角ターン) を列挙する。
// ポート直近のセグメント (スタブ) も対象とする。
func PathQuality(pts []Point) []QualityIssue {
	var issues []QualityIssue
	for i := 1; i < len(pts); i++ {
		if d := Dist(pts[i-1], pts[i]); d > 0 && d < MinSegment {
			issues = append(issues, QualityIssue{Kind: "short-segment", Length: d, At: pts[i-1]})
		}
	}
	for i := 2; i < len(pts); i++ {
		if turnAt(pts[i-2], pts[i-1], pts[i]) {
			d1 := Dist(pts[i-2], pts[i-1])
			d2 := Dist(pts[i-1], pts[i])
			if d := math.Min(d1, d2); d < MinTurnClearance {
				issues = append(issues, QualityIssue{Kind: "tight-turn", Length: d, At: pts[i-1]})
			}
		}
	}
	return issues
}
