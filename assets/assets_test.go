package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// updateAssetHashes が真のとき、期待ハッシュファイルを現在のアセットから再生成する。
var updateAssetHashes = flag.Bool("update-assets", false, "アセットハッシュの golden を再生成する")

// hashedAssets はハッシュを固定する埋め込みアセット。
var hashedAssets = []string{"template.html", "viewer.css", "viewer.js"}

// TestAssetHashes は埋め込みアセットの SHA-256 を固定する (§14)。
//
// 期待値は testdata/asset-hashes.txt に保存し、-update-assets で再生成する。
// アセットを意図的に変更した場合は生成 HTML のゴールデンファイルの差分をレビューした上で
// `go test ./assets -run TestAssetHashes -update-assets` で更新する。
func TestAssetHashes(t *testing.T) {
	goldenPath := filepath.Join("testdata", "asset-hashes.txt")

	if *updateAssetHashes {
		var b strings.Builder
		names := append([]string(nil), hashedAssets...)
		sort.Strings(names)
		for _, name := range names {
			sum := sha256.Sum256(MustRead(name))
			b.WriteString(name + " " + hex.EncodeToString(sum[:]) + "\n")
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("期待ハッシュファイルがありません (-update-assets で生成): %v", err)
	}
	want := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 {
			want[parts[0]] = parts[1]
		}
	}
	for _, name := range hashedAssets {
		sum := sha256.Sum256(MustRead(name))
		got := hex.EncodeToString(sum[:])
		if want[name] != got {
			t.Errorf("%s の SHA-256 が期待値と一致しません。アセットが変更されています。\n意図的ならゴールデンファイルの差分をレビューの上 -update-assets で更新してください。", name)
		}
	}
}
