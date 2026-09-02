// output.go は診断・受領証の人間可読/機械可読出力を提供する。
package cli

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"io"
	"strconv"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/safe"
)

// writeJSON は機械可読出力を書く。マップキー順は Deterministic で固定する。
// broken pipe・ディスク枯渇などの書き込み失敗を呼び出し側へ伝播させ、
// 「書けていないのに成功終了」を防ぐ (短い書き込みも失敗として扱う)。
func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return err
	}
	b = append(b, '\n')
	n, err := w.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	return err
}

// printDiags は人間可読の診断出力。IR 由来の文字列が混ざり得るため
// 全行を safe.ForTerminal で無害化する (§10.2)。
// 書き込みエラーは errWriter が一元捕捉するため、ここでは戻り値を持たない。
func printDiags(w io.Writer, ds diag.List) {
	for _, d := range ds {
		loc := d.Pointer
		if loc == "" && d.Subject != nil {
			loc = d.Subject.Surface
			if d.Subject.ID != "" {
				loc += ":" + d.Subject.ID
			}
		}
		fmt.Fprintf(w, "%-7s %-36s %s\n", d.Severity, d.Code, safe.ForTerminal(d.Message))
		if loc != "" {
			fmt.Fprintf(w, "        at: %s\n", safe.ForTerminal(loc))
		}
		for _, fix := range d.SupportedFixes {
			fmt.Fprintf(w, "        fix: %s\n", safe.ForTerminal(fix))
		}
	}
}

// errWriter は最初の書き込みエラーを記録する io.Writer ラッパー。
// CLI の標準出力をこれで包み、コマンド終了後に Err を確認することで
// 「書けていないのに成功終了」(broken pipe・ディスク枯渇) を検出する。
type errWriter struct {
	w   io.Writer
	Err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.Err != nil {
		return 0, e.Err
	}
	n, err := e.w.Write(p)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		e.Err = err
	}
	return n, err
}

// sha256Hex はバイト列の SHA-256 (hex)。
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// shortHash は人間可読出力用の短縮ハッシュ (先頭 12 hex + …)。
func shortHash(hexStr string) string {
	if len(hexStr) <= 12 {
		return hexStr
	}
	return hexStr[:12] + "…"
}

// formatBytes は 3 桁区切りのバイト数表記 ("712,043 bytes")。
func formatBytes(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out) + " bytes"
	}
	return string(out) + " bytes"
}
