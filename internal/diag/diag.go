// Package diag は veduta 全体で使う診断オブジェクトを定義する。
//
// 診断は「LLM が自動修復するための機械可読な指摘」であり、
// どこが(Pointer / Subject)・何が(Code / Message)・どう直すか(SupportedFixes)を必ず持つ。
// IR 由来の文字列を Message に埋め込む場合は safe.ForDiag を通すこと。
package diag

// Severity は診断の重大度。
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Subject は診断の対象要素を示す。
type Subject struct {
	// Surface は対象の種類 (component / connection / boundary / card / view / meta / document)。
	Surface string `json:"surface"`
	// ID は対象要素の id。id を持たない要素では空。
	ID string `json:"id,omitempty"`
}

// Diagnostic は 1 件の診断。
type Diagnostic struct {
	// Code は "領域/種別" 形式の診断コード (例: "schema/type", "composition/label-collision")。
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	// Message は人間可読の説明。IR 由来の文字列は無害化・切り詰め済みであること。
	Message string `json:"message"`
	// Pointer は JSON Pointer (RFC 6901) 形式の位置 (例: "/components/3/type")。
	Pointer string   `json:"pointer,omitempty"`
	Subject *Subject `json:"subject,omitempty"`
	// Evidence は数値・座標などの根拠。キー順は出力時にソートされる。
	Evidence map[string]any `json:"evidence,omitempty"`
	// SupportedFixes は具体的な修正操作の列挙。無内容な文言を入れないこと。
	SupportedFixes []string `json:"supportedFixes,omitempty"`
}

// MaxDiagnostics は 1 回の検証で保持する診断件数の上限。
//
// スキーマ違反入力 (巨大配列) や上限内の最大要素数 (connections 2000 の総当たり = 約 200 万ペア)
// でも診断オブジェクトが無制限に増えないよう、決定論的にここで打ち切る (§10.9 のリソース枯渇対策)。
// LLM やユーザーが修復するのに数百件もあれば十分であり、超過分は capped で 1 件に集約する。
const MaxDiagnostics = 200

// List は診断の列。生成順 (= 決定論的な検査順) を保持する。
type List []Diagnostic

// AtCap は診断がこれ以上追加できない状態か。O(n²) 検査のループを決定論的に打ち切るのに使う。
//
// 閾値を「> MaxDiagnostics」にすることで、上限ちょうど (MaxDiagnostics 件) の次の Append が
// diagnostics/truncated を 1 件付け (合計 MaxDiagnostics+1)、その後のループが AtCap で止まる。
// これにより「打ち切られた」ことが診断として必ず利用者に伝わる。
func (l List) AtCap() bool {
	return len(l) > MaxDiagnostics
}

// Append は上限を尊重して診断を追加する。上限到達時は 1 度だけ truncated 診断を足し、
// 以降は無視する。これによりメモリ・出力サイズが入力サイズに対して爆発しない。
func Append(ds *List, d Diagnostic) {
	switch {
	case len(*ds) < MaxDiagnostics:
		*ds = append(*ds, d)
	case len(*ds) == MaxDiagnostics:
		*ds = append(*ds, Diagnostic{
			Code:     "diagnostics/truncated",
			Severity: SeverityError,
			Message:  "診断件数が上限に達したため以降を打ち切りました。まず表示済みの問題を修正してください。",
			SupportedFixes: []string{
				"表示された診断を修正してから再実行する",
				"要素数を減らして図を分割する",
			},
		})
	}
}

// HasErrors は severity=error が 1 件以上あるか。
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings は severity=warning が 1 件以上あるか。
func (l List) HasWarnings() bool {
	for _, d := range l {
		if d.Severity == SeverityWarning {
			return true
		}
	}
	return false
}

// Count は (エラー数, 警告数) を返す。
func (l List) Count() (errors, warnings int) {
	for _, d := range l {
		switch d.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		}
	}
	return
}

// Error は最小限の診断を error 相当として生成するヘルパー。
func Error(code, pointer, message string, fixes ...string) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityError, Message: message, Pointer: pointer, SupportedFixes: fixes}
}

// Warning は warning 診断を生成するヘルパー。
func Warning(code, pointer, message string, fixes ...string) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityWarning, Message: message, Pointer: pointer, SupportedFixes: fixes}
}
