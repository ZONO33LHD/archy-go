// Package cli は veduta のコマンド実装を提供する。
//
// main はこのパッケージの薄いラッパーであり、CLI 全体はテスト可能な形で構成する。
// 終了コード: 0=成功 / 1=検証・受け入れ失敗 / 2=使い方の誤り / 3=内部エラー。
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ZONO33LHD/archy-go/internal/browser"
)

// 終了コード。
const (
	ExitOK       = 0
	ExitFailed   = 1
	ExitUsage    = 2
	ExitInternal = 3
)

// CLI は依存を注入されたコマンド実行環境。
type CLI struct {
	Stdout io.Writer
	Stderr io.Writer
	// Cwd は出力封じ込めの基準ディレクトリ。
	Cwd string
	// Launcher はブラウザ起動 (テストではフェイク)。
	Launcher browser.Launcher
	// Probe は自動オープン判定の環境観測値。
	Probe browser.Probe
	// Version はビルド時に固定されるバージョン文字列。
	Version string
}

// Run はコマンドを実行する。IR 由来の入力でパニックしてはならないため、
// 最上位に recover を 1 つだけ置く (これは保険であり、パニック自体はファジングで潰す §10.9)。
//
// 標準出力は errWriter で包み、コマンドが ExitOK を返しても書き込みに失敗していれば
// ExitInternal に格上げする (broken pipe・ディスク枯渇で「書けていないのに成功」を防ぐ §CRITICAL)。
func (c *CLI) Run(args []string) (code int) {
	ew := &errWriter{w: c.Stdout}
	c.Stdout = ew
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(c.Stderr, "veduta: 内部エラーが発生しました: %v\n", r)
			code = ExitInternal
			return
		}
		if code == ExitOK && ew.Err != nil {
			fmt.Fprintf(c.Stderr, "veduta: 標準出力への書き込みに失敗しました: %v\n", ew.Err)
			code = ExitInternal
		}
	}()

	if len(args) == 0 {
		c.usage()
		return ExitUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "validate":
		return c.cmdValidate(rest)
	case "render":
		return c.cmdRender(rest)
	case "deliver":
		return c.cmdDeliver(rest)
	case "doctor":
		return c.cmdDoctor(rest)
	case "demo":
		return c.cmdDemo(rest)
	case "examples":
		return c.cmdExamples(rest)
	case "preview", "compare", "inspect":
		fmt.Fprintf(c.Stderr, "veduta: %s は第2段階で実装予定です (未実装)\n", cmd)
		return ExitUsage
	case "version", "--version", "-v":
		fmt.Fprintln(c.Stdout, "veduta "+c.Version)
		return ExitOK
	case "help", "--help", "-h":
		c.usage()
		return ExitOK
	default:
		fmt.Fprintf(c.Stderr, "veduta: 未知のコマンドです: %s\n", cmd)
		c.usage()
		return ExitUsage
	}
}

func (c *CLI) usage() {
	fmt.Fprint(c.Stderr, `veduta — 検証可能なシステム構成図ジェネレータ

使い方:
  veduta validate <type> <input.json> [--json] [--quality standard|showcase]
  veduta render   <type> <input.json> <output.html> [--json] [--quality ...] [--open|--no-open]
  veduta deliver  <type> <input.json> <output.html> [--json] [--quality ...] [--open|--no-open]
  veduta doctor   [--json]
  veduta demo     <dir>
  veduta examples

図種 (第1段階): architecture
`)
}

// NewSystem は実環境向けの CLI を構成する。
func NewSystem(version string) (*CLI, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("作業ディレクトリを取得できません: %w", err)
	}
	return &CLI{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Cwd:      cwd,
		Launcher: browser.ExecLauncher{},
		Probe:    browser.SystemProbe(),
		Version:  version,
	}, nil
}
