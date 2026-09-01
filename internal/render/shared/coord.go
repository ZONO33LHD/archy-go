// coord.go は座標の丸めと書式を一元化する (§9)。
//
// 座標を出力する関数は Coord ただ 1 つであり、全レンダラがこれを通す。
// 丸めは書式化時点ではなく計算時点 (Round2) で行い、丸めた値を後段に渡す。
// そうしないと検証 (丸め前) と描画 (丸め後) がずれ、
// 「検証は通るのに図が壊れている」状態になる。
package shared

import (
	"math"
	"strconv"
	"strings"
)

// Round2 は小数第 2 位への丸め。ジオメトリ計算の最終段で必ず適用する。
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// Coord は座標を文字列化する。小数第 2 位で丸め、末尾のゼロを落とす。
//
//	100.0   → "100"
//	100.50  → "100.5"
//	100.456 → "100.46"
//	-0.0    → "0"
func Coord(v float64) string {
	r := Round2(v)
	if r == 0 {
		return "0" // "-0" を作らない
	}
	s := strconv.FormatFloat(r, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return s
}

// Font はフォントサイズの文字列化 (0.1 単位)。
func Font(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}
