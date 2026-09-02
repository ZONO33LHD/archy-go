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

// フラグ名。コマンドごとに受理するフラグを絞る。
const (
	flagJSON    = "--json"
	flagQuality = "--quality"
	flagOpen    = "--open"
	flagNoOpen  = "--no-open"
)

// splitArgs は args をフラグと位置引数に分離する。allowed はこのコマンドが受理するフラグ名の集合。
// フラグは "--" 始まりのみ。"-x.html" のような単一ダッシュ始まりは位置引数として扱い、
// "--" 以降はすべて位置引数とする ('-' 始まりの正当なファイル名を排除しないため)。
// 無関係なフラグ (例: doctor --quality) は使い方の誤りとして拒否する。
func splitArgs(args []string, allowed ...string) (cmdFlags, []string, error) {
	allow := map[string]bool{}
	for _, a := range allowed {
		allow[a] = true
	}
	ensure := func(name string) error {
		if !allow[name] {
			return fmt.Errorf("このコマンドでは %s は使用できません", name)
		}
		return nil
	}
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
		case a == flagJSON:
			if err := ensure(flagJSON); err != nil {
				return f, nil, err
			}
			f.json = true
		case a == flagOpen:
			if err := ensure(flagOpen); err != nil {
				return f, nil, err
			}
			f.open = true
		case a == flagNoOpen:
			if err := ensure(flagNoOpen); err != nil {
				return f, nil, err
			}
			f.noOpen = true
		case a == flagQuality:
			if err := ensure(flagQuality); err != nil {
				return f, nil, err
			}
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("--quality に値がありません")
			}
			i++
			f.quality = args[i]
		case strings.HasPrefix(a, "--quality="):
			if err := ensure(flagQuality); err != nil {
				return f, nil, err
			}
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
