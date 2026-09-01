// Package ir は veduta の中間表現 (Typed JSON IR) の型定義と入口の検疫を提供する。
//
// IR の JSON は信頼できない入力である (§10.0)。LLM がコードベースや外部ドキュメントから
// 生成する以上、悪意ある文字列が流れ込む前提で「ネットワーク越しに届いたユーザー入力」として扱う。
package ir

// Document は architecture IR のルート。
type Document struct {
	SchemaVersion int          `json:"schema_version"`
	DiagramType   string       `json:"diagram_type"`
	Meta          Meta         `json:"meta"`
	Components    []Component  `json:"components"`
	Boundaries    []Boundary   `json:"boundaries,omitempty"`
	Connections   []Connection `json:"connections,omitempty"`
	Cards         []Card       `json:"cards,omitempty"`
}

// Meta は図全体のメタ情報。
//
// animation / visual_preset / views は第 1 段階では検証・保持のみ行う予約フィールドで、
// 現行のレンダラ・ビューアは視覚差を出さない (§15 の第 9・11 段階で解釈を実装予定)。
// スキーマは受理するが、これらを指定しても出力は classic・静的・全体表示のままである。
type Meta struct {
	Title string `json:"title"`
	// Output は IR が宣言する出力先。制約 (§10.4) は検証するが、
	// 実際の出力先は常に人間が打つ CLI 引数で決まる (§10.0 の原則)。
	Output         string    `json:"output,omitempty"`
	QualityProfile string    `json:"quality_profile,omitempty"`
	Animation      string    `json:"animation,omitempty"`     // 予約: トレースアニメーション (未実装)
	VisualPreset   string    `json:"visual_preset,omitempty"` // 予約: 視覚プリセット (未実装)
	Locale         string    `json:"locale,omitempty"`
	ViewBox        []float64 `json:"viewBox,omitempty"`
	Views          []View    `json:"views,omitempty"` // 予約: ガイド付きビュー (ビューアは未参照)
}

// View はガイド付きビューの定義。
type View struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Focus []string `json:"focus,omitempty"`
	Note  string   `json:"note,omitempty"`
}

// Component はノード 1 つ。pos / size は LLM が決めた座標をそのまま保持する。
type Component struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Label    string     `json:"label"`
	Sublabel string     `json:"sublabel,omitempty"`
	Tag      string     `json:"tag,omitempty"`
	Pos      [2]float64 `json:"pos"`
	Size     [2]float64 `json:"size"`
}

// Boundary はクラウド境界・セキュリティ境界などの囲み。
type Boundary struct {
	Kind  string   `json:"kind"`
	Label string   `json:"label"`
	Wraps []string `json:"wraps"`
}

// Connection はコンポーネント間の辺。
type Connection struct {
	ID       string       `json:"id"`
	From     string       `json:"from"`
	To       string       `json:"to"`
	Label    string       `json:"label,omitempty"`
	Variant  string       `json:"variant,omitempty"`
	FromSide string       `json:"fromSide,omitempty"`
	ToSide   string       `json:"toSide,omitempty"`
	Via      [][2]float64 `json:"via,omitempty"`
	LabelDy  *float64     `json:"labelDy,omitzero"`
	LabelAt  *float64     `json:"labelAt,omitzero"`
	ChannelX *float64     `json:"channelX,omitzero"`
	ChannelY *float64     `json:"channelY,omitzero"`
}

// Card は図に添える情報カード。
type Card struct {
	Dot   string   `json:"dot,omitempty"`
	Title string   `json:"title"`
	Items []string `json:"items"`
}

// QualityProfile は meta.quality_profile の解決値を返す (省略時 standard)。
func (d *Document) QualityProfile() string {
	if d.Meta.QualityProfile == "" {
		return "standard"
	}
	return d.Meta.QualityProfile
}

// VisualPreset は meta.visual_preset の解決値を返す (省略時 classic)。
func (d *Document) VisualPreset() string {
	if d.Meta.VisualPreset == "" {
		return "classic"
	}
	return d.Meta.VisualPreset
}

// Locale は meta.locale の解決値を返す (省略時 en)。
func (d *Document) Locale() string {
	if d.Meta.Locale == "" {
		return "en"
	}
	return d.Meta.Locale
}
