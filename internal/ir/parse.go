// parse.go は IR JSON の受け入れ検疫 (§10.1) を実装する。
//
// encoding/json/v2 を使う。v2 の既定で重複キーと不正 UTF-8 は拒否される。
// これらを緩める v1 互換オプションは渡してはならない。
// 自前で強制するのは: 入力サイズ上限、ネスト深度上限、未知フィールドの拒否、数値レンジ。
package ir

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"

	"encoding/json/jsontext"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/schema"
)

const (
	// MaxInputBytes は入力 JSON のサイズ上限 (既定 4MiB)。
	MaxInputBytes = 4 << 20
	// MaxDepth はネスト深度の上限 (既定 32)。
	MaxDepth = 32
)

// ReadLimited は r から入力を読み、サイズ上限を強制する。
func ReadLimited(r io.Reader) ([]byte, diag.List) {
	data, err := io.ReadAll(io.LimitReader(r, MaxInputBytes+1))
	if err != nil {
		return nil, diag.List{diag.Error("input/read", "",
			"入力の読み込みに失敗しました: "+err.Error())}
	}
	if len(data) > MaxInputBytes {
		return nil, diag.List{diag.Error("input/too-large", "",
			fmt.Sprintf("入力サイズが上限 %d バイトを超えています", MaxInputBytes),
			"IR を分割するか、不要な要素を削除してサイズを減らす")}
	}
	return data, nil
}

// Parse は入力バイト列を検疫し、スキーマ検証を経て型付き Document を返す。
//
// 手順:
//  1. jsontext.Decoder でトークン走査 — 深度上限・重複キー・不正 UTF-8・構文を検査
//  2. 汎用値 (any) へデコードし、生成されたスキーマバリデータで検証
//  3. 型付き Document へデコード (未知メンバー拒否は保険として重ねる)
//
// スキーマ検証に失敗した場合 Document は nil。診断はすべて返す (パニックさせない)。
func Parse(diagramType string, data []byte) (*Document, diag.List) {
	if len(data) > MaxInputBytes {
		return nil, diag.List{diag.Error("input/too-large", "",
			fmt.Sprintf("入力サイズが上限 %d バイトを超えています", MaxInputBytes))}
	}

	// 層0: トークン走査。深度は組み込みの上限を前提にせず自前で数える。
	if ds := scanTokens(data); len(ds) > 0 {
		return nil, ds
	}

	// 層1: スキーマ検証。v2 の既定 (重複キー拒否・不正 UTF-8 拒否) をそのまま使う。
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, diag.List{decodeErrorDiag(err)}
	}
	ds := schema.Validate(diagramType, generic)

	// 図種の宣言と CLI 引数の一致を検査する。
	if obj, ok := generic.(map[string]any); ok {
		if dt, ok := obj["diagram_type"].(string); ok && dt != diagramType {
			ds = append(ds, diag.Error("input/type-mismatch", "/diagram_type",
				fmt.Sprintf("diagram_type が CLI 引数と一致しません (IR: %q, CLI: %q)", dt, diagramType),
				"CLI 引数の図種を IR の diagram_type に合わせる",
				"IR の diagram_type を修正する"))
		}
	}
	if ds.HasErrors() {
		return nil, ds
	}

	var doc Document
	if err := json.Unmarshal(data, &doc, json.RejectUnknownMembers(true)); err != nil {
		ds = append(ds, decodeErrorDiag(err))
		return nil, ds
	}
	return &doc, ds
}

// scanTokens は jsontext.Decoder で全トークンを走査し、深度と構文の健全性を検査する。
func scanTokens(data []byte) diag.List {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.ReadToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return diag.List{decodeErrorDiag(err)}
		}
		if dec.StackDepth() > MaxDepth {
			return diag.List{diag.Error("input/too-deep", string(dec.StackPointer()),
				fmt.Sprintf("ネスト深度が上限 %d を超えています", MaxDepth),
				"IR の構造を平坦化する")}
		}
	}
}

// decodeErrorDiag はデコードエラーを診断に変換する。
// エラーメッセージに入力文字列の断片が含まれ得るため、そのまま埋め込まず分類する。
func decodeErrorDiag(err error) diag.Diagnostic {
	msg := err.Error()
	switch {
	case errors.Is(err, jsontext.ErrDuplicateName) || strings.Contains(msg, "duplicate object member name"):
		return diag.Error("input/duplicate-key", pointerOf(err),
			"同一オブジェクト内にキーが重複しています",
			"重複したキーの一方を削除する")
	case strings.Contains(msg, "invalid UTF-8"):
		return diag.Error("input/invalid-utf8", pointerOf(err),
			"JSON 文字列内に不正な UTF-8 が含まれています",
			"文字列を正しい UTF-8 に修正する")
	case strings.Contains(msg, "unknown object member") || strings.Contains(msg, "unknown name"):
		return diag.Error("input/unknown-field", pointerOf(err),
			"未知のフィールドが含まれています",
			"フィールドを削除するか、スキーマで定義された名前に修正する")
	case strings.Contains(msg, "exceeds maximum") || strings.Contains(msg, "out of range"):
		return diag.Error("input/number-range", pointerOf(err),
			"数値が表現可能な範囲を超えています",
			"数値を現実的なレンジ (±1e6) に収める")
	default:
		return diag.Error("input/syntax", pointerOf(err),
			"JSON として解釈できません: "+clipASCII(msg, 160),
			"JSON の構文を修正する")
	}
}

// pointerOf はエラーが JSON Pointer を持っていれば取り出す。
func pointerOf(err error) string {
	var se *json.SemanticError
	if errors.As(err, &se) && se.JSONPointer != "" {
		return string(se.JSONPointer)
	}
	var te *jsontext.SyntacticError
	if errors.As(err, &te) && te.JSONPointer != "" {
		return string(te.JSONPointer)
	}
	return ""
}

// clipASCII はエラーメッセージを診断向けに切り詰め、制御文字を除去する。
// (ANSI エスケープ注入対策 §10.2。入力断片がエラー文に混ざる可能性があるため)
func clipASCII(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			b.WriteString("…")
			break
		}
		if r < 0x20 || r == 0x7F {
			b.WriteRune('�')
		} else {
			b.WriteRune(r)
		}
		n++
	}
	return b.String()
}
