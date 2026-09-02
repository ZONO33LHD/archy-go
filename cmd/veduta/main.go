// veduta — 検証可能なシステム構成図ジェネレータ。
//
// LLM に判断させ、コードは決定論的に描画・検証する。
// 同じ入力 JSON からは、いつ・どのマシンで実行してもバイト単位で同一の HTML が出る。
package main

import (
	"fmt"
	"os"

	"github.com/ZONO33LHD/archy-go/internal/cli"
)

// version はビルド時に -ldflags "-X main.version=..." で固定される。
// タイムスタンプ等の環境依存値は決して埋め込まない (§9)。
var version = "dev"

func main() {
	c, err := cli.NewSystem(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "veduta: %v\n", err)
		os.Exit(cli.ExitInternal)
	}
	os.Exit(c.Run(os.Args[1:]))
}
