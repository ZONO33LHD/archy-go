package geometry

import (
	"testing"
	"time"
)

func TestRouteStraight(t *testing.T) {
	// 対向ポートが同一軸 → 直線 1 セグメント。
	res := Route(RouteRequest{
		From: Rect{0, 0, 100, 50}, To: Rect{200, 0, 100, 50},
		FromSide: SideRight, ToSide: SideLeft,
	})
	if len(res.Points) != 2 {
		t.Fatalf("直線経路の頂点数 = %d, want 2 (%v)", len(res.Points), res.Points)
	}
	if res.Points[0] != (Point{100, 25}) || res.Points[1] != (Point{200, 25}) {
		t.Errorf("ポート位置が想定と異なります: %v", res.Points)
	}
	if res.Crossings != 0 {
		t.Errorf("Crossings = %d", res.Crossings)
	}
}

func TestRouteSideContract(t *testing.T) {
	// 方向の契約: fromSide=right なら最初のセグメントは右向き、
	// toSide=top なら最後のセグメントは上辺に垂直に入る。
	res := Route(RouteRequest{
		From: Rect{0, 0, 100, 50}, To: Rect{300, 200, 100, 50},
		FromSide: SideRight, ToSide: SideTop,
	})
	pts := res.Points
	if len(pts) < 2 {
		t.Fatal("経路が短すぎます")
	}
	first := [2]Point{pts[0], pts[1]}
	if first[1].X <= first[0].X || first[1].Y != first[0].Y {
		t.Errorf("最初のセグメントが右向き水平ではありません: %v", first)
	}
	last := [2]Point{pts[len(pts)-2], pts[len(pts)-1]}
	if last[1].Y <= last[0].Y || last[1].X != last[0].X {
		t.Errorf("最後のセグメントが上から下向き垂直 (top 進入) ではありません: %v", last)
	}
}

func TestRouteViaRespected(t *testing.T) {
	via := []Point{{620, 142}, {620, 246}}
	res := Route(RouteRequest{
		From: Rect{470, 100, 130, 60}, To: Rect{690, 220, 130, 60},
		FromSide: SideRight, ToSide: SideLeft,
		Via: via,
	})
	// 共線の via 点は頂点としては簡約されるが、経路は必ずその上を通る。
	for _, v := range via {
		if !pointOnPath(res.Points, v) {
			t.Errorf("via 点 %v が経路上にありません: %v", v, res.Points)
		}
	}
}

// pointOnPath は点が折れ線のいずれかのセグメント上にあるか (軸平行前提)。
func pointOnPath(pts []Point, p Point) bool {
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if a.Y == b.Y && p.Y == a.Y &&
			p.X >= min(a.X, b.X) && p.X <= max(a.X, b.X) {
			return true
		}
		if a.X == b.X && p.X == a.X &&
			p.Y >= min(a.Y, b.Y) && p.Y <= max(a.Y, b.Y) {
			return true
		}
	}
	return false
}

// TestRouteViaHonorsSideContract: via 指定でも最初/最後のセグメントが
// fromSide/toSide に垂直に出入りする (レビュー HIGH#7 の回帰)。
func TestRouteViaHonorsSideContract(t *testing.T) {
	// fromSide=right なのに via がノード左側にある悪意的なケース。
	res := Route(RouteRequest{
		From: Rect{200, 100, 100, 50}, To: Rect{200, 300, 100, 50},
		FromSide: SideRight, ToSide: SideRight,
		Via: []Point{{100, 200}}, // 左側の via
	})
	pts := res.Points
	if len(pts) < 2 {
		t.Fatal("経路が短すぎます")
	}
	// 最初のセグメントは右向き水平 (fromSide=right の契約)。
	if pts[1].X <= pts[0].X || pts[1].Y != pts[0].Y {
		t.Errorf("最初のセグメントが right 契約に反します: %v -> %v", pts[0], pts[1])
	}
	// 最後のセグメントは右から左へ (toSide=right へ垂直進入)。
	last0, last1 := pts[len(pts)-2], pts[len(pts)-1]
	if last1.X >= last0.X || last1.Y != last0.Y {
		t.Errorf("最後のセグメントが right 進入契約に反します: %v -> %v", last0, last1)
	}
	// 全セグメントが直交している。
	for i := 1; i < len(pts); i++ {
		if pts[i-1].X != pts[i].X && pts[i-1].Y != pts[i].Y {
			t.Errorf("斜めセグメントがあります: %v -> %v", pts[i-1], pts[i])
		}
	}
}

