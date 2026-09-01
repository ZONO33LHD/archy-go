// cmd_render.go は render コマンドを実装する。
// render はエージェントの修復ループで繰り返し呼ばれるため、既定ではブラウザを開かない (§11.1)。
package cli

import (
	"fmt"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/outpath"
	"github.com/ZONO33LHD/archy-go/internal/safe"
	"github.com/ZONO33LHD/archy-go/internal/validate"
)

// renderReport は render --json の出力形。
type renderReport struct {
	OK          bool                   `json:"ok"`
	Profile     string                 `json:"profile"`
	Output      string                 `json:"output,omitempty"`
	HTMLSHA256  string                 `json:"html_sha256,omitempty"`
	HTMLBytes   int64                  `json:"html_bytes,omitempty"`
	Errors      int                    `json:"errors"`
	Warnings    int                    `json:"warnings"`
	Diagnostics diag.List              `json:"diagnostics"`
	Checks      []validate.CheckResult `json:"checks,omitempty"`
	Opened      bool                   `json:"opened"`
	OpenReason  string                 `json:"open_reason,omitempty"`
}

func (c *CLI) cmdRender(args []string) int {
	return c.renderLike(args, false)
}

// renderLike は render / deliver 共通の入口 (deliver は snapshot 経由で cmd_deliver.go)。
func (c *CLI) renderLike(args []string, _ bool) int {
	f, pos, err := splitArgs(args)
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta render: %v\n", err)
		return ExitUsage
	}
	if len(pos) != 3 {
		fmt.Fprintln(c.Stderr, "使い方: veduta render <type> <input.json> <output.html> [--json] [--quality ...] [--open|--no-open]")
		return ExitUsage
	}
	diagramType, inputPath, outPath := pos[0], pos[1], pos[2]

	// 出力パスは書き込み前に検査して早期に落とす。
	if err := outpath.Validate(outPath); err != nil {
		fmt.Fprintf(c.Stderr, "veduta render: 出力パスが不正です: %v\n", err)
		return ExitUsage
	}

	data, code := c.readInput(inputPath, f.json)
	if code != ExitOK {
		return code
	}
	res := build(diagramType, data, f.quality, false)
	if !res.OK {
		return c.reportValidation(res, f.json)
	}

	wr, err := outpath.WriteAtomic(c.Cwd, outPath, []byte(res.HTML))
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta render: 出力に失敗しました: %v\n", err)
		return ExitFailed
	}

	opened, openReason := c.maybeOpen(false, f, wr.AbsPath, wr.Bytes, sha256Hex([]byte(res.HTML)))

	errs, warns := res.Diags.Count()
	if f.json {
		_ = writeJSON(c.Stdout, renderReport{
			OK: true, Profile: res.Profile, Output: wr.AbsPath,
			HTMLSHA256: sha256Hex([]byte(res.HTML)), HTMLBytes: wr.Bytes,
			Errors: errs, Warnings: warns,
			Diagnostics: orEmpty(res.Diags), Checks: res.Checks,
			Opened: opened, OpenReason: openReason,
		})
		return ExitOK
	}
	printDiags(c.Stdout, res.Diags)
	// 自動オープンの有無にかかわらず、成果物の絶対パスは必ず表示する (§11.6)。
	fmt.Fprintln(c.Stdout, wr.AbsPath)
	fmt.Fprintf(c.Stdout, "  html   sha256:%s  %s\n", shortHash(sha256Hex([]byte(res.HTML))), formatBytes(wr.Bytes))
	ran, issues := checkSummary(res.Checks)
	fmt.Fprintf(c.Stdout, "  checks %d/%d passed\n", ran-issues, ran)
	c.printOpenStatus(opened, openReason, f.open)
	return ExitOK
}

func orEmpty(ds diag.List) diag.List {
	if ds == nil {
		return diag.List{}
	}
	return ds
}

// maybeOpen は §11 の判定に従ってブラウザを開く。失敗は終了コードに影響しない (§11.5)。
// wantSHA256 は書き込んだ内容の SHA-256 で、開く直前のすり替え検査に使う。
func (c *CLI) maybeOpen(cmdDefault bool, f cmdFlags, absPath string, wantBytes int64, wantSHA256 string) (bool, string) {
	dec := c.decide(cmdDefault, f)
	if !dec.Open {
		return false, dec.Reason
	}
	target, err := c.verifyAndTarget(absPath, wantBytes, wantSHA256)
	if err != nil {
		return false, err.Error()
	}
	if err := c.Launcher.Open(target); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (c *CLI) decide(cmdDefault bool, f cmdFlags) decision {
	d := decideOpen(cmdDefault, f, c)
	return d
}

// printOpenStatus は人間可読出力にオープン状態を一行添える。
func (c *CLI) printOpenStatus(opened bool, reason string, explicitOpen bool) {
	switch {
	case opened:
		fmt.Fprintln(c.Stdout, "  opened in browser")
	case reason == "":
	case explicitOpen:
		// --open 明示時のみ失敗を警告として目立たせてよい。それでも終了コードは変えない。
		fmt.Fprintf(c.Stdout, "  警告: ブラウザを開けませんでした (%s)\n", safe.ForTerminal(reason))
	default:
		fmt.Fprintf(c.Stdout, "  (ブラウザは開きません: %s)\n", safe.ForTerminal(reason))
	}
}
