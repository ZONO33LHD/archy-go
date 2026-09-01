// Package geometry は直交ルーティング・ポート計算・衝突判定を提供する。
//
// すべての計算は決定論的である。辺の処理順序は入力配列順に固定され、
// マップの反復順に依存しない。座標は計算の最終段で Round2 相当の丸めを受ける。
package geometry

import "math"

// Point は 2 次元座標。
type Point struct {
	X, Y float64
}

// Rect は軸平行の矩形。
type Rect struct {
	X, Y, W, H float64
}

func (r Rect) MaxX() float64 { return r.X + r.W }
func (r Rect) MaxY() float64 { return r.Y + r.H }
func (r Rect) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }
func (r Rect) IsEmpty() bool { return r.W <= 0 || r.H <= 0 }

// Overlaps は 2 矩形が重なるか (gap 分のクリアランスを要求)。
func (r Rect) Overlaps(o Rect, gap float64) bool {
	return r.X < o.MaxX()+gap && o.X < r.MaxX()+gap &&
		r.Y < o.MaxY()+gap && o.Y < r.MaxY()+gap
}

// ContainsRect は o が r の内部 (pad 分の余白込み) に収まるか。
func (r Rect) ContainsRect(o Rect, pad float64) bool {
	return o.X >= r.X+pad && o.Y >= r.Y+pad &&
		o.MaxX() <= r.MaxX()-pad && o.MaxY() <= r.MaxY()-pad
}

// ContainsPoint は点が矩形内 (inflate 分拡大) にあるか。
func (r Rect) ContainsPoint(p Point, inflate float64) bool {
	return p.X >= r.X-inflate && p.X <= r.MaxX()+inflate &&
		p.Y >= r.Y-inflate && p.Y <= r.MaxY()+inflate
}

// Inflate は矩形を四方に d だけ拡大した矩形を返す (非破壊)。
func (r Rect) Inflate(d float64) Rect {
	return Rect{r.X - d, r.Y - d, r.W + 2*d, r.H + 2*d}
}

// Union は 2 矩形を包含する最小の矩形を返す。
func (r Rect) Union(o Rect) Rect {
	x1 := math.Min(r.X, o.X)
	y1 := math.Min(r.Y, o.Y)
	x2 := math.Max(r.MaxX(), o.MaxX())
	y2 := math.Max(r.MaxY(), o.MaxY())
	return Rect{x1, y1, x2 - x1, y2 - y1}
}

// BoundingBox は矩形群を包含する矩形を返す。空なら zero 値。
func BoundingBox(rects []Rect) Rect {
	if len(rects) == 0 {
		return Rect{}
	}
	bb := rects[0]
	for _, r := range rects[1:] {
		bb = bb.Union(r)
	}
	return bb
}

// Side はポートが取り付く辺の向き。方向の契約であり、
// fromSide: "right" なら最初のセグメントは必ず右向きに垂直に出る。
type Side string

const (
	SideLeft   Side = "left"
	SideRight  Side = "right"
	SideTop    Side = "top"
	SideBottom Side = "bottom"
)

// Horizontal は左右の辺 (= 水平に出入りする) か。
func (s Side) Horizontal() bool { return s == SideLeft || s == SideRight }

// Outward は辺の外向き単位ベクトル。
func (s Side) Outward() Point {
	switch s {
	case SideLeft:
		return Point{-1, 0}
	case SideRight:
		return Point{1, 0}
	case SideTop:
		return Point{0, -1}
	default:
		return Point{0, 1}
	}
}

// PortPoint は矩形 r の辺 s 上のポート座標を返す。
// offset は辺の中心からの符号付きオフセット (辺に沿った方向)。
func PortPoint(r Rect, s Side, offset float64) Point {
	c := r.Center()
	switch s {
	case SideLeft:
		return Point{r.X, c.Y + offset}
	case SideRight:
		return Point{r.MaxX(), c.Y + offset}
	case SideTop:
		return Point{c.X + offset, r.Y}
	default:
		return Point{c.X + offset, r.MaxY()}
	}
}

// SideLength は辺 s の長さ。
func SideLength(r Rect, s Side) float64 {
	if s.Horizontal() {
		return r.H
	}
	return r.W
}

// AutoSides は fromSide / toSide が省略されたときの既定の辺を、
// 2 矩形の相対位置から決定論的に選ぶ。
func AutoSides(from, to Rect) (Side, Side) {
	fc, tc := from.Center(), to.Center()
	dx, dy := tc.X-fc.X, tc.Y-fc.Y
	if math.Abs(dx) >= math.Abs(dy) {
		if dx >= 0 {
			return SideRight, SideLeft
		}
		return SideLeft, SideRight
	}
	if dy >= 0 {
		return SideBottom, SideTop
	}
	return SideTop, SideBottom
}

// SegIntersectsRect は軸平行セグメント a-b が矩形 r (inflate 分拡大) と交差するか。
// セグメントは水平または垂直であることを前提とする。
func SegIntersectsRect(a, b Point, r Rect, inflate float64) bool {
	rr := r.Inflate(inflate)
	if a.Y == b.Y { // 水平
		if a.Y <= rr.Y || a.Y >= rr.MaxY() {
			return false
		}
		lo, hi := math.Min(a.X, b.X), math.Max(a.X, b.X)
		return lo < rr.MaxX() && hi > rr.X
	}
	// 垂直
	if a.X <= rr.X || a.X >= rr.MaxX() {
		return false
	}
	lo, hi := math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
	return lo < rr.MaxY() && hi > rr.Y
}

// SegIntersectsRectClosed は境界接触も交差とみなす版 (ラベル衝突判定用)。
func SegIntersectsRectClosed(a, b Point, r Rect) bool {
	if a.Y == b.Y {
		if a.Y < r.Y || a.Y > r.MaxY() {
			return false
		}
		lo, hi := math.Min(a.X, b.X), math.Max(a.X, b.X)
		return lo <= r.MaxX() && hi >= r.X
	}
	if a.X < r.X || a.X > r.MaxX() {
		return false
	}
	lo, hi := math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
	return lo <= r.MaxY() && hi >= r.Y
}

// Dist は 2 点間のユークリッド距離。
func Dist(a, b Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

// PathLength は折れ線の全長。
func PathLength(pts []Point) float64 {
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += Dist(pts[i-1], pts[i])
	}
	return total
}

// PointAt は折れ線上の位置 t (0..1) の点を返す。
func PointAt(pts []Point, t float64) Point {
	if len(pts) == 0 {
		return Point{}
	}
	if len(pts) == 1 || t <= 0 {
		return pts[0]
	}
	if t >= 1 {
		return pts[len(pts)-1]
	}
	target := PathLength(pts) * t
	acc := 0.0
	for i := 1; i < len(pts); i++ {
		d := Dist(pts[i-1], pts[i])
		if acc+d >= target && d > 0 {
			f := (target - acc) / d
			return Point{
				X: pts[i-1].X + (pts[i].X-pts[i-1].X)*f,
				Y: pts[i-1].Y + (pts[i].Y-pts[i-1].Y)*f,
			}
		}
		acc += d
	}
	return pts[len(pts)-1]
}
