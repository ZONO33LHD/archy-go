package shared

import (
	"math"
	"strings"
	"testing"
)

// TestTextUnits は §6 の units 規則の回帰テスト。
// 実行時に unicode パッケージの文字属性を参照していないことの固定にもなる:
// 凍結テーブル (Unicode 15.0) の判定であり、ツールチェーン更新で変わってはならない。
func TestTextUnits(t *testing.T) {
	tests := []struct {
		name, in string
		want     int
	}{
		{"空", "", 0},
		{"ASCII", "hello", 5},
		{"CJK漢字", "構成図", 6},
		{"ひらがな", "あいう", 6},
		{"カタカナ", "アーキ", 6},
		{"半角カナは1", "ｱｲｳ", 3}, // FF61-FF9F は Halfwidth (テーブル外)
		{"全角英数", "ＡＢ", 4},
		{"混在", "A図B", 4},
		{"ハングル", "한글", 4},
		{"絵文字(単独Wide)", "🚀", 2},
		{"VS16で2", "☁️", 2}, // U+2601 (Narrow) + FE0F → 2
		{"VS15で1", "☁︎", 1},
		{"VSのみは0", "️︎", 0},
		{"改行は最長行", "abcd\n図", 4},
		{"改行は最長行2", "ab\n構成図", 6},
		{"CJK記号", "、。", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TextUnits(tt.in); got != tt.want {
				t.Errorf("TextUnits(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestFitFont(t *testing.T) {
	// "HTTPS" (5 units) / box 120px: available=112, fitted = 112/3 = 37.3 → preferred 14 のまま
	font, ok := FitFont("HTTPS", 120, 14, 9)
	if !ok || font != 14 {
		t.Errorf("FitFont(HTTPS, 120) = %v %v, want 14 true", font, ok)
	}
	// 収まらない: 40 units の CJK / box 100px, available=92, fitted = 92/24 = 3.83 → min 9 でも溢れる
	long := strings.Repeat("図", 20)
	font, ok = FitFont(long, 100, 14, 9)
	if ok {
		t.Errorf("FitFont(long) = ok, want overflow (font=%v)", font)
	}
	if font != 9 {
		t.Errorf("溢れ時は最小フォントを返す: got %v", font)
	}
	// 0.1px 切り捨て: units=10, box 68 → available 60, fitted = 60/6 = 10 → 10
	font, ok = FitFont("abcdefghij", 68, 14, 6)
	if !ok || font != 10 {
		t.Errorf("FitFont(10units, 68) = %v %v, want 10 true", font, ok)
	}
}

func TestCoord(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{100.0, "100"},
		{100.50, "100.5"},
		{100.456, "100.46"},
		{math.Copysign(0, -1), "0"},
		{-0.004, "0"}, // 丸めで -0 になるケース
		{0.1 + 0.2, "0.3"},
		{-12.345, "-12.35"},
	}
	for _, tt := range tests {
		if got := Coord(tt.in); got != tt.want {
			t.Errorf("Coord(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFont(t *testing.T) {
	if got := Font(14.0); got != "14" {
		t.Errorf("Font(14) = %q", got)
	}
	if got := Font(9.5); got != "9.5" {
		t.Errorf("Font(9.5) = %q", got)
	}
}

// TestWidthTableHash はテーブルの無自覚な変更 (ツールチェーン追随など) を検出する。
// 意図的に更新する場合は cli.WidthTableHashExpected とゴールデンファイルを併せて更新する。
func TestWidthTableHash(t *testing.T) {
	const expected = "43c5ab8b026a1aac4bce312356a70e7f383d969e9a4fb5baed3b9ea449dbc6f7"
	if got := WidthTableHash(); got != expected {
		t.Errorf("WidthTableHash() = %s, want %s\n文字幅テーブルが変更されています。意図的な更新ならゴールデンファイルの差分をレビューした上で期待値を更新してください。", got, expected)
	}
}

// FuzzTextUnits: 不正 UTF-8・結合文字・サロゲート断片でもパニックしない (§13)。
func FuzzTextUnits(f *testing.F) {
	f.Add("hello")
	f.Add("構成図️")
	f.Add("\xff\xfe\xfd")
	f.Add("a︎️")
	f.Add("👨‍👩‍👧‍👦")      // ZWJ シーケンス
	f.Add("\xed\xa0\x80") // サロゲート断片の UTF-8 エンコード
	f.Fuzz(func(t *testing.T, s string) {
		u := TextUnits(s)
		if u < 0 {
			t.Errorf("TextUnits(%q) = %d (負値)", s, u)
		}
		if u > 2*len(s)+2 {
			t.Errorf("TextUnits(%q) = %d (バイト長の2倍を超過)", s, u)
		}
	})
}
