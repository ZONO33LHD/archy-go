// cmd_demo.go は demo / examples コマンドを実装する。
package cli

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/ZONO33LHD/archy-go/assets"
	"github.com/ZONO33LHD/archy-go/internal/outpath"
)

// demoExample は同梱サンプルの定義。
type demoExample struct {
	Name  string // assets/examples/ 配下のファイル名 (拡張子なし)
	Type  string
	Title string
}

var demoExamples = []demoExample{
	{Name: "architecture-web-app", Type: "architecture", Title: "Sample Web App"},
}

func (c *CLI) cmdDemo(args []string) int {
	f, pos, err := splitArgs(args)
	if err != nil || len(pos) != 1 {
		fmt.Fprintln(c.Stderr, "使い方: veduta demo <dir>")
		return ExitUsage
	}
	dir := pos[0]
	if err := validateDemoDir(dir); err != nil {
		fmt.Fprintf(c.Stderr, "veduta demo: %v\n", err)
		return ExitUsage
	}

	root, err := os.OpenRoot(c.Cwd)
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta demo: 作業ディレクトリを開けません: %v\n", err)
		return ExitFailed
	}
	defer root.Close()
	if err := root.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(c.Stderr, "veduta demo: ディレクトリを作成できません: %v\n", err)
		return ExitFailed
	}

	for _, ex := range demoExamples {
		data := assets.MustRead("examples/" + ex.Name + ".json")

		jsonPath := path.Join(dir, ex.Name+".json")
		if err := writeViaRoot(root, jsonPath, data); err != nil {
			fmt.Fprintf(c.Stderr, "veduta demo: %s の書き込みに失敗しました: %v\n", jsonPath, err)
			return ExitFailed
		}

		res := build(ex.Type, data, "", false)
		if !res.OK {
			printDiags(c.Stdout, res.Diags)
			fmt.Fprintf(c.Stderr, "veduta demo: 同梱サンプル %s が検証を通りません (内部不整合)\n", ex.Name)
			return ExitInternal
		}
		htmlPath := path.Join(dir, ex.Name+".html")
		wr, err := outpath.WriteAtomic(c.Cwd, htmlPath, []byte(res.HTML))
		if err != nil {
			fmt.Fprintf(c.Stderr, "veduta demo: %s の書き込みに失敗しました: %v\n", htmlPath, err)
			return ExitFailed
		}
		fmt.Fprintln(c.Stdout, wr.AbsPath)
	}
	_ = f
	return ExitOK
}

// validateDemoDir は demo 出力ディレクトリの安全性検査 (outpath の規則のサブセット)。
func validateDemoDir(dir string) error {
	if dir == "" || len(dir) > 200 {
		return fmt.Errorf("ディレクトリ名が不正です")
	}
	if strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, "~") ||
		strings.ContainsAny(dir, "\\:") {
		return fmt.Errorf("cwd 配下の相対パスのみ指定できます")
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == "" || seg == "." || seg == ".." ||
			strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return fmt.Errorf("不正なパスセグメントが含まれています")
		}
	}
	for _, r := range dir {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("パスに制御文字が含まれています")
		}
	}
	return nil
}

// writeViaRoot は os.Root 経由の単純書き込み (JSON サンプル用)。
func writeViaRoot(root *os.Root, rel string, data []byte) error {
	fh, err := root.OpenFile(rel, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := fh.Write(data)
	if cerr := fh.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

func (c *CLI) cmdExamples(args []string) int {
	_, pos, err := splitArgs(args)
	if err != nil || len(pos) != 0 {
		fmt.Fprintln(c.Stderr, "使い方: veduta examples")
		return ExitUsage
	}
	sorted := append([]demoExample{}, demoExamples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, ex := range sorted {
		fmt.Fprintf(c.Stdout, "%-28s %-14s %s\n", ex.Name, ex.Type, ex.Title)
	}
	return ExitOK
}
