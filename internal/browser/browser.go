// Package browser は生成後の自動オープン (§11) を実装する。
//
// 原則:
//   - 開いてよいのは、そのプロセスが今まさに書いたファイルだけ (§11.3)
//   - 環境変数は抑止のみに使える。有効化する環境変数は存在しない (§11.2)
//   - オープンの失敗は決して終了コードに影響しない (§11.5)
package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Launcher はブラウザ起動の抽象。テストではフェイクに差し替える。
type Launcher interface {
	// Open は絶対パス (または検証済み URL) を既定のブラウザで開く。
	Open(target string) error
}

// Decision は自動オープンの判定結果。
type Decision struct {
	Open bool
	// Reason は開かない理由 (人間可読・受領証に載せる)。開く場合は空。
	Reason string
}

// Probe は判定に使う環境の観測値。テストで任意に構成できるよう関数と値で持つ。
type Probe struct {
	// LookupEnv は環境変数の存在と値を返す (os.LookupEnv 互換)。
	LookupEnv func(string) (string, bool)
	// StdoutTTY / StderrTTY は標準出力・標準エラーが端末か。
	StdoutTTY bool
	StderrTTY bool
	// GOOS は実行 OS。
	GOOS string
	// HasDockerEnv は /.dockerenv が存在するか。
	HasDockerEnv bool
}

// SystemProbe は実環境から Probe を構成する。
func SystemProbe() Probe {
	_, dockerErr := os.Stat("/.dockerenv")
	return Probe{
		LookupEnv:    os.LookupEnv,
		StdoutTTY:    isTerminal(os.Stdout),
		StderrTTY:    isTerminal(os.Stderr),
		GOOS:         runtime.GOOS,
		HasDockerEnv: dockerErr == nil,
	}
}

// isTerminal は外部パッケージを使わない端末判定 (§11.2)。
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Decide は自動オープンするかを判定する。
// cmdDefault はコマンド別の既定 (§11.1)。flagOpen / flagNoOpen は CLI の明示指定。
// 明示指定は既定に優先するが、環境による抑止条件 (§11.2) は fail-closed であり
// --open でも解除されない (表示環境が無い場所で起動を試みても無意味なため)。
func Decide(cmdDefault, flagOpen, flagNoOpen bool, p Probe) Decision {
	if flagNoOpen {
		return Decision{Reason: "--no-open が指定されています"}
	}
	if v, ok := p.LookupEnv("VEDUTA_NO_OPEN"); ok && v != "" {
		return Decision{Reason: "VEDUTA_NO_OPEN が設定されています"}
	}
	if _, ok := p.LookupEnv("CI"); ok {
		return Decision{Reason: "CI 環境です"}
	}
	if !p.StdoutTTY || !p.StderrTTY {
		return Decision{Reason: "標準出力が端末ではありません"}
	}
	if v, _ := p.LookupEnv("TERM"); v == "dumb" {
		return Decision{Reason: "TERM=dumb です"}
	}
	if p.HasDockerEnv {
		return Decision{Reason: "コンテナ内 (/.dockerenv) です"}
	}
	display, hasDisplay := p.LookupEnv("DISPLAY")
	wayland, hasWayland := p.LookupEnv("WAYLAND_DISPLAY")
	displayOK := (hasDisplay && display != "") || (hasWayland && wayland != "")
	if p.GOOS == "linux" && !displayOK {
		return Decision{Reason: "表示環境 (DISPLAY / WAYLAND_DISPLAY) がありません"}
	}
	_, ssh1 := p.LookupEnv("SSH_CONNECTION")
	_, ssh2 := p.LookupEnv("SSH_TTY")
	if (ssh1 || ssh2) && !displayOK {
		return Decision{Reason: "SSH セッション中で表示環境がありません"}
	}
	if !cmdDefault && !flagOpen {
		return Decision{Reason: "このコマンドの既定では開きません"}
	}
	return Decision{Open: true}
}

