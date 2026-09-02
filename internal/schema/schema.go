// Package schema は JSON Schema (assets/schemas/) から生成されたバリデータを提供する。
//
// スキーマが単一の真実の源であり、generated_validators.go は `go generate ./...` で再生成する。
// CI は再生成後に差分が出ないことを検証する。
package schema

//go:generate go run ./gen

import (
	"slices"
	"sort"
	"strings"

	"github.com/ZONO33LHD/archy-go/internal/diag"
)

// SupportedTypes は現在サポートされる図種 (第1段階は architecture のみ)。
var SupportedTypes = []string{"architecture"}

// IsSupportedType は図種がサポート対象かを返す。
func IsSupportedType(t string) bool {
	return slices.Contains(SupportedTypes, t)
}

// Validate は図種 diagramType のスキーマ検証を汎用デコード結果 v に対して実行する。
func Validate(diagramType string, v any) diag.List {
	switch diagramType {
	case "architecture":
		return ValidateArchitecture(v)
	default:
		return diag.List{diag.Error("schema/unsupported-type", "",
			"未サポートの図種です: "+diagramType,
			"図種を次のいずれかにする: "+strings.Join(SupportedTypes, " | "))}
	}
}

// addSchemaDiag は生成コードから呼ばれる診断追加ヘルパー。
// diag.Append 経由で件数上限を効かせ、巨大配列による診断爆発を防ぐ (§10.9)。
func addSchemaDiag(ds *diag.List, code, pointer, message string, fixes ...string) {
	diag.Append(ds, diag.Diagnostic{
		Code:           code,
		Severity:       diag.SeverityError,
		Message:        message,
		Pointer:        pointer,
		SupportedFixes: fixes,
	})
}

// sortedKeys はマップのキーをソートして返す (診断順の決定論性のため)。
func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ptrEscape は JSON Pointer (RFC 6901) のトークンエスケープ。
func ptrEscape(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}
