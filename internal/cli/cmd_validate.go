// cmd_validate.go は validate コマンド (スキーマ検証 + 構図検証) を実装する。
package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/validate"
)

// validateReport は validate --json の出力形。
type validateReport struct {
	OK          bool                   `json:"ok"`
	Profile     string                 `json:"profile"`
	Errors      int                    `json:"errors"`
	Warnings    int                    `json:"warnings"`
	Diagnostics diag.List              `json:"diagnostics"`
	Checks      []validate.CheckResult `json:"checks,omitempty"`
}

func (c *CLI) cmdValidate(args []string) int {
	f, pos, err := splitArgs(args)
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta validate: %v\n", err)
		return ExitUsage
	}
	if len(pos) != 2 {
		fmt.Fprintln(c.Stderr, "使い方: veduta validate <type> <input.json> [--json] [--quality ...]")
		return ExitUsage
	}
	diagramType, inputPath := pos[0], pos[1]

	data, code := c.readInput(inputPath, f.json)
	if code != ExitOK {
		return code
	}
	res := build(diagramType, data, f.quality, false)
	return c.reportValidation(res, f.json)
}

// readInput は入力ファイルをサイズ上限つきで読む。
func (c *CLI) readInput(path string, jsonOut bool) ([]byte, int) {
	fh, err := os.Open(path)
	if err != nil {
		c.reportReadError(err, jsonOut)
		return nil, ExitFailed
	}
	defer fh.Close()
	data, ds := ir.ReadLimited(fh)
	if len(ds) > 0 {
		if jsonOut {
			_ = writeJSON(c.Stdout, validateReport{Profile: "standard", Errors: len(ds), Diagnostics: ds})
		} else {
			printDiags(c.Stdout, ds)
		}
		return nil, ExitFailed
	}
	return data, ExitOK
}

func (c *CLI) reportReadError(err error, jsonOut bool) {
	d := diag.Error("input/read", "", "入力ファイルを開けません: "+err.Error())
	if jsonOut {
		_ = writeJSON(c.Stdout, validateReport{Profile: "standard", Errors: 1, Diagnostics: diag.List{d}})
	} else {
		printDiags(c.Stdout, diag.List{d})
	}
}

// reportValidation は検証結果を出力し終了コードを返す。
func (c *CLI) reportValidation(res *buildResult, jsonOut bool) int {
	errs, warns := res.Diags.Count()
	rep := validateReport{
		OK: res.OK, Profile: res.Profile,
		Errors: errs, Warnings: warns,
		Diagnostics: res.Diags, Checks: res.Checks,
	}
	if rep.Diagnostics == nil {
		rep.Diagnostics = diag.List{}
	}
	if jsonOut {
		_ = writeJSON(c.Stdout, rep)
	} else {
		printDiags(c.Stdout, res.Diags)
		var buf bytes.Buffer
		ran, issues := checkSummary(res.Checks)
		fmt.Fprintf(&buf, "profile=%s errors=%d warnings=%d checks=%d/%d passed\n",
			res.Profile, errs, warns, ran-issues, ran)
		if res.OK {
			buf.WriteString("OK\n")
		} else {
			buf.WriteString("FAILED\n")
		}
		_, _ = c.Stdout.Write(buf.Bytes())
	}
	if res.OK {
		return ExitOK
	}
	return ExitFailed
}

// checkSummary は (実行数, 検出ありのチェック数) を返す。
func checkSummary(checks []validate.CheckResult) (ran, withIssues int) {
	for _, ch := range checks {
		if ch.Ran {
			ran++
			if ch.Issues > 0 {
				withIssues++
			}
		}
	}
	return
}
