// cmd_deliver.go は deliver コマンド (最終受け入れの儀式 §12) を実装する。
//
//  1. 入力 JSON のバイト列を同一ディレクトリの非公開スナップショット (0600) に凍結する
//  2. スナップショットからレンダリングする (元ファイルが途中で変わっても影響を受けない)
//  3. 全チェックを実行する
//  4. すべて通ればアトミックにコミット。1 つでも落ちれば既存の出力を一切変更せず非ゼロ終了
//  5. レシート (SHA-256・バイト数・チェック一覧) を出力する
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/outpath"
	"github.com/ZONO33LHD/archy-go/internal/validate"
)

// deliverReceipt は deliver --json のレシート。
type deliverReceipt struct {
	OK          bool                   `json:"ok"`
	Profile     string                 `json:"profile"`
	Output      string                 `json:"output,omitempty"`
	InputSHA256 string                 `json:"input_sha256"`
	InputBytes  int64                  `json:"input_bytes"`
	HTMLSHA256  string                 `json:"html_sha256,omitempty"`
	HTMLBytes   int64                  `json:"html_bytes,omitempty"`
	Snapshot    string                 `json:"snapshot,omitempty"`
	Errors      int                    `json:"errors"`
	Warnings    int                    `json:"warnings"`
	Diagnostics diag.List              `json:"diagnostics"`
	Checks      []validate.CheckResult `json:"checks"`
	Opened      bool                   `json:"opened"`
	OpenReason  string                 `json:"open_reason,omitempty"`
}

func (c *CLI) cmdDeliver(args []string) int {
	f, pos, err := splitArgs(args)
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta deliver: %v\n", err)
		return ExitUsage
	}
	if len(pos) != 3 {
		fmt.Fprintln(c.Stderr, "使い方: veduta deliver <type> <input.json> <output.html> [--json] [--quality ...] [--open|--no-open]")
		return ExitUsage
	}
	diagramType, inputPath, outPath := pos[0], pos[1], pos[2]

	if err := outpath.Validate(outPath); err != nil {
		fmt.Fprintf(c.Stderr, "veduta deliver: 出力パスが不正です: %v\n", err)
		return ExitUsage
	}

	data, code := c.readInput(inputPath, f.json)
	if code != ExitOK {
		return code
	}

	// 手順 1-3: 読み込み済みの不変バイト列 (readInput 済み) からレンダリングし全チェックを実行する。
	// data は既にメモリ上で凍結されており、元ファイルが途中で変わっても影響を受けない (§12)。
	inputHash := sha256Hex(data)
	res := build(diagramType, data, f.quality, true)

	receipt := deliverReceipt{
		Profile:     res.Profile,
		InputSHA256: inputHash,
		InputBytes:  int64(len(data)),
		Diagnostics: orEmpty(res.Diags),
		Checks:      res.Checks,
	}
	receipt.Errors, receipt.Warnings = res.Diags.Count()

	if !res.OK {
		// 手順 4 (失敗側): 既存の出力を一切変更せず非ゼロ終了。スナップショットも作らない
		// (検証失敗時に残すと、悪意ある入力の反復でディスクを消費できてしまう)。
		// 非ゼロ終了を成功と表現してはいけない。何が落ちたかを診断として返す。
		if f.json {
			_ = writeJSON(c.Stdout, receipt)
		} else {
			printDiags(c.Stdout, res.Diags)
			ran, issues := checkSummary(res.Checks)
			fmt.Fprintf(c.Stdout, "deliver FAILED — 出力は変更されていません (checks %d/%d passed, errors=%d warnings=%d)\n",
				ran-issues, ran, receipt.Errors, receipt.Warnings)
		}
		return ExitFailed
	}

	// 手順 4 (成功側): 検証を通ったので入力バイトを非公開スナップショットに凍結し、
	// その後アトミックにコミットする。
	snapPath, err := freezeSnapshot(inputPath, inputHash, data)
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta deliver: スナップショットの作成に失敗しました: %v\n", err)
		return ExitFailed
	}
	wr, err := outpath.WriteAtomic(c.Cwd, outPath, []byte(res.HTML))
	if err != nil {
		fmt.Fprintf(c.Stderr, "veduta deliver: 出力に失敗しました: %v\n", err)
		return ExitFailed
	}

	htmlHash := sha256Hex([]byte(res.HTML))
	opened, openReason := c.maybeOpen(true, f, wr.AbsPath, wr.Bytes, htmlHash)

	receipt.OK = true
	receipt.Output = wr.AbsPath
	receipt.Snapshot = snapPath
	receipt.HTMLSHA256 = htmlHash
	receipt.HTMLBytes = wr.Bytes
	receipt.Opened = opened
	receipt.OpenReason = openReason

	if f.json {
		_ = writeJSON(c.Stdout, receipt)
		return ExitOK
	}
	// 手順 5: レシート (§11.6 の形式)。
	printDiags(c.Stdout, res.Diags)
	fmt.Fprintln(c.Stdout, wr.AbsPath)
	fmt.Fprintf(c.Stdout, "  spec   sha256:%s  %s\n", shortHash(inputHash), formatBytes(receipt.InputBytes))
	fmt.Fprintf(c.Stdout, "  html   sha256:%s  %s\n", shortHash(receipt.HTMLSHA256), formatBytes(receipt.HTMLBytes))
	ran, issues := checkSummary(res.Checks)
	fmt.Fprintf(c.Stdout, "  checks %d/%d passed\n", ran-issues, ran)
	c.printOpenStatus(opened, openReason, f.open)
	return ExitOK
}

// freezeSnapshot は入力バイト列を入力と同一ディレクトリの非公開ファイル (0600) に凍結する。
//
// 入力ディレクトリを os.Root で開いて封じ込め、固定名の解決がシンボリックリンク経由で
// 外へ逃げないようにする。既存ファイルがある場合は Lstat で「通常ファイル・非 symlink・0600」を
// 確認し、内容一致 (名前は完全ハッシュ由来) のときだけ再利用する。0644 や symlink の既存は拒否する
// (「非公開スナップショット」の契約を守るため)。
func freezeSnapshot(inputPath, inputHash string, data []byte) (string, error) {
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	name := "." + base + ".veduta-snap-" + inputHash + ".json"

	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("入力ディレクトリを開けません: %w", err)
	}
	defer root.Close()

	if fi, lerr := root.Lstat(name); lerr == nil {
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return "", errors.New("既存スナップショットが通常ファイルではありません")
		}
		if fi.Mode().Perm() != 0o600 {
			return "", errors.New("既存スナップショットのパーミッションが 0600 ではありません")
		}
		existing, rerr := root.ReadFile(name)
		if rerr == nil && bytes.Equal(existing, data) {
			return filepath.Join(dir, name), nil // 同一内容を再利用する
		}
		return "", errors.New("既存スナップショットの内容が一致しません")
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return "", fmt.Errorf("既存スナップショットを検査できません: %w", lerr)
	}

	fh, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, werr := fh.Write(data)
	if serr := fh.Sync(); werr == nil {
		werr = serr
	}
	if cerr := fh.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = root.Remove(name)
		return "", werr
	}
	return filepath.Join(dir, name), nil
}

var _ = ir.MaxInputBytes // ドキュメント参照 (サイズ上限は readInput で強制済み)
