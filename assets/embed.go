// Package assets はテンプレート・スキーマ・サンプルをバイナリに埋め込む。
// 単一バイナリで完結することが配布上の要件 (§3)。
package assets

import "embed"

// FS は埋め込みアセット。
//
//go:embed template.html viewer.css viewer.js schemas examples
var FS embed.FS

// MustRead は埋め込みファイルを読む。埋め込みは build 時に確定しており、
// 失敗はビルド不整合 (= プログラミングエラー) なので panic でよい。
func MustRead(name string) []byte {
	b, err := FS.ReadFile(name)
	if err != nil {
		panic("assets: 埋め込みファイルがありません: " + name)
	}
	return b
}
