// svg.go は確定済み Layout から SVG 文字列を生成する。
//
// セキュリティ (§10.3):
//   - IR 由来の文字列は必ず safe.HTML を通す
//   - foreignObject / xlink:href / on* 属性 / style 属性は一切出力しない
//   - id 属性に使う ID は検証済みだが、防御として safe.IsValidID を再確認する
package architecture

import (
	"strconv"
	"strings"

	"github.com/ZONO33LHD/archy-go/internal/geometry"
	"github.com/ZONO33LHD/archy-go/internal/render"
	"github.com/ZONO33LHD/archy-go/internal/render/shared"
	"github.com/ZONO33LHD/archy-go/internal/safe"
)

// SVG は Layout から SVG を生成する。非決定性はない。
func (r *Renderer) SVG(l *render.Layout) string {
	var b strings.Builder
	b.Grow(1 << 16)

	b.WriteString(`<svg class="vd-svg" xmlns="http://www.w3.org/2000/svg" viewBox="`)
	b.WriteString(shared.Coord(l.ViewBox[0]) + " " + shared.Coord(l.ViewBox[1]) + " " +
		shared.Coord(l.ViewBox[2]) + " " + shared.Coord(l.ViewBox[3]))
	b.WriteString(`" role="img" aria-labelledby="vd-svg-title vd-svg-desc" tabindex="0">` + "\n")
	b.WriteString(`<title id="vd-svg-title">` + safe.HTML(l.Title) + "</title>\n")
	b.WriteString(`<desc id="vd-svg-desc">` + safe.HTML(svgDescription(l)) + "</desc>\n")

	writeDefs(&b)
	b.WriteString(`<g id="vd-canvas">` + "\n")

	for _, bd := range l.Boundaries {
		writeBoundary(&b, bd)
	}
	for _, e := range l.Edges {
		writeEdge(&b, e)
	}
	for _, n := range l.Nodes {
		writeNode(&b, n)
	}
	writeLegend(&b, l)

	b.WriteString("</g>\n</svg>\n")
	return b.String()
}

// writeDefs は矢印マーカーを定義する。id は固定文字列のみ。
func writeDefs(b *strings.Builder) {
	b.WriteString("<defs>\n")
	for _, v := range variantOrder {
		b.WriteString(`<marker id="vd-arrow-` + v + `" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">`)
		b.WriteString(`<path d="M 0 1 L 9 5 L 0 9 z" class="vd-arrowhead vd-v-` + v + `"/>`)
		b.WriteString("</marker>\n")
	}
	b.WriteString("</defs>\n")
}

func rectAttrs(r geometry.Rect) string {
	return `x="` + shared.Coord(r.X) + `" y="` + shared.Coord(r.Y) +
		`" width="` + shared.Coord(r.W) + `" height="` + shared.Coord(r.H) + `"`
}

// textEl は複数行対応のテキスト要素を書き出す (行は tspan で分割)。
func textEl(b *strings.Builder, x, y, font float64, class, anchor, s string) {
	b.WriteString(`<text x="` + shared.Coord(x) + `" y="` + shared.Coord(y) +
		`" font-size="` + shared.Font(font) + `" class="` + class + `" text-anchor="` + anchor + `">`)
	lines := strings.Split(s, "\n")
	if len(lines) == 1 {
		b.WriteString(safe.HTML(s))
	} else {
		for i, line := range lines {
			dy := "0"
			if i > 0 {
				dy = shared.Font(font * 1.25)
			}
			b.WriteString(`<tspan x="` + shared.Coord(x) + `" dy="` + dy + `">` + safe.HTML(line) + `</tspan>`)
		}
	}
	b.WriteString("</text>\n")
}

func writeBoundary(b *strings.Builder, bd render.BoundaryBox) {
	b.WriteString(`<g class="vd-boundary vd-b-` + safeKind(bd.Kind) + `">` + "\n")
	b.WriteString(`<rect ` + rectAttrs(bd.Rect) + ` rx="14"/>` + "\n")
	textEl(b, bd.Rect.X+12, bd.Rect.Y+16, bd.LabelFont, "vd-boundary-label", "start", bd.Label)
	b.WriteString("</g>\n")
}

// safeKind は kind (schema enum 検証済み) を防御的に再確認する。
func safeKind(kind string) string {
	for _, r := range kind {
		if (r < 'a' || r > 'z') && r != '-' {
			return "region"
		}
	}
	return kind
}

func writeEdge(b *strings.Builder, e render.Edge) {
	idAttr := ""
	if safe.IsValidID(e.ID) {
		idAttr = ` id="vd-edge-` + e.ID + `"`
	}
	b.WriteString(`<g class="vd-edge-group"` + idAttr + ">\n")
	var d strings.Builder
	for i, p := range e.Points {
		if i == 0 {
			d.WriteString("M " + shared.Coord(p.X) + " " + shared.Coord(p.Y))
		} else {
			d.WriteString(" L " + shared.Coord(p.X) + " " + shared.Coord(p.Y))
		}
	}
	b.WriteString(`<path class="vd-edge vd-v-` + safeVariant(e.Variant) + `" d="` + d.String() +
		`" marker-end="url(#vd-arrow-` + safeVariant(e.Variant) + `)"/>` + "\n")
	if e.HasLabel {
		b.WriteString(`<rect class="vd-edge-label-mask" ` + rectAttrs(e.LabelRect) + ` rx="4"/>` + "\n")
		c := e.LabelRect.Center()
		textEl(b, c.X, c.Y+e.LabelFont*0.35, e.LabelFont, "vd-edge-label", "middle", e.Label)
	}
	b.WriteString("</g>\n")
}