// VerifyWritten は書き込み直後のファイルがすり替えられていないかを検査し (§11.3)、
// 起動に渡す絶対パスを返す。相対パスを渡すと '-' で始まる名前が引数として
// 解釈される余地が残るため、必ず絶対パスに正規化する。
//
// 検査は Lstat (シンボリックリンク拒否) → 通常ファイル確認 → サイズ一致 →
// 内容の SHA-256 一致 (wantSHA256 が非空のとき) の順。これにより「同じサイズの
// 別ファイル」や「リンクへの差し替え」を検出する。パスを外部オープナーに渡す以上、
// 検査後の再差し替えレースは完全には閉じられないが、同一プロセスが直後に検査するため窓は極小。
func VerifyWritten(absPath string, wantBytes int64, wantSHA256 string) (string, error) {
	if !filepath.IsAbs(absPath) {
		abs, err := filepath.Abs(absPath)
		if err != nil {
			return "", fmt.Errorf("browser/abs: 絶対パス化に失敗しました: %w", err)
		}
		absPath = abs
	}
	fi, err := os.Lstat(absPath)
	if err != nil {
		return "", fmt.Errorf("browser/stat: 出力ファイルを確認できません: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("browser/symlink: 出力先がシンボリックリンクです (すり替えの可能性)")
	}
	if !fi.Mode().IsRegular() {
		return "", errors.New("browser/not-regular: 出力先が通常ファイルではありません")
	}
	if fi.Size() != wantBytes {
		return "", errors.New("browser/size-mismatch: 書き込み後にファイルサイズが変わっています (すり替えの可能性)")
	}
	if wantSHA256 != "" {
		got, err := os.ReadFile(absPath)
		if err != nil {
			return "", fmt.Errorf("browser/read: 出力ファイルを読めません: %w", err)
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != wantSHA256 {
			return "", errors.New("browser/content-mismatch: 書き込み後に内容が変わっています (すり替えの可能性)")
		}
	}
	return absPath, nil
}

// ExecLauncher は実際に OS のオープナーを起動する Launcher。
type ExecLauncher struct{}

// Open は §10.5 のサブプロセス規則に従いブラウザを起動する。
//   - オープナーは OS ごとの信頼済み絶対パスを優先する (PATH 汚染で偽の open/xdg-open を
//     掴まされないため)。絶対パスが見つからない場合のみ exec.LookPath にフォールバックする
//   - シェルを経由しない。環境変数は継承せず、表示に必要なもののみ明示的に渡す
//   - ブラウザの終了を待たず、Start() の成否だけで起動判定して切り離す
//     (CommandContext は使わない。Start は即座に返るため、タイムアウト付き context だと
//     defer cancel() が起動直後に子プロセスを kill してしまう)
func (ExecLauncher) Open(target string) error {
	path, args, err := resolveOpener(runtime.GOOS, target)
	if err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Env = openerEnv()
	cmd.Stdin = nil
	cmd.Stdout = nil // ブラウザの出力が受領証を汚さないよう破棄する
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("browser/start: 起動に失敗しました: %w", err)
	}
	// 終了を待たない。ゾンビ回収のみ別 goroutine で行う。
	go func() { _ = cmd.Wait() }()
	return nil
}

// resolveOpener は OS ごとのオープナーの実行パスと引数を解決する。
// 信頼済み絶対パス候補を先に試し、無ければ PATH 解決 (カレントディレクトリ解決は拒否)。
func resolveOpener(goos, target string) (string, []string, error) {
	var candidates []string
	var lookup []string
	var args []string
	switch goos {
	case "darwin":
		candidates = []string{"/usr/bin/open"}
		lookup = []string{"open"}
		args = []string{target}
	case "windows":
		sysRoot := os.Getenv("SystemRoot")
		if sysRoot == "" {
			sysRoot = `C:\Windows`
		}
		candidates = []string{sysRoot + `\System32\rundll32.exe`}
		lookup = []string{"rundll32.exe"}
		args = []string{"url.dll,FileProtocolHandler", target}
	default:
		candidates = []string{"/usr/bin/xdg-open", "/bin/xdg-open"}
		lookup = []string{"xdg-open", "wslview"} // WSL では wslview にフォールバック (§11.4)
		args = []string{target}
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
			return c, args, nil
		}
	}
	for _, name := range lookup {
		p, err := exec.LookPath(name)
		if err == nil {
			return p, args, nil
		}
		if errors.Is(err, exec.ErrDot) {
			return "", nil, errors.New("browser/lookpath: カレントディレクトリからのコマンド解決は拒否します")
		}
	}
	return "", nil, errors.New("browser/lookpath: 信頼できるオープナーが見つかりません")
}

// openerEnv は表示に必要な環境変数のみを明示的に構築する。
func openerEnv() []string {
	var env []string
	for _, key := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "PATH", "HOME"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}

// SuppressedByEnvOnly は判定理由が環境由来かを返す (受領証の文言用)。
func SuppressedByEnvOnly(reason string) bool {
	return reason != "" && !strings.Contains(reason, "--no-open") &&
		!strings.Contains(reason, "既定")
}
