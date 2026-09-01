// open.go は cli 層と internal/browser の橋渡し。
package cli

import "github.com/ZONO33LHD/archy-go/internal/browser"

type decision = browser.Decision

// decideOpen は自動オープン判定 (§11.1, §11.2)。
func decideOpen(cmdDefault bool, f cmdFlags, c *CLI) browser.Decision {
	return browser.Decide(cmdDefault, f.open, f.noOpen, c.Probe)
}

// verifyAndTarget は書き込んだファイルのすり替え検査と絶対パス化 (§11.3)。
func (c *CLI) verifyAndTarget(absPath string, wantBytes int64, wantSHA256 string) (string, error) {
	return browser.VerifyWritten(absPath, wantBytes, wantSHA256)
}
