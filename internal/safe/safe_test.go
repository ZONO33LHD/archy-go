package safe

import (
	"strings"
	"testing"
)

func TestHTML(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"プレーン", "hello", "hello"},
		{"5文字すべて", `&<>"'`, "&amp;&lt;&gt;&quot;&#39;"},
		{"scriptタグ", `</script><script>alert(1)</script>`,
			"&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"imgタグ", `<img src=x onerror=alert(1)>`,
			"&lt;img src=x onerror=alert(1)&gt;"},
		{"属性脱出", `" onload="alert(1)`, "&quot; onload=&quot;alert(1)"},
		{"日本語はそのまま", "構成図", "構成図"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTML(tt.in); got != tt.want {
				t.Errorf("HTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCheckText(t *testing.T) {
	tests := []struct {
		name, in string
		wantCode string // "" なら違反なし
	}{
		{"プレーン", "hello 世界", ""},
		{"タブと改行は許可", "a\tb\nc", ""},
		{"NUL", "a\x00b", "sanitize/control-char"},
		{"C0制御", "a\x01b", "sanitize/control-char"},
		{"DEL", "a\x7fb", "sanitize/control-char"},
		{"C1制御", "a\u0085b", "sanitize/control-char"},
		{"ANSIエスケープ", "a\x1b[31mred", "sanitize/control-char"},
		{"RLO (Trojan Source)", "a\u202Eb", "sanitize/bidi-control"},
		{"LRI", "a\u2066b", "sanitize/bidi-control"},
		{"LRM", "a\u200Eb", "sanitize/bidi-control"},
		{"ゼロ幅スペース", "a\u200Bb", "sanitize/zero-width"},
		{"ZWJ", "a\u200Db", "sanitize/zero-width"},
		{"BOM", "a\uFEFFb", "sanitize/zero-width"},
		{"ALM (U+061C)", "a\u061Cb", "sanitize/bidi-control"},
		{"WORD JOINER", "a\u2060b", "sanitize/zero-width"},
		{"行区切り U+2028", "a\u2028b", "sanitize/line-separator"},
		{"段落区切り U+2029", "a\u2029b", "sanitize/line-separator"},
		{"不正UTF-8", "a\xffb", "sanitize/invalid-utf8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := CheckText(tt.in)
			if tt.wantCode == "" {
				if len(vs) != 0 {
					t.Errorf("CheckText(%q) = %v, want none", tt.in, vs)
				}
				return
			}
			if len(vs) == 0 {
				t.Fatalf("CheckText(%q) = 違反なし, want %s", tt.in, tt.wantCode)
			}
			if vs[0].Code != tt.wantCode {
				t.Errorf("CheckText(%q)[0].Code = %s, want %s", tt.in, vs[0].Code, tt.wantCode)
			}
		})
	}
}

func TestForTerminal(t *testing.T) {
	// ANSI エスケープが可視表現に置換され、生の ESC が残らないこと (§10.2)。
	got := ForTerminal("red\x1b[31mtext")
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("ForTerminal に生の ESC が残っています: %q", got)
	}
	if !strings.Contains(got, `\x1b`) {
		t.Errorf("ForTerminal が ESC を可視化していません: %q", got)
	}
	// 双方向制御文字も可視化される。
	got = ForTerminal("a\u202Eb")
	if strings.ContainsRune(got, 0x202E) {
		t.Errorf("ForTerminal に生の U+202E が残っています: %q", got)
	}
}

func TestForDiag(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := ForDiag(long)
	if len([]rune(got)) > 60 {
		t.Errorf("ForDiag が切り詰めていません: 長さ %d", len([]rune(got)))
	}
	if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
		t.Errorf("ForDiag が引用符で囲んでいません: %q", got)
	}
}

func TestIsValidID(t *testing.T) {
	valid := []string{"a", "A0", "node-1", "api_server", "x" + strings.Repeat("y", 63)}
	for _, s := range valid {
		if !IsValidID(s) {
			t.Errorf("IsValidID(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"", "-a", "_a", "a b", "a\"b", `a" onclick="x`, "../x", "a/b",
		"日本語", "a" + strings.Repeat("b", 64), "a<b", "a:b",
	}
	for _, s := range invalid {
		if IsValidID(s) {
			t.Errorf("IsValidID(%q) = true, want false", s)
		}
	}
}
