// Package outpath は出力パスの安全性検査とアトミック書き込みの集約点である (§10.4)。
//
// cwd 配下への封じ込めは os.Root で行う。os.Root は相対パスの解決をカーネル境界で行い、
// `..` による脱出とシンボリックリンクによる脱出をどちらも拒否する。
// ただし os.Root はファイル名そのものの妥当性は検査しないため、
// 拡張子・Windows 予約名・ADS・制御文字は Validate で自前検査する。
package outpath

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ZONO33LHD/archy-go/internal/safe"
)

// MaxPathLen は受け付けるパスの最大バイト長。
const MaxPathLen = 300

// windowsReserved は Windows の予約デバイス名 (大文字表記)。
// 拡張子の有無にかかわらず拒否する (CON.html も CON も不可)。
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"CONIN$": true, "CONOUT$": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true, "COM0": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true, "LPT0": true,
}

// Validate は出力パスが「cwd 配下の相対パスかつ .html」の制約を満たすか検査する。
// パーセントデコードや Unicode 正規化は行わない (入力をそのまま検査する)。
// 返す error のメッセージにはパス文字列をそのまま含めない (制御文字注入対策は呼び出し側で行う)。
func Validate(p string) error {
	if p == "" {
		return errors.New("outpath/empty: 出力パスが空です")
	}
	if len(p) > MaxPathLen {
		return fmt.Errorf("outpath/too-long: 出力パスが長すぎます (最大 %d バイト)", MaxPathLen)
	}
	// 制御文字 (改行・タブ含む)・双方向制御文字・ゼロ幅文字・不正 UTF-8 をパスでも拒否する。
	// パスは端末やエラーメッセージに現れるため safe と同じ基準で無害化する。
	// CheckSingleLine を使い、本文では許す \n \t もパスでは拒否する。
	if vs := safe.CheckSingleLine(p); len(vs) > 0 {
		return fmt.Errorf("outpath/bad-char: 出力パスに使用できない文字 (%s) が含まれています", vs[0].Rune)
	}
	if strings.ContainsRune(p, '\\') {
		// UNC (\\server\share) とドライブ相対 (C:foo) の温床。区切りは / のみ許可。
		return errors.New("outpath/backslash: パス区切りは '/' のみ使用できます")
	}
	if strings.ContainsRune(p, ':') {
		// ドライブ指定 (C:) と NTFS 代替データストリーム (evil.html:stream) を弾く。
		return errors.New("outpath/colon: 出力パスに ':' は使用できません")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return errors.New("outpath/absolute: 出力パスは cwd 配下の相対パスのみ許可されます")
	}
	segs := strings.Split(p, "/")
	for _, seg := range segs {
		if seg == "" {
			return errors.New("outpath/empty-segment: 空のパスセグメントが含まれています")
		}
		if seg == "." || seg == ".." {
			return errors.New("outpath/dot-segment: '.' および '..' セグメントは使用できません")
		}
		if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return errors.New("outpath/trailing-dot-space: セグメント末尾のドット・空白は使用できません")
		}
		// 予約名は最初のドットより前の部分で判定する (CON.html も対象)。
		// Windows は末尾の空白・ドットを無視して予約名を解決するため、
		// トリムした基底名でも判定する ("CON .html" や "CON.html" を弾く)。
		base, _, _ := strings.Cut(seg, ".")
		base = strings.TrimRight(base, " .")
		if windowsReserved[strings.ToUpper(base)] {
			return errors.New("outpath/reserved-name: Windows 予約デバイス名は使用できません")
		}
	}
	final := segs[len(segs)-1]
	if !strings.HasSuffix(final, ".html") || len(final) <= len(".html") {
		return errors.New("outpath/extension: 出力パスの拡張子は .html のみ許可されます")
	}
	return nil
}

// WriteResult はアトミック書き込みの結果。
type WriteResult struct {
	// AbsPath はコミットされたファイルの絶対パス (自動オープン・表示用)。
	AbsPath string
	// Bytes は書き込んだバイト数。オープン前のすり替え検査に使う。
	Bytes int64
}

// WriteAtomic は §10.4 の手順で data を cwd 配下の rel へアトミックに書き込む。
//  1. 同一ディレクトリに一意名の一時ファイルを O_CREATE|O_EXCL|O_WRONLY, 0600 で作る
//  2. 内容を書き Sync する
//  3. 0644 に変更する
//  4. 最終パスへ rename する
//  5. 親ディレクトリを Sync する (rename の永続化)
//  6. 失敗時は一時ファイルを必ず削除する
//
// すべて os.Root のメソッド経由で行い、検査と操作の間の TOCTOU 窓を作らない。
func WriteAtomic(cwd, rel string, data []byte) (*WriteResult, error) {
	if err := Validate(rel); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, fmt.Errorf("outpath/root: 作業ディレクトリを開けません: %w", err)
	}
	defer root.Close()

	dir := path.Dir(rel)
	if dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("outpath/mkdir: 出力ディレクトリを作成できません: %w", err)
		}
	}

	// 出力先が既にシンボリックリンクなら拒否する (cwd 外を指すリンクの置換も含めて許可しない)。
	if fi, err := root.Lstat(rel); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("outpath/symlink-target: 出力先がシンボリックリンクです")
		}
		if fi.IsDir() {
			return nil, errors.New("outpath/is-dir: 出力先がディレクトリです")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("outpath/lstat: 出力先を検査できません: %w", err)
	}

	// 一時ファイル名の乱数は成果物の内容に影響しないため決定論性を損なわない。
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return nil, fmt.Errorf("outpath/rand: 一時ファイル名を生成できません: %w", err)
	}
	tmp := path.Join(dir, ".veduta-tmp-"+hex.EncodeToString(suffix[:]))

	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("outpath/tmp-create: 一時ファイルを作成できません: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = root.Remove(tmp)
		}
	}()

	n, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	if werr == nil {
		werr = f.Chmod(0o644)
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return nil, fmt.Errorf("outpath/write: 一時ファイルへの書き込みに失敗しました: %w", werr)
	}
	if n != len(data) {
		return nil, errors.New("outpath/short-write: 書き込みバイト数が一致しません")
	}

	if err := root.Rename(tmp, rel); err != nil {
		return nil, fmt.Errorf("outpath/rename: 最終パスへの rename に失敗しました: %w", err)
	}
	committed = true

	// 親ディレクトリの fsync で rename を永続化する。ここでの失敗は
	// クラッシュ時に成果物が失われうることを意味するため、エラーとして返す
	// (ファイル自体は既にコミット済みなので committed=true のまま)。
	if d, err := root.Open(dir); err == nil {
		syncErr := d.Sync()
		closeErr := d.Close()
		if syncErr != nil {
			return nil, fmt.Errorf("outpath/dir-sync: 親ディレクトリの同期に失敗しました: %w", syncErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("outpath/dir-close: 親ディレクトリのクローズに失敗しました: %w", closeErr)
		}
	} else {
		return nil, fmt.Errorf("outpath/dir-open: 親ディレクトリを開けません: %w", err)
	}

	abs, err := filepath.Abs(filepath.Join(cwd, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("outpath/abs: 絶対パスの解決に失敗しました: %w", err)
	}
	return &WriteResult{AbsPath: abs, Bytes: int64(len(data))}, nil
}
