package outpath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidate は §13 のパス系セキュリティ回帰テスト。
// 各項目は「攻撃入力」と「期待される拒否理由コード」の対で書く。
func TestValidate(t *testing.T) {
	tests := []struct {
		name, in string
		wantCode string // エラーメッセージ先頭の "outpath/..." コード。"" は許可
	}{
		{"通常", "out.html", ""},
		{"サブディレクトリ", "dist/out.html", ""},
		{"ダッシュ始まり", "-x.html", ""},
		{"特殊文字を含む名前", "a#b?c.html", ""},
		{"トラバーサル", "../../../etc/passwd", "outpath/dot-segment"},
		{"絶対パス", "/tmp/x.html", "outpath/absolute"},
		{"チルダ", "~/x.html", "outpath/absolute"},
		{"ドライブ相対", "C:foo.html", "outpath/colon"},
		{"UNC", `\\server\share\x.html`, "outpath/backslash"},
		{"ADS", "evil.html:stream", "outpath/colon"},
		{"予約名CON", "CON.html", "outpath/reserved-name"},
		{"予約名NUL", "NUL", "outpath/reserved-name"},
		{"予約名COM9", "com9.html", "outpath/reserved-name"},
		{"予約名CONIN$", "CONIN$.html", "outpath/reserved-name"},
		{"予約名CON末尾空白", "CON .html", "outpath/reserved-name"},
		{"末尾ドット", "x.html.", "outpath/trailing-dot-space"},
		{"末尾空白", "x.html ", "outpath/trailing-dot-space"},
		{"拡張子なし", "output", "outpath/extension"},
		{"拡張子違い", "x.htm", "outpath/extension"},
		{"拡張子のみ", ".html", "outpath/extension"},
		{"空", "", "outpath/empty"},
		{"NUL文字", "a\x00b.html", "outpath/bad-char"},
		{"改行", "a\nb.html", "outpath/bad-char"},
		{"タブ", "a\tb.html", "outpath/bad-char"},
		{"双方向制御", "a\u202Eb.html", "outpath/bad-char"},
		{"ゼロ幅", "a\u200Bb.html", "outpath/bad-char"},
		{"カレント参照", "./x.html", "outpath/dot-segment"},
		{"空セグメント", "a//b.html", "outpath/empty-segment"},
		{"長すぎ", strings.Repeat("a", 300) + ".html", "outpath/too-long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.in)
			if tt.wantCode == "" {
				if err != nil {
					t.Errorf("Validate(%q) = %v, want nil", tt.in, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want %s", tt.in, tt.wantCode)
			}
			if !strings.HasPrefix(err.Error(), tt.wantCode) {
				t.Errorf("Validate(%q) = %v, want prefix %s", tt.in, err, tt.wantCode)
			}
		})
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	data := []byte("<html>test</html>")

	wr, err := WriteAtomic(dir, "out.html", data)
	if err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, err := os.ReadFile(wr.AbsPath)
	if err != nil || string(got) != string(data) {
		t.Fatalf("書き込み内容が一致しません: %v", err)
	}
	fi, _ := os.Stat(wr.AbsPath)
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("最終パーミッション = %v, want 0644", fi.Mode().Perm())
	}
	if wr.Bytes != int64(len(data)) {
		t.Errorf("Bytes = %d, want %d", wr.Bytes, len(data))
	}

	// 一時ファイルが残っていないこと。
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".veduta-tmp-") {
			t.Errorf("一時ファイルが残っています: %s", e.Name())
		}
	}

	// 上書きも成功する (アトミック置換)。
	if _, err := WriteAtomic(dir, "out.html", []byte("v2")); err != nil {
		t.Fatalf("上書き WriteAtomic: %v", err)
	}
}

func TestWriteAtomicMkdir(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAtomic(dir, "sub/dir/out.html", []byte("x")); err != nil {
		t.Fatalf("サブディレクトリへの WriteAtomic: %v", err)
	}
}

// TestWriteAtomicSymlinkEscape: 出力先の親が cwd 外を指すシンボリックリンク → 拒否。
func TestWriteAtomicSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink 不可: %v", err)
	}
	if _, err := WriteAtomic(dir, "link/out.html", []byte("x")); err == nil {
		t.Fatal("cwd 外への symlink 経由の書き込みが拒否されませんでした")
	}
}

// TestWriteAtomicSymlinkTarget: 出力先そのものが symlink → 拒否 (cwd 外を指す場合も含む)。
func TestWriteAtomicSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.html")
	if err := os.WriteFile(target, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "out.html")); err != nil {
		t.Skipf("symlink 不可: %v", err)
	}
	_, err := WriteAtomic(dir, "out.html", []byte("new"))
	if err == nil {
		t.Fatal("symlink 出力先への書き込みが拒否されませんでした")
	}
	if !strings.Contains(err.Error(), "symlink-target") {
		t.Errorf("エラーコードが想定と異なります: %v", err)
	}
	// 参照先が変更されていないこと。
	got, _ := os.ReadFile(target)
	if string(got) != "orig" {
		t.Error("symlink 参照先が変更されています")
	}
}

// TestWriteAtomicSymlinkCycle: シンボリックリンクの循環 → 無限ループせずエラー。
func TestWriteAtomicSymlinkCycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("b", filepath.Join(dir, "a")); err != nil {
		t.Skipf("symlink 不可: %v", err)
	}
	if err := os.Symlink("a", filepath.Join(dir, "b")); err != nil {
		t.Skip("symlink 不可")
	}
	done := make(chan error, 1)
	go func() {
		_, err := WriteAtomic(dir, "a/out.html", []byte("x"))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("循環 symlink がエラーになりませんでした")
		}
	case <-t.Context().Done():
		t.Fatal("循環 symlink で無限ループしています")
	}
}

// FuzzOutputPath: パニックしない・有限時間で返る (§13 ファジング)。
func FuzzOutputPath(f *testing.F) {
	seeds := []string{
		"out.html", "../x.html", "/abs.html", "a\\b.html", "CON.html",
		"a:b.html", "x.html.", "", strings.Repeat("あ", 100) + ".html",
		"a//b.html", "-x.html", "a#b?c.html", "\x00.html",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		_ = Validate(p) // 不変条件: パニックしない
	})
}
