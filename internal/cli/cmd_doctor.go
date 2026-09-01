// cmd_doctor.go は doctor コマンド (自己診断) を実装する。
package cli

import (
	json "encoding/json/v2"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/ZONO33LHD/archy-go/assets"
	"github.com/ZONO33LHD/archy-go/internal/render/shared"
)

// WidthTableHashExpected は凍結済み文字幅レンジテーブルの期待 SHA-256。
// テーブルを意図的に更新した場合のみ、この値とゴールデンファイルを併せて更新する。
const WidthTableHashExpected = "43c5ab8b026a1aac4bce312356a70e7f383d969e9a4fb5baed3b9ea449dbc6f7"

// forbiddenViewerTokens は生成 HTML / ビューア JS に決して含めないトークン (§10.3)。
var forbiddenViewerTokens = []string{
	"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write",
	"eval(", "new Function", "<foreignObject", "javascript:",
}

type doctorCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Note string `json:"note,omitempty"`
}

func (c *CLI) cmdDoctor(args []string) int {
	f, pos, err := splitArgs(args)
	if err != nil || len(pos) != 0 {
		fmt.Fprintln(c.Stderr, "使い方: veduta doctor [--json]")
		return ExitUsage
	}

	var checks []doctorCheck
	add := func(name string, ok bool, note string) {
		checks = append(checks, doctorCheck{Name: name, OK: ok, Note: note})
	}

	// 埋め込みアセットの健全性。
	tmpl := string(assets.MustRead("template.html"))
	missing := []string{}
	for _, ph := range []string{"{{VD_LANG}}", "{{VD_SCRIPT_HASH}}", "{{VD_STYLE_HASH}}",
		"{{VD_TITLE}}", "{{VD_CSS}}", "{{VD_JS}}", "{{VD_SVG}}", "{{VD_CARDS}}", "{{VD_DATA}}"} {
		if !strings.Contains(tmpl, ph) {
			missing = append(missing, ph)
		}
	}
	add("template-placeholders", len(missing) == 0, strings.Join(missing, " "))

	js := string(assets.MustRead("viewer.js"))
	var badTokens []string
	for _, tok := range forbiddenViewerTokens {
		if strings.Contains(js, tok) || strings.Contains(tmpl, tok) {
			badTokens = append(badTokens, tok)
		}
	}
	add("viewer-forbidden-tokens", len(badTokens) == 0, strings.Join(badTokens, " "))

	var schemaDoc any
	schemaErr := json.Unmarshal(assets.MustRead("schemas/architecture.schema.json"), &schemaDoc)
	add("schema-parses", schemaErr == nil, errNote(schemaErr))

	var exampleDoc any
	exampleErr := json.Unmarshal(assets.MustRead("examples/architecture-web-app.json"), &exampleDoc)
	add("example-parses", exampleErr == nil, errNote(exampleErr))

	// 文字幅テーブルの凍結検証 (§6)。
	gotHash := shared.WidthTableHash()
	add("widthtable-hash", gotHash == WidthTableHashExpected, "sha256:"+gotHash[:12])

	// cwd 封じ込め (os.Root) が機能するか。
	root, rootErr := os.OpenRoot(c.Cwd)
	if rootErr == nil {
		_ = root.Close()
	}
	add("os-root", rootErr == nil, errNote(rootErr))

	add("go-toolchain", strings.HasPrefix(runtime.Version(), "go1.27"), runtime.Version())

	allOK := true
	for _, ch := range checks {
		if !ch.OK {
			allOK = false
		}
	}

	if f.json {
		_ = writeJSON(c.Stdout, map[string]any{"ok": allOK, "checks": checks, "version": c.Version})
	} else {
		for _, ch := range checks {
			mark := "ok"
			if !ch.OK {
				mark = "NG"
			}
			if ch.Note != "" {
				fmt.Fprintf(c.Stdout, "%-28s %s  (%s)\n", ch.Name, mark, ch.Note)
			} else {
				fmt.Fprintf(c.Stdout, "%-28s %s\n", ch.Name, mark)
			}
		}
	}
	if allOK {
		return ExitOK
	}
	return ExitFailed
}

func errNote(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
