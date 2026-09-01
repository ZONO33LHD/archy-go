package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZONO33LHD/archy-go/internal/browser"
)

// fakeLauncher は起動せず「どのターゲットで呼ばれたか」を記録する (§13)。
type fakeLauncher struct {
	calls []string
	err   error
}

func (f *fakeLauncher) Open(target string) error {
	f.calls = append(f.calls, target)
	return f.err
}

// permissiveProbe は抑止条件に一切該当しない環境。
func permissiveProbe(env map[string]string) browser.Probe {
	return browser.Probe{
		LookupEnv: func(k string) (string, bool) {
			v, ok := env[k]
			return v, ok
		},
		StdoutTTY: true, StderrTTY: true,
		GOOS: "darwin", HasDockerEnv: false,
	}
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name       string
		cmdDefault bool
		open       bool
		noOpen     bool
		probe      browser.Probe
		want       bool
	}{
		{"render既定は開かない", false, false, false, permissiveProbe(nil), false},
		{"deliver既定は開く", true, false, false, permissiveProbe(nil), true},
		{"--no-openはdeliver既定に優先", true, false, true, permissiveProbe(nil), false},
		{"--openはrender既定に優先", false, true, false, permissiveProbe(nil), true},
		{"VEDUTA_NO_OPEN", true, false, false, permissiveProbe(map[string]string{"VEDUTA_NO_OPEN": "1"}), false},
		{"CI=true", true, false, false, permissiveProbe(map[string]string{"CI": "true"}), false},
		{"CI=空文字でも抑止", true, false, false, permissiveProbe(map[string]string{"CI": ""}), false},
		{"TERM=dumb", true, false, false, permissiveProbe(map[string]string{"TERM": "dumb"}), false},
		{"非TTY", true, false, false, func() browser.Probe {
			p := permissiveProbe(nil)
			p.StdoutTTY = false
			return p
		}(), false},
		{"LinuxでDISPLAY未設定", true, false, false, func() browser.Probe {
			p := permissiveProbe(nil)
			p.GOOS = "linux"
			return p
		}(), false},
		{"LinuxでDISPLAYあり", true, false, false, func() browser.Probe {
			p := permissiveProbe(map[string]string{"DISPLAY": ":0"})
			p.GOOS = "linux"
			return p
		}(), true},
		{"SSHで表示環境なし", true, false, false, permissiveProbe(map[string]string{"SSH_CONNECTION": "x"}), false},
		{"SSHでもDISPLAYがあれば開く", true, false, false, permissiveProbe(map[string]string{"SSH_CONNECTION": "x", "DISPLAY": ":0"}), true},
		{"Docker内", true, false, false, func() browser.Probe {
			p := permissiveProbe(nil)
			p.HasDockerEnv = true
			return p
		}(), false},
		// VEDUTA_OPEN=1 のような有効化用の環境変数は存在しない (§11.2)。
		{"VEDUTA_OPEN=1でもrender既定のまま", false, false, false, permissiveProbe(map[string]string{"VEDUTA_OPEN": "1"}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := browser.Decide(tt.cmdDefault, tt.open, tt.noOpen, tt.probe)
			if got.Open != tt.want {
				t.Errorf("Decide = %v (%s), want %v", got.Open, got.Reason, tt.want)
			}
		})
	}
}

// newTestCLI はテスト用の CLI (一時 cwd + フェイクランチャー)。
func newTestCLI(t *testing.T, launcher browser.Launcher, probe browser.Probe) (*CLI, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()
	var out bytes.Buffer
	c := &CLI{
		Stdout: &out, Stderr: &out,
		Cwd:      dir,
		Launcher: launcher,
		Probe:    probe,
		Version:  "test",
	}
	return c, &out, dir
}

