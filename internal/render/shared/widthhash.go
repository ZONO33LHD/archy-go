// widthhash.go は文字幅レンジテーブルの同一性検証を提供する (§6, §14)。
package shared

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// WidthTableHash はレンジテーブル内容の SHA-256 (hex) を返す。
// テストと doctor が期待値と照合し、テーブルの無自覚な変更を検出する。
func WidthTableHash() string {
	h := sha256.New()
	var buf [8]byte
	for _, r := range wideRanges {
		binary.BigEndian.PutUint32(buf[0:4], uint32(r[0]))
		binary.BigEndian.PutUint32(buf[4:8], uint32(r[1]))
		h.Write(buf[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
