package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestAssetHashes は埋め込みアセットの SHA-256 を固定する (§14)。
// アセットを意図的に変更した場合は、生成 HTML のゴールデンファイルの差分を
// レビューした上で、この期待値を併せて更新する。
func TestAssetHashes(t *testing.T) {
	expected := map[string]string{
		"template.html": "8f6d91749697b0ffd7240d14f957dd8a0e0c4e4ec84881d54da5684f9efd6af3",
		"viewer.css":    "9acca6ae42a407c59862a29e0c98d3370d6300bf8fdc2fb652085be732eda392",
		"viewer.js":     "1b756e1e1749b73e167c96a386b9f49d560245092f69baacd48517da39945564",
	}
	for name, want := range expected {
		sum := sha256.Sum256(MustRead(name))
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s の SHA-256 = %s, want %s\nアセットが変更されています。意図的ならゴールデンファイルと併せて期待値を更新してください。", name, got, want)
		}
	}
}
