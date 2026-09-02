package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// -update フラグでゴールデンファイルを再生成する。
// 期待値の差分レビューが、レンダラ変更時の唯一の防波堤になる (§13)。
var update = flag.Bool("update", false, "ゴールデンファイルを再生成する")

func goldenInputs(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("testdata/architecture/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("testdata が見つかりません: %v", err)
	}
	return paths
}

func TestGolden(t *testing.T) {
	for _, input := range goldenInputs(t) {
		t.Run(filepath.Base(input), func(t *testing.T) {
			data, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			res := build("architecture", data, "", false)
			if !res.OK {
				t.Fatalf("ゴールデン入力が検証を通りません: %+v", res.Diags)
			}
			goldenPath := strings.TrimSuffix(input, ".json") + ".golden.html"
			if *update {
				if err := os.WriteFile(goldenPath, []byte(res.HTML), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("ゴールデンファイルがありません (-update で生成): %v", err)
			}
			if string(want) != res.HTML {
				t.Errorf("ゴールデンファイルと不一致です。差分を確認し、意図した変更なら -update で更新してください。\nwant sha=%s got sha=%s",
					hashOf(want), hashOf([]byte(res.HTML)))
			}
		})
	}
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// TestDeterminism100: 同じ入力を 100 回レンダリングして全出力の SHA-256 が一致する (§13)。
// マップ順序のリークを確実に捕まえる。
func TestDeterminism100(t *testing.T) {
	data, err := os.ReadFile("testdata/architecture/basic.json")
	if err != nil {
		t.Fatal(err)
	}
	first := build("architecture", data, "", false)
	if !first.OK {
		t.Fatalf("入力が検証を通りません: %+v", first.Diags)
	}
	firstHash := hashOf([]byte(first.HTML))
	for i := range 100 {
		res := build("architecture", data, "", false)
		if !res.OK || hashOf([]byte(res.HTML)) != firstHash {
			t.Fatalf("%d 回目のレンダリングで出力が変わりました", i+1)
		}
	}
}

// buildDocJSON はテスト用の IR を組み立てる (label などを差し替え可能)。
func buildDocJSON(label string) string {
	// label は JSON 文字列として埋め込むため引用符などを最小エスケープする。
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(label)
	return `{
  "schema_version": 1,
  "diagram_type": "architecture",
  "meta": { "title": "XSS Test" },
  "components": [
    { "id": "a", "type": "backend", "label": "` + esc + `", "pos": [0, 0], "size": [420, 60] },
    { "id": "b", "type": "database", "label": "B", "pos": [560, 0], "size": [120, 60] }
  ],
  "connections": [ { "id": "ab", "from": "a", "to": "b" } ]
}`
}

// TestHTMLSecurityXSS は §13 の XSS / 注入回帰テスト。
func TestHTMLSecurityXSS(t *testing.T) {
	t.Run("script要素は増えない", func(t *testing.T) {
		payload := `</script><script>alert(1)</script>`
		res := build("architecture", []byte(buildDocJSON(payload)), "", false)
		if !res.OK {
			t.Fatalf("検証を通りません: %+v", res.Diags)
		}
		// 生成 HTML の <script 出現数は 2 (データブロック + ビューア) のまま。
		if got := strings.Count(res.HTML, "<script"); got != 2 {
			t.Errorf("<script 要素数 = %d, want 2", got)
		}
		// 埋め込み JSON 内では \u003c にエスケープされる。
		if !strings.Contains(res.HTML, `\u003c/script`) {
			t.Error(`埋め込み JSON 内の </script が \u003c エスケープされていません`)
		}
		if strings.Contains(res.HTML, "<script>alert(1)") {
			t.Error("ペイロードが生の <script> として出力されています")
		}
	})

	t.Run("imgタグはエスケープ", func(t *testing.T) {
		res := build("architecture", []byte(buildDocJSON(`<img src=x onerror=alert(1)>`)), "", false)
		if !res.OK {
			t.Fatalf("検証を通りません: %+v", res.Diags)
		}
		if strings.Contains(res.HTML, "<img") {
			t.Error("img タグが生のまま出力されています")
		}
	})

	t.Run("属性を脱出しない", func(t *testing.T) {
		res := build("architecture", []byte(buildDocJSON(`" onload="alert(1)`)), "", false)
		if !res.OK {
			t.Fatalf("検証を通りません: %+v", res.Diags)
		}
		if regexp.MustCompile(`\son[a-z]+="`).MatchString(res.HTML) {
			t.Error("on* 属性が生成されています (属性脱出)")
		}
	})

	t.Run("双方向制御文字は検証エラー", func(t *testing.T) {
		res := build("architecture", []byte(buildDocJSON("evil\u202Etxt.js")), "", false)
		if res.OK {
			t.Fatal("U+202E を含む label が受理されました")
		}
		if !hasDiagCode(res, "sanitize/bidi-control") {
			t.Errorf("sanitize/bidi-control が返りません: %v", diagCodes(res))
		}
	})

	t.Run("ANSIエスケープは検証エラーかつ診断で無害化", func(t *testing.T) {
		res := build("architecture", []byte(buildDocJSON("x\x1b[31mred")), "", false)
		if res.OK {
			t.Fatal("ESC を含む label が受理されました")
		}
		var sb strings.Builder
		printDiags(&sb, res.Diags)
		if strings.ContainsRune(sb.String(), 0x1b) {
			t.Error("診断出力に生の ESC が含まれています")
		}
	})

	t.Run("不正なidは検証エラー", func(t *testing.T) {
		for _, id := range []string{`a" onclick="x`, `../x`, `a b`} {
			doc := strings.Replace(buildDocJSON("ok"), `"id": "a"`, `"id": "`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(id)+`"`, 1)
			res := build("architecture", []byte(doc), "", false)
			if res.OK {
				t.Errorf("不正な id %q が受理されました", id)
			}
		}
	})

	t.Run("localeの注入は検証エラー", func(t *testing.T) {
		doc := strings.Replace(buildDocJSON("ok"), `"title": "XSS Test"`, `"title": "XSS Test", "locale": "en\" onload=\"x"`, 1)
		res := build("architecture", []byte(doc), "", false)
		if res.OK {
			t.Fatal("locale の注入が受理されました")
		}
		if !hasDiagCode(res, "schema/enum") {
			t.Errorf("schema/enum が返りません: %v", diagCodes(res))
		}
	})
}

// TestHTMLSecurityInvariants は生成 HTML の静的不変条件 (§13)。
func TestHTMLSecurityInvariants(t *testing.T) {
	data, err := os.ReadFile("testdata/architecture/basic.json")
	if err != nil {
		t.Fatal(err)
	}
	res := build("architecture", data, "", false)
	if !res.OK {
		t.Fatal("入力が検証を通りません")
	}
	html := res.HTML

	for _, tok := range []string{
		"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write",
		"eval(", "new Function", "<foreignObject", "javascript:", "<base", "http-equiv=\"refresh\"",
	} {
		if strings.Contains(html, tok) {
			t.Errorf("生成 HTML に禁止トークン %q が含まれています", tok)
		}
	}
	if regexp.MustCompile(`\son[a-z]+=`).MatchString(html) {
		t.Error("生成 HTML に on* 属性が含まれています")
	}

	// 外部 URL 参照ゼロ (SVG 名前空間のみ例外 §8)。
	urlRe := regexp.MustCompile(`(https?:)?//[^\s"'<>]+`)
	for _, m := range urlRe.FindAllString(html, -1) {
		if m == "//www.w3.org/2000/svg" || strings.HasPrefix(m, "http://www.w3.org/2000/svg") {
			continue
		}
		t.Errorf("外部 URL 参照が含まれています: %s", m)
	}

	// CSP のスクリプトハッシュが実際のインラインスクリプトと一致する。
	verifyCSPHash(t, html, `script-src 'sha256-`, "<script>", "</script>")
	verifyCSPHash(t, html, `style-src 'sha256-`, "<style>", "</style>")
}

func verifyCSPHash(t *testing.T, html, marker, open, close string) {
	t.Helper()
	mi := strings.Index(html, marker)
	if mi < 0 {
		t.Errorf("CSP に %s がありません", marker)
		return
	}
	rest := html[mi+len(marker):]
	declared := rest[:strings.Index(rest, "'")]

	oi := strings.Index(html, open)
	ci := strings.Index(html[oi:], close)
	content := html[oi+len(open) : oi+ci]
	sum := sha256.Sum256([]byte(content))
	actual := base64.StdEncoding.EncodeToString(sum[:])
	if declared != actual {
		t.Errorf("CSP ハッシュ不一致: 宣言=%s 実際=%s", declared, actual)
	}
}

// TestPropertyRender はプロパティテスト (§13):
//   - validate が合格させた IR は必ず render も成功する
//   - SVG は整形式の XML としてパースできる
//   - SVG のテキストノードに IR 由来でない文字列が現れない (凡例語彙を除く)
func TestPropertyRender(t *testing.T) {
	for _, input := range goldenInputs(t) {
		data, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		res := build("architecture", data, "", false)
		if !res.OK {
			continue
		}
		if res.HTML == "" {
			t.Fatalf("%s: validate 合格なのに render 出力が空です", input)
		}

		svg := extractSVG(t, res.HTML)
		texts := svgTexts(t, svg)

		// IR 由来の全文字列 + レンダラの凡例語彙。
		allowed := strings.Join([]string{
			string(data),
			"Frontend", "Backend", "Database", "Cloud", "Security", "Message Bus", "External",
			"Default link", "Emphasis link", "Security link", "Dashed link",
		}, "\n")
		for _, txt := range texts {
			trimmed := strings.TrimSpace(txt)
			if trimmed == "" {
				continue
			}
			if !strings.Contains(allowed, trimmed) {
				t.Errorf("%s: IR に無いテキストノードが出力されています: %q", input, trimmed)
			}
		}
	}
}

func extractSVG(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, "<svg")
	end := strings.Index(html, "</svg>")
	if start < 0 || end < 0 {
		t.Fatal("SVG が見つかりません")
	}
	return html[start : end+len("</svg>")]
}

// svgTexts は SVG を XML としてパースし、テキストノードを収集する。
// パースできなければ整形式でないとして失敗させる。
func svgTexts(t *testing.T, svg string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	var texts []string
	skip := 0 // <title>/<desc> は意図的な説明メタデータなので本文チェックから除外する
	for {
		tok, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("SVG が整形式の XML ではありません: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "title" || el.Name.Local == "desc" {
				skip++
			}
		case xml.EndElement:
			if el.Name.Local == "title" || el.Name.Local == "desc" {
				if skip > 0 {
					skip--
				}
			}
		case xml.CharData:
			if skip == 0 {
				texts = append(texts, string(el))
			}
		}
	}
	return texts
}

func hasDiagCode(res *buildResult, code string) bool {
	for _, d := range res.Diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func diagCodes(res *buildResult) []string {
	var cs []string
	for _, d := range res.Diags {
		cs = append(cs, d.Code)
	}
	return cs
}