func safeVariant(v string) string {
	switch v {
	case "default", "emphasis", "security", "dashed":
		return v
	}
	return "default"
}

func safeType(t string) string {
	if _, ok := typeLabels[t]; ok {
		return t
	}
	return "external"
}

// typeGlyph はノード種別を表す 1 文字バッジ (色に依存しない識別符号)。
// 色覚差やグレースケール印刷でも種別を判別できるようにする。
var typeGlyph = map[string]string{
	"frontend": "F", "backend": "B", "database": "D", "cloud": "C",
	"security": "S", "messagebus": "M", "external": "E",
}

func writeNode(b *strings.Builder, n render.Node) {
	idAttr := ""
	if safe.IsValidID(n.ID) {
		idAttr = ` id="vd-node-` + n.ID + `"`
	}
	t := safeType(n.Type)
	// 支援技術向けに種別とラベルを aria-label で伝える (label はエスケープ済み)。
	aria := typeLabels[t] + ": " + n.Label
	b.WriteString(`<g class="vd-node vd-t-` + t + `"` + idAttr +
		` role="group" aria-label="` + safe.HTML(aria) + `"` +
		` data-label="` + safe.HTML(n.Label) + `">` + "\n")
	b.WriteString(`<rect class="vd-node-box" ` + rectAttrs(n.Rect) + ` rx="10"/>` + "\n")
	c := n.Rect.Center()
	if n.Sublabel != "" {
		textEl(b, c.X, c.Y-3, n.LabelFont, "vd-node-label", "middle", n.Label)
		textEl(b, c.X, c.Y+n.SublabelFont+3, n.SublabelFont, "vd-node-sub", "middle", n.Sublabel)
	} else {
		textEl(b, c.X, c.Y+n.LabelFont*0.35, n.LabelFont, "vd-node-label", "middle", n.Label)
	}
	// 種別グリフバッジ (左上角)。色以外の識別符号。
	bx, by := shared.Round2(n.Rect.X+12), shared.Round2(n.Rect.Y+12)
	b.WriteString(`<circle class="vd-node-glyph-bg" cx="` + shared.Coord(bx) + `" cy="` + shared.Coord(by) + `" r="8"/>` + "\n")
	textEl(b, bx, by+3, 9, "vd-node-glyph", "middle", typeGlyph[t])
	if n.HasTag {
		b.WriteString(`<rect class="vd-node-tag-pill" ` + rectAttrs(n.TagRect) + ` rx="8"/>` + "\n")
		tc := n.TagRect.Center()
		textEl(b, tc.X, tc.Y+n.TagFont*0.35, n.TagFont, "vd-node-tag", "middle", n.Tag)
	}
	b.WriteString("</g>\n")
}

// svgDescription は図の概要を数値のみで記述する (支援技術向け。IR 由来文字列は含めない)。
func svgDescription(l *render.Layout) string {
	counts := map[string]int{}
	for _, n := range l.Nodes {
		counts[safeType(n.Type)]++
	}
	var parts []string
	for _, t := range typeOrder {
		if counts[t] > 0 {
			parts = append(parts, strconv.Itoa(counts[t])+" "+typeLabels[t])
		}
	}
	desc := strconv.Itoa(len(l.Nodes)) + " components (" + strings.Join(parts, ", ") + "), " +
		strconv.Itoa(len(l.Edges)) + " connections"
	if len(l.Boundaries) > 0 {
		desc += ", " + strconv.Itoa(len(l.Boundaries)) + " boundaries"
	}
	return desc + "."
}

func writeLegend(b *strings.Builder, l *render.Layout) {
	if len(l.Legend) == 0 {
		return
	}
	b.WriteString(`<g class="vd-legend" role="list" aria-label="Legend">` + "\n")
	for _, en := range l.Legend {
		x, y := en.X, en.Y
		b.WriteString(`<g role="listitem" aria-label="` + safe.HTML(en.Label) + `">` + "\n")
		if en.IsEdge {
			b.WriteString(`<line class="vd-legend-line vd-v-` + safeVariant(en.Kind) +
				`" x1="` + shared.Coord(x) + `" y1="` + shared.Coord(y+10) +
				`" x2="` + shared.Coord(x+14) + `" y2="` + shared.Coord(y+10) + `"/>` + "\n")
		} else {
			t := safeType(en.Kind)
			b.WriteString(`<rect class="vd-legend-swatch vd-t-` + t +
				`" x="` + shared.Coord(x) + `" y="` + shared.Coord(y+4) + `" width="14" height="14" rx="3"/>` + "\n")
			// 種別グリフをスウォッチに重ねる (色以外の識別符号)。
			textEl(b, x+7, y+15, 9, "vd-legend-glyph", "middle", typeGlyph[t])
		}
		textEl(b, x+18, y+14, legendFont, "vd-legend-label", "start", en.Label)
		b.WriteString("</g>\n")
	}
	b.WriteString("</g>\n")
}