func writeInput(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderDoesNotOpenByDefault(t *testing.T) {
	fl := &fakeLauncher{}
	c, _, dir := newTestCLI(t, fl, permissiveProbe(nil))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	if code := c.Run([]string{"render", "architecture", in, "out.html"}); code != ExitOK {
		t.Fatalf("render 失敗: code=%d", code)
	}
	if len(fl.calls) != 0 {
		t.Errorf("render が既定でブラウザを起動しました: %v", fl.calls)
	}
}

func TestDeliverOpensByDefault(t *testing.T) {
	fl := &fakeLauncher{}
	c, out, dir := newTestCLI(t, fl, permissiveProbe(nil))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	if code := c.Run([]string{"deliver", "architecture", in, "out.html"}); code != ExitOK {
		t.Fatalf("deliver 失敗: code=%d\n%s", code, out.String())
	}
	if len(fl.calls) != 1 {
		t.Fatalf("deliver が既定でブラウザを起動しませんでした")
	}
	// 渡されるのは絶対パス。
	if !filepath.IsAbs(fl.calls[0]) {
		t.Errorf("起動ターゲットが絶対パスではありません: %s", fl.calls[0])
	}
}

func TestDeliverNoOpenFlag(t *testing.T) {
	fl := &fakeLauncher{}
	c, _, dir := newTestCLI(t, fl, permissiveProbe(nil))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	if code := c.Run([]string{"deliver", "architecture", in, "out.html", "--no-open"}); code != ExitOK {
		t.Fatal("deliver 失敗")
	}
	if len(fl.calls) != 0 {
		t.Error("--no-open なのに起動されました")
	}
}

// TestDeliverOpenFailureDoesNotAffectExitCode: 起動が ENOENT で失敗しても
// 終了コードは 0 のまま、かつパスが標準出力に出る (§11.5)。
func TestDeliverOpenFailureDoesNotAffectExitCode(t *testing.T) {
	fl := &fakeLauncher{err: errors.New("exec: \"open\": executable file not found in $PATH")}
	c, out, dir := newTestCLI(t, fl, permissiveProbe(nil))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	code := c.Run([]string{"deliver", "architecture", in, "out.html"})
	if code != ExitOK {
		t.Fatalf("起動失敗が終了コードに伝播しています: code=%d", code)
	}
	if !strings.Contains(out.String(), filepath.Join(dir, "out.html")) {
		t.Error("成果物パスが標準出力に出ていません")
	}
	// レシート (spec/html/checks) も正常に出力される。
	for _, want := range []string{"spec   sha256:", "html   sha256:", "checks "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("レシートに %q がありません:\n%s", want, out.String())
		}
	}
}

// TestDeliverFailureDoesNotOpen: 検証失敗で非ゼロ終了なら起動しない (§13)。
func TestDeliverFailureDoesNotOpen(t *testing.T) {
	fl := &fakeLauncher{}
	c, out, dir := newTestCLI(t, fl, permissiveProbe(nil))
	// ノード重なりで構図検証に落ちる入力。
	bad := strings.Replace(buildDocJSON("ok"), `"pos": [560, 0]`, `"pos": [10, 10]`, 1)
	in := writeInput(t, dir, "in.json", bad)
	code := c.Run([]string{"deliver", "architecture", in, "out.html"})
	if code == ExitOK {
		t.Fatal("壊れた図が deliver を通りました")
	}
	if len(fl.calls) != 0 {
		t.Error("deliver 失敗時にブラウザが起動されました")
	}
	if _, err := os.Stat(filepath.Join(dir, "out.html")); !os.IsNotExist(err) {
		t.Error("deliver 失敗時に出力が書かれています")
	}
	if strings.Contains(out.String(), "OK") && !strings.Contains(out.String(), "FAILED") {
		t.Error("非ゼロ終了が成功と表現されています")
	}
}

