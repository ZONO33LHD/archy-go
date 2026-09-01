// args.go はフラグと位置引数の分離を行う。
// Go 標準の flag はフラグが位置引数の後ろに来ると解釈しないため、
// 位置を問わない小さなパーサを持つ (フラグ数が少ないので手書きで十分)。
package cli

import (
	"fmt"
	"strings"
)

// cmdFlags は全コマンド共通のフラグ集合。
type cmdFlags struct {
	json    bool
	quality string // "" = meta.quality_profile に従う
	open    bool   // --open 明示
	noOpen  bool   // --no-open 明示
}

// splitArgs は args をフラグと位置引数に分離する。
// フラグは "--" 始まりのみ。"-x.html" のような単一ダッシュ始まりは位置引数として扱い、
// "--" 以降はすべて位置引数とする ('-' 始まりの正当なファイル名を排除しないため)。
func splitArgs(args []string) (cmdFlags, []string, error) {
	var f cmdFlags
	var pos []string
	terminated := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if terminated {
			pos = append(pos, a)
			continue
		}
		switch {
		case a == "--":
			terminated = true
		case a == "--json":
			f.json = true
		case a == "--open":
			f.open = true
		case a == "--no-open":
			f.noOpen = true
		case a == "--quality":
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--quality に値がありません")
			}
			i++
			f.quality = args[i]
		case strings.HasPrefix(a, "--quality="):
			f.quality = strings.TrimPrefix(a, "--quality=")
		case strings.HasPrefix(a, "--"):
			return f, nil, fmt.Errorf("未知のフラグです: %s", a)
		default:
			pos = append(pos, a)
		}
	}
	if f.quality != "" && f.quality != "standard" && f.quality != "showcase" {
		return f, nil, fmt.Errorf("--quality は standard | showcase のみ指定できます")
	}
	if f.open && f.noOpen {
		return f, nil, fmt.Errorf("--open と --no-open は同時に指定できません")
	}
	return f, pos, nil
}
