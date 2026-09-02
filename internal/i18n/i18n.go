// Package i18n はビューア UI のメッセージカタログを提供する。
//
// meta.locale はビューア UI の言語だけを制御し、図の本文は翻訳しない (§2)。
// locale は enum (en / ja / zh-CN) からの写像のみで、IR の値を直接 HTML に書かない (§10.3)。
package i18n

// Catalog は 1 ロケール分の UI 文字列。値はすべて静的でユーザー入力を含まない。
type Catalog struct {
	// Lang は <html lang> に書く BCP 47 タグ。
	Lang              string
	SearchPlaceholder string
	ThemeButton       string
	ResetButton       string
	NotesHeading      string
	ZoomGroup         string
	ZoomIn            string
	ZoomOut           string
	Stage             string
}

var catalogs = map[string]Catalog{
	"en": {
		Lang:              "en",
		SearchPlaceholder: "Search nodes",
		ThemeButton:       "Theme",
		ResetButton:       "Reset view",
		NotesHeading:      "Notes",
		ZoomGroup:         "Zoom",
		ZoomIn:            "Zoom in",
		ZoomOut:           "Zoom out",
		Stage:             "Diagram canvas (drag to pan, Ctrl+wheel or pinch to zoom, arrow keys to move)",
	},
	"ja": {
		Lang:              "ja",
		SearchPlaceholder: "ノードを検索",
		ThemeButton:       "テーマ",
		ResetButton:       "表示をリセット",
		NotesHeading:      "ノート",
		ZoomGroup:         "ズーム",
		ZoomIn:            "拡大",
		ZoomOut:           "縮小",
		Stage:             "図キャンバス (ドラッグでパン、Ctrl+ホイールまたはピンチでズーム、矢印キーで移動)",
	},
	"zh-CN": {
		Lang:              "zh-CN",
		SearchPlaceholder: "搜索节点",
		ThemeButton:       "主题",
		ResetButton:       "重置视图",
		NotesHeading:      "备注",
		ZoomGroup:         "缩放",
		ZoomIn:            "放大",
		ZoomOut:           "缩小",
		Stage:             "图画布 (拖动平移，Ctrl+滚轮或双指缩放，方向键移动)",
	},
}

// For は locale (検証済み enum) に対応するカタログを返す。
// 未知の値は en に落とす (許可リスト外は検証層が既に拒否している)。
func For(locale string) Catalog {
	if c, ok := catalogs[locale]; ok {
		return c
	}
	return catalogs["en"]
}