// TestDeliverKeepsExistingOutputOnFailure: 失敗時は既存の出力を一切変更しない (§12)。
func TestDeliverKeepsExistingOutputOnFailure(t *testing.T) {
	fl := &fakeLauncher{}
	c, _, dir := newTestCLI(t, fl, permissiveProbe(nil))
	prev := filepath.Join(dir, "out.html")
	if err := os.WriteFile(prev, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(buildDocJSON("ok"), `"pos": [560, 0]`, `"pos": [10, 10]`, 1)
	in := writeInput(t, dir, "in.json", bad)
	if code := c.Run([]string{"deliver", "architecture", in, "out.html"}); code == ExitOK {
		t.Fatal("失敗すべき deliver が成功しました")
	}
	got, _ := os.ReadFile(prev)
	if string(got) != "previous" {
		t.Error("deliver 失敗時に既存の出力が変更されました")
	}
}

// TestOpenTargetWithDashName: 出力名が '-' 始まりでも、渡されるのは
// '-' で始まらない絶対パス (§13)。
func TestOpenTargetWithDashName(t *testing.T) {
	fl := &fakeLauncher{}
	c, _, dir := newTestCLI(t, fl, permissiveProbe(nil))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	if code := c.Run([]string{"deliver", "architecture", in, "-x.html"}); code != ExitOK {
		t.Fatal("deliver 失敗")
	}
	if len(fl.calls) != 1 {
		t.Fatal("起動されていません")
	}
	if strings.HasPrefix(fl.calls[0], "-") {
		t.Errorf("起動ターゲットが '-' で始まっています: %s", fl.calls[0])
	}
	if !filepath.IsAbs(fl.calls[0]) {
		t.Errorf("絶対パスではありません: %s", fl.calls[0])
	}
}

// TestOpenTargetSpecialChars: '#' '?' を含む名前でも切り詰められない (§11.3)。
func TestOpenTargetSpecialChars(t *testing.T) {
	fl := &fakeLauncher{}
	c, _, dir := newTestCLI(t, fl, permissiveProbe(nil))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	if code := c.Run([]string{"deliver", "architecture", in, "a#b?c.html"}); code != ExitOK {
		t.Fatal("deliver 失敗")
	}
	if len(fl.calls) != 1 || !strings.HasSuffix(fl.calls[0], "a#b?c.html") {
		t.Errorf("特殊文字を含むパスが切り詰められました: %v", fl.calls)
	}
}

// TestVerifyWrittenSizeMismatch: 書き込み後にサイズ・内容が変わっていたら開かない (§11.3)。
func TestVerifyWrittenSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.html")
	if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha := sha256HexStr([]byte("12345"))
	if _, err := browser.VerifyWritten(p, 5, sha); err != nil {
		t.Errorf("サイズ・内容一致で失敗: %v", err)
	}
	if _, err := browser.VerifyWritten(p, 4, ""); err == nil {
		t.Error("サイズ不一致 (すり替え) が検出されませんでした")
	}
	// 同じサイズだが内容が違う場合は content-mismatch で拒否される。
	if _, err := browser.VerifyWritten(p, 5, sha256HexStr([]byte("54321"))); err == nil {
		t.Error("内容不一致 (同サイズすり替え) が検出されませんでした")
	}
	// シンボリックリンクは拒否される。
	link := filepath.Join(dir, "link.html")
	if err := os.Symlink(p, link); err == nil {
		if _, err := browser.VerifyWritten(link, 5, sha); err == nil {
			t.Error("シンボリックリンクが拒否されませんでした")
		}
	}
}

func sha256HexStr(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestDeliverSnapshotFrozen: 入力バイトがスナップショットに 0600 で凍結される (§12)。
func TestDeliverSnapshotFrozen(t *testing.T) {
	fl := &fakeLauncher{}
	c, _, dir := newTestCLI(t, fl, permissiveProbe(map[string]string{"VEDUTA_NO_OPEN": "1"}))
	content := buildDocJSON("ok")
	in := writeInput(t, dir, "in.json", content)
	if code := c.Run([]string{"deliver", "architecture", in, "out.html"}); code != ExitOK {
		t.Fatal("deliver 失敗")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".in.json.veduta-snap-*.json"))
	if len(matches) != 1 {
		t.Fatalf("スナップショットが見つかりません: %v", matches)
	}
	fi, _ := os.Stat(matches[0])
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("スナップショットのパーミッション = %v, want 0600", fi.Mode().Perm())
	}
	got, _ := os.ReadFile(matches[0])
	if string(got) != content {
		t.Error("スナップショット内容が入力と一致しません")
	}
	// 再実行してもスナップショットは再利用され、成功する。
	if code := c.Run([]string{"deliver", "architecture", in, "out.html"}); code != ExitOK {
		t.Fatal("2 回目の deliver が失敗")
	}
}

// TestRenderJSONReceipt: --json レシートに opened / open_reason が含まれる (§11.5)。
func TestRenderJSONReceipt(t *testing.T) {
	fl := &fakeLauncher{}
	c, out, dir := newTestCLI(t, fl, permissiveProbe(map[string]string{"CI": "1"}))
	in := writeInput(t, dir, "in.json", buildDocJSON("ok"))
	if code := c.Run([]string{"deliver", "architecture", in, "out.html", "--json"}); code != ExitOK {
		t.Fatal("deliver 失敗")
	}
	s := out.String()
	if !strings.Contains(s, `"opened":false`) {
		t.Errorf("レシートに opened がありません: %s", s)
	}
	if !strings.Contains(s, `"open_reason"`) {
		t.Errorf("レシートに open_reason がありません: %s", s)
	}
}
