// Package safe はエスケープ・サニタイズ・文字列検査の集約点である。
//
// IR 由来の文字列がレンダリング・診断・ターミナルへ流れる前に必ずこのパッケージを通す。
// エスケープを各レンダラで個別に書くことは禁止 (分散すると必ず漏れる)。
package safe

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// idPattern は SVG の id 属性・CSS セレクタ・URL フラグメントに流せる安全な ID の形。
// ここを絞ることでセレクタ注入・CSS 注入を根本から断つ。
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// IsValidID は component.id / connection.id / view.id として許可される形かを返す。
func IsValidID(s string) bool {
	return idPattern.MatchString(s)
}

// HTML は & < > " ' の 5 文字のみを実体参照に変換する明示エスケープ関数。
// html/template の自動エスケープには任せない (SVG 属性内で二重エスケープが起きるため)。
// 出力 HTML/SVG に IR 由来の文字列を埋める箇所は必ずこの関数を通す。
func HTML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Violation は文字列検査で見つかった禁止文字。
type Violation struct {
	// Code は診断コード (sanitize/...)。
	Code string
	// Rune は問題の文字の U+XXXX 表記 (安全に表示できる)。
	Rune string
	// Index は文字列先頭からのコードポイント位置。
	Index int
}

// CheckText は本文系文字列 (label / title / note など) の禁止文字を検査する。
// 拒否する文字 (§10.2): C0/C1 制御文字 (\t \n を除く)、U+007F、双方向制御文字、
// ゼロ幅文字、U+0000、不正なサロゲート・UTF-8。
// 黙って除去せず、検証エラーとして返すための材料を列挙する。
func CheckText(s string) []Violation {
	var vs []Violation
	idx := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			vs = append(vs, Violation{Code: "sanitize/invalid-utf8", Rune: "U+FFFD", Index: idx})
			i += size
			idx++
			continue
		}
		if code := forbiddenRuneCode(r); code != "" {
			vs = append(vs, Violation{Code: code, Rune: fmt.Sprintf("U+%04X", r), Index: idx})
		}
		i += size
		idx++
	}
	return vs
}

// forbiddenRuneCode は禁止文字なら診断コードを、許可文字なら空文字を返す。
//
// bidi 制御文字とゼロ幅文字の範囲はこのツールが凍結するレイアウト規則であり、
// Unicode バージョン更新への追随対象ではない (§6 と同じ理由)。範囲を明示列挙する。
func forbiddenRuneCode(r rune) string {
	switch {
	case r == 0x0000:
		return "sanitize/control-char"
	case r == '\t' || r == '\n':
		return "" // 本文中のタブ・改行のみ許可
	case r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F):
		return "sanitize/control-char"
	case r == 0x061C || // ARABIC LETTER MARK
		r == 0x200E || r == 0x200F || // LRM / RLM
		(r >= 0x202A && r <= 0x202E) || // LRE RLE PDF LRO RLO
		(r >= 0x2066 && r <= 0x2069): // LRI RLI FSI PDI
		// Trojan Source 攻撃: 図の見た目と実際の文字列を食い違わせられる
		return "sanitize/bidi-control"
	case (r >= 0x200B && r <= 0x200D) || // ZWSP ZWNJ ZWJ
		r == 0x2060 || // WORD JOINER
		(r >= 0x2061 && r <= 0x2064) || // 不可視の数学演算子
		(r >= 0x206A && r <= 0x206F) || // 非推奨のフォーマット制御
		r == 0xFEFF: // ZERO WIDTH NO-BREAK SPACE (BOM)
		return "sanitize/zero-width"
	case r == 0x2028 || r == 0x2029:
		// 行区切り・段落区切り。SVG テキストや JS 文字列文脈で行制御になる
		return "sanitize/line-separator"
	case r >= 0xD800 && r <= 0xDFFF:
		return "sanitize/invalid-utf8"
	default:
		return ""
	}
}

// CheckSingleLine は単一行フィールド (label / sublabel / tag / title / boundary label /
// edge label) の検査。CheckText に加えてタブ・改行も拒否する。
//
// SVG テキストは折り返されず、改行は行ごとの tspan として無制限に縦へ積まれるため、
// 単一行フィールドに \n を許すとボックスやラベルマスクを縦にはみ出せてしまう (SVG 破壊)。
// note やカード項目など複数行を許すフィールドは CheckText を使う。
func CheckSingleLine(s string) []Violation {
	vs := CheckText(s)
	idx := 0
	for _, r := range s {
		if r == '\n' || r == '\t' {
			vs = append(vs, Violation{Code: "sanitize/multiline", Rune: fmt.Sprintf("U+%04X", r), Index: idx})
		}
		idx++
	}
	return vs
}

// ForTerminal は診断・ログに IR 由来の文字列を出す際の無害化。
// ANSI エスケープ (\x1b[...) を含む非表示文字を可視表現 (\xNN / \uNNNN) に置換する。
func ForTerminal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			b.WriteString(`\xFFFD`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F):
			fmt.Fprintf(&b, `\x%02x`, r)
		case forbiddenRuneCode(r) != "":
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// maxDiagQuote は診断メッセージに埋め込む文字列の最大コードポイント数。
const maxDiagQuote = 48

// ForDiag は診断メッセージに埋め込むための引用形。
// 長さを切り詰め、制御文字を可視化し、引用符で囲む。
func ForDiag(s string) string {
	return "'" + ForTerminal(Clip(s, maxDiagQuote)) + "'"
}

// Clip は文字列を最大 max コードポイントに切り詰める。切り詰めた場合は … を付す。
func Clip(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "…"
}