func TestRouteAvoidsObstacle(t *testing.T) {
	// 中央に障害物 → 貫通しない候補が選ばれる。
	res := Route(RouteRequest{
		From: Rect{0, 100, 100, 50}, To: Rect{400, 100, 100, 50},
		FromSide: SideRight, ToSide: SideLeft,
		Obstacles: []Rect{{200, 50, 100, 150}},
	})
	if res.Crossings != 0 {
		t.Errorf("障害物を貫通しています: %v (crossings=%d)", res.Points, res.Crossings)
	}
}

func TestRouteDeterministic(t *testing.T) {
	req := RouteRequest{
		From: Rect{0, 0, 100, 50}, To: Rect{300, 200, 100, 50},
		FromSide: SideBottom, ToSide: SideLeft,
		Obstacles: []Rect{{150, 80, 60, 60}, {100, 150, 40, 40}},
	}
	first := Route(req)
	for range 50 {
		got := Route(req)
		if len(got.Points) != len(first.Points) {
			t.Fatal("経路が実行ごとに変わります")
		}
		for i := range got.Points {
			if got.Points[i] != first.Points[i] {
				t.Fatal("経路の頂点が実行ごとに変わります")
			}
		}
	}
}

func TestRouteChannelX(t *testing.T) {
	cx := 250.0
	res := Route(RouteRequest{
		From: Rect{0, 0, 100, 50}, To: Rect{400, 200, 100, 50},
		FromSide: SideRight, ToSide: SideLeft,
		ChannelX: &cx,
	})
	found := false
	for i := 1; i < len(res.Points); i++ {
		if res.Points[i-1].X == cx && res.Points[i].X == cx {
			found = true
		}
	}
	if !found {
		t.Errorf("channelX=%v の垂直チャネルが使われていません: %v", cx, res.Points)
	}
}

func TestPathQuality(t *testing.T) {
	// 5px の短セグメントと 10px の内角ターン。
	pts := []Point{{0, 0}, {5, 0}, {5, 100}}
	issues := PathQuality(pts)
	var kinds []string
	for _, i := range issues {
		kinds = append(kinds, i.Kind)
	}
	hasShort, hasTight := false, false
	for _, k := range kinds {
		if k == "short-segment" {
			hasShort = true
		}
		if k == "tight-turn" {
			hasTight = true
		}
	}
	if !hasShort || !hasTight {
		t.Errorf("品質違反の検出漏れ: %v", kinds)
	}
	// 健全な経路は違反なし。
	if got := PathQuality([]Point{{0, 0}, {100, 0}, {100, 100}}); len(got) != 0 {
		t.Errorf("健全な経路で違反が検出されました: %v", got)
	}
}

// FuzzRoute: 極端な座標配置でもパニックせず有限時間で返る (§13)。
func FuzzRoute(f *testing.F) {
	f.Add(0.0, 0.0, 100.0, 50.0, 300.0, 200.0, 100.0, 50.0, 0, 1)
	f.Add(-1e6, -1e6, 1.0, 1.0, 1e6, 1e6, 1.0, 1.0, 2, 3)
	f.Add(0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0, 0)
	sides := []Side{SideLeft, SideRight, SideTop, SideBottom}
	f.Fuzz(func(t *testing.T, x1, y1, w1, h1, x2, y2, w2, h2 float64, s1, s2 int) {
		if s1 < 0 || s2 < 0 {
			return
		}
		start := time.Now()
		res := Route(RouteRequest{
			From: Rect{x1, y1, w1, h1}, To: Rect{x2, y2, w2, h2},
			FromSide: sides[s1%4], ToSide: sides[s2%4],
			Obstacles: []Rect{{x1 + x2, y1 + y2, w1, h2}},
		})
		if len(res.Points) < 2 {
			t.Error("経路の頂点が 2 未満です")
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("ルーティングに %v かかりました (無限ループの疑い)", elapsed)
		}
	})
}
