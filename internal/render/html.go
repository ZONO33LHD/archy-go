// html.go は単一 HTML ファイルの組み立てを行う (§8, §10.3)。
//
//   - 生成 HTML は完全にオフラインで完結する (外部リソース参照ゼロ)
//   - CSP のスクリプト/スタイルハッシュは「実際に挿入するバイト列」から計算するため、
//     テンプレートとハッシュがずれることは構造的に起きない
//   - 埋め込み JSON は < / U+2028 / U+2029 をエスケープしてスクリプトブレイクアウトを防ぐ
//   - プレースホルダ置換は strings.Replacer の単一パスで行い、
//     ユーザー文字列に含まれるプレースホルダ様のトークンが再走査されないようにする
package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/ZONO33LHD/archy-go/assets"
	"github.com/ZONO33LHD/archy-go/internal/i18n"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/safe"
)

// AssembleHTML は SVG・IR・ビューアから単一 HTML を組み立てる。
func AssembleHTML(doc *ir.Document, svg string) (string, error) {
	tmpl := string(assets.MustRead("template.html"))
	css := string(assets.MustRead("viewer.css"))
	js := string(assets.MustRead("viewer.js"))

	cat := i18n.For(doc.Locale())

	dataJSON, err := marshalEmbeddedJSON(doc)
	if err != nil {
		return "", fmt.Errorf("render/data-json: 埋め込み IR の生成に失敗しました: %w", err)
	}

	// テンプレート整合性は「置換前の tmpl」に対して検査する。置換後の成果物を検査すると、
	// 正当な IR 文字列 (タイトルやラベル、埋め込み JSON) に "{{VD_..." が含まれるだけで
	// 誤って内部エラーになってしまう (ユーザーデータをテンプレート制御と誤認しない)。
	if missing := unresolvedPlaceholders(tmpl); len(missing) > 0 {
		return "", fmt.Errorf("render/template: テンプレートに未定義のプレースホルダがあります: %s", strings.Join(missing, " "))
	}

	replacer := strings.NewReplacer(
		"{{VD_LANG}}", cat.Lang,
		"{{VD_SCRIPT_HASH}}", cspHash(js),
		"{{VD_STYLE_HASH}}", cspHash(css),
		"{{VD_TITLE}}", safe.HTML(doc.Meta.Title),
		"{{VD_CSS}}", css,
		"{{VD_JS}}", js,
		"{{VD_SVG}}", svg,
		"{{VD_CARDS}}", cardsHTML(doc, cat),
		"{{VD_DATA}}", dataJSON,
		"{{VD_T_SEARCH}}", safe.HTML(cat.SearchPlaceholder),
		"{{VD_T_RESET}}", safe.HTML(cat.ResetButton),
		"{{VD_T_THEME}}", safe.HTML(cat.ThemeButton),
		"{{VD_T_ZOOM}}", safe.HTML(cat.ZoomGroup),
		"{{VD_T_ZOOMIN}}", safe.HTML(cat.ZoomIn),
		"{{VD_T_ZOOMOUT}}", safe.HTML(cat.ZoomOut),
		"{{VD_T_STAGE}}", safe.HTML(cat.Stage),
	)
	// 出力の改行は常に LF (テンプレートは LF で管理される)。
	return replacer.Replace(tmpl), nil
}

// knownPlaceholders はレンダラが置換する全プレースホルダ。
var KnownPlaceholders = []string{
	"{{VD_LANG}}", "{{VD_SCRIPT_HASH}}", "{{VD_STYLE_HASH}}", "{{VD_TITLE}}",
	"{{VD_CSS}}", "{{VD_JS}}", "{{VD_SVG}}", "{{VD_CARDS}}", "{{VD_DATA}}",
	"{{VD_T_SEARCH}}", "{{VD_T_RESET}}", "{{VD_T_THEME}}",
	"{{VD_T_ZOOM}}", "{{VD_T_ZOOMIN}}", "{{VD_T_ZOOMOUT}}", "{{VD_T_STAGE}}",
}

// unresolvedPlaceholders はテンプレート内の "{{VD_...}}" のうち、レンダラが置換しないものを返す。
// 埋め込みアセットは build 時に確定するため、通常これは空 (アセット改変時のみ非空になる)。
func unresolvedPlaceholders(tmpl string) []string {
	var missing []string
	rest := tmpl
	for {
		i := strings.Index(rest, "{{VD_")
		if i < 0 {
			break
		}
		rest = rest[i:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			missing = append(missing, rest)
			break
		}
		ph := rest[:end+2]
		if !slices.Contains(KnownPlaceholders, ph) {
			missing = append(missing, ph)
		}
		rest = rest[end+2:]
	}
	return missing
}

// cspHash は CSP source list 用の 'sha256-...' 値を、挿入する内容そのものから計算する。
func cspHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

// marshalEmbeddedJSON は IR を <script type="application/json"> に埋め込める形にする。
// v2 の Deterministic でキー順を固定し、スクリプトブレイクアウト文字をエスケープする。
func marshalEmbeddedJSON(doc *ir.Document) (string, error) {
	b, err := json.Marshal(doc, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	b = bytes.ReplaceAll(b, []byte("<"), []byte("\\u003c"))
	b = bytes.ReplaceAll(b, []byte("\u2028"), []byte("\\u2028"))
	b = bytes.ReplaceAll(b, []byte("\u2029"), []byte("\\u2029"))
	return string(b), nil
}

// cardsHTML は情報カードを HTML として組み立てる。IR 由来の文字列は必ず safe.HTML を通す。
func cardsHTML(doc *ir.Document, cat i18n.Catalog) string {
	if len(doc.Cards) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="vd-cards">` + "\n")
	b.WriteString("<h2>" + safe.HTML(cat.NotesHeading) + "</h2>\n")
	for _, c := range doc.Cards {
		dot := c.Dot
		switch dot {
		case "cyan", "blue", "green", "amber", "red", "purple", "gray":
		default:
			dot = "gray"
		}
		b.WriteString("<article>\n")
		b.WriteString(`<h3><span class="vd-dot vd-dot-` + dot + `"></span>` + safe.HTML(c.Title) + "</h3>\n")
		b.WriteString("<ul>\n")
		for _, item := range c.Items {
			b.WriteString("<li>" + safe.HTML(item) + "</li>\n")
		}
		b.WriteString("</ul>\n</article>\n")
	}
	b.WriteString("</section>\n")
	return b.String()
}
