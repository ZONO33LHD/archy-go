// Package shared はレンダラ共通のテキスト計測・数値書式ユーティリティを提供する。
//
// SVG のテキストは折り返されないため、幅を「テキスト単位 (units)」で近似し、
// 収まらないものは検証エラーにする (静かに溢れるくらいなら落とす)。
package shared

//go:generate go run ./gen/widthtable

const (
	// UnitWidthFactor は 1 unit あたりの幅係数 (units × fontSize × 0.6)。
	UnitWidthFactor = 0.6
	// BoxPadding はボックス左右パディングの合計 (px)。
	BoxPadding = 8.0
)

// TextUnits は文字列の幅を units で数える (§6)。
//
//	全角文字 (CJK・全角記号)               → 2
//	U+FE0F (絵文字表示 VS) が後続する文字    → 2
//	U+FE0E (テキスト表示 VS) が後続する文字  → 1
//	バリエーションセレクタ自体              → 0 (スキップ)
//	それ以外                              → 1
//
// 改行を含む場合は最長行の units を返す (SVG では行ごとに tspan 描画する)。
func TextUnits(s string) int {
	max := 0
	cur := 0
	rs := []rune(s) // コードポイント単位で走査する
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\n' {
			if cur > max {
				max = cur
			}
			cur = 0
			continue
		}
		if r == 0xFE0F || r == 0xFE0E {
			continue // 単独のバリエーションセレクタは 0
		}
		if i+1 < len(rs) {
			switch rs[i+1] {
			case 0xFE0F:
				cur += 2
				i++
				continue
			case 0xFE0E:
				cur++
				i++
				continue
			}
		}
		if isWideRune(r) {
			cur += 2
		} else {
			cur++
		}
	}
	if cur > max {
		max = cur
	}
	return max
}

// TextWidth は fontSize での見積り幅 (px) を返す。
func TextWidth(s string, fontSize float64) float64 {
	return float64(TextUnits(s)) * fontSize * UnitWidthFactor
}

// FitFont はボックス幅 boxWidth に文字列 s を収めるフォントサイズを計算する。
//
//	fitted   = min(preferred, available / (units × 0.6))
//	fontSize = max(minimum, floor(fitted × 10) / 10)   ← 0.1px 単位に切り捨て
//
// 最小サイズまで縮めても収まらない場合は ok=false を返す。
// 丸めは計算時点で行い、丸めた値をそのまま検証と描画の両方に使う (§9)。
func FitFont(s string, boxWidth, preferred, minimum float64) (fontSize float64, ok bool) {
	units := TextUnits(s)
	available := boxWidth - BoxPadding
	if units == 0 {
		return preferred, true
	}
	if available <= 0 {
		return minimum, false
	}
	fitted := available / (float64(units) * UnitWidthFactor)
	if fitted > preferred {
		fitted = preferred
	}
	fontSize = floor1(fitted)
	if fontSize < minimum {
		fontSize = minimum
	}
	// 丸め後のサイズで実際に収まるかを最終判定する (検証と描画のズレを作らない)。
	if float64(units)*UnitWidthFactor*fontSize > available {
		return fontSize, false
	}
	return fontSize, true
}

// floor1 は 0.1 単位への切り捨て。浮動小数点の丸め誤差を避けるため
// 一旦 1e-9 のイプシロンを足してから floor する (10.0*3 = 29.999... 対策)。
func floor1(v float64) float64 {
	scaled := v*10 + 1e-9
	return float64(int64(scaled)) / 10
}
