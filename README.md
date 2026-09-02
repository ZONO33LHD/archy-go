# veduta — 検証可能なシステム構成図ジェネレータ

LLM に判断させ、コードは決定論的に描画・検証する CLI ツール。

```
自然言語 / Mermaid / コードベース
        ↓  ← LLM(エージェント)の担当。トポロジと構図を決める
   Typed JSON IR (JSON Schema で型付け)
        ↓  ← veduta の担当。ここから先に一切の非決定性はない
   検証 → SVG生成 → 単一HTMLファイル
```

LLM に座標計算をやらせると必ず破綻する。そこで LLM の出力をスキーマで縛り、
幾何と構図の妥当性はコード側で機械的に検証し、通らないものは落とす。
**「それらしいが壊れている図」を出さないことが最大の価値。**

## 非機能要件 (最重要)

1. **決定論性** — 同じ入力 JSON からは、いつ・どのマシンで実行してもバイト単位で同一の HTML が出る
2. **サードパーティ依存ゼロ** — 標準ライブラリのみ (`make deps-check` で機械的に保証)
3. **入力を信頼しない** — IR の JSON は「ネットワーク越しに届いたユーザー入力」として扱う

## 必要環境

Go 1.27.0 固定 (go.mod の完全バージョン `go 1.27.0` ディレクティブ)。
`toolchain` 行は go ディレクティブと一致すると `go` コマンドや `go mod tidy` が
冗長として削除するため書かない。CI は実ツールチェーンの `GOVERSION == go1.27.0` と
go ディレクティブを検証してバージョン固定を保証する。決定論的ビルドでは
`GOTOOLCHAIN=go1.27.0` を明示するとより確実。
バージョン固定は決定論性のため: Go 1.27 は `unicode` を Unicode 15→17 に更新し、
`compress/flate` の出力バイトが変わった。文字幅判定はコミット済みの凍結テーブル
(`internal/render/shared/widthtable.go`, Unicode 15.0 準拠) で行い、
実行時に `unicode` パッケージの文字属性を参照しない。

## 使い方

```bash
make build   # bin/veduta (CGO_ENABLED=0, -trimpath, SHA-256 添付)

veduta validate architecture input.json [--json] [--quality standard|showcase]
veduta render   architecture input.json output.html [--open|--no-open]
veduta deliver  architecture input.json output.html   # 最終受け入れ + レシート
veduta doctor                                          # 自己診断
veduta demo out-dir                                    # サンプル出力
veduta examples                                        # 同梱サンプル一覧
```

- 図種は第 1 段階では `architecture` のみ。`workflow` / `sequence` / `dataflow` / `lifecycle` は
  同じ `render.Renderer` インターフェースに乗せて追加する。
- `preview` / `compare` / `inspect` は第 2 段階 (未実装)。

### 終了コード

| code | 意味 |
|---|---|
| 0 | 成功 |
| 1 | 検証・受け入れ失敗 (診断が出力される) |
| 2 | 使い方の誤り |
| 3 | 内部エラー (パニックからの回復を含む) |

### 品質プロファイル

- `standard`: 基本チェック (ノード重なり / テキスト溢れ / 境界整合 / viewBox 収まり)。エラー 0 で合格
- `showcase`: 全チェック (ラベル衝突 / 経路貫通 / 経路品質 / ポート間隔 / 凡例整合を追加)。
  **エラー 0 かつ警告 0 で初めて合格**
- `deliver` はプロファイルにかかわらず全チェックを実行する (合否判定はプロファイル基準)

### deliver の契約 (§12)

1. 入力バイト列を同一ディレクトリの非公開スナップショット (0600) に凍結
2. スナップショットからレンダリング
3. 全チェック実行
4. すべて通れば `os.Root` 経由でアトミックにコミット。落ちれば既存出力を一切変更せず非ゼロ終了
5. レシート出力 (入力/HTML の SHA-256・バイト数・チェック一覧・opened)

### 自動オープン (§11)

| コマンド | 既定 |
|---|---|
| `render` | 開かない (修復ループでの多重起動防止) |
| `deliver` | 開く |
| その他 | 開かない |

`--open` / `--no-open` は既定に優先する。抑止条件 (fail-closed、`--open` でも解除されない):
`VEDUTA_NO_OPEN` / `CI` / 非 TTY / `TERM=dumb` / Linux で表示環境なし / SSH で表示環境なし / コンテナ内。
**有効化する環境変数は存在しない** (エージェントが設定できてしまうため §10.0)。
オープンの失敗は終了コードに影響せず、成果物パスは常に表示される。

## アーキテクチャ

```
cmd/veduta/                  CLI エントリポイント (薄い main)
internal/cli/                コマンド実装・パイプライン・レシート
internal/ir/                 IR 型定義 + 入口の検疫 (encoding/json/v2, §10.1)
internal/schema/             JSON Schema から生成されたバリデータ (go generate)
internal/validate/           参照整合性・サニタイズ・構図検証 (composition checks)
internal/geometry/           直交ルーティング・ポート計算・衝突判定
internal/render/             Layout モデル + HTML 組み立て (CSP ハッシュ)
internal/render/architecture/ architecture 図種のレンダラ
internal/render/shared/      テキスト計測 (凍結文字幅テーブル)・座標書式
internal/safe/               エスケープ・サニタイズの集約点 (分散禁止)
internal/outpath/            出力パス検査 + os.Root によるアトミック書き込み
internal/browser/            自動オープン (判定・起動・すり替え検査)
internal/diag/               診断オブジェクト (JSON Pointer + supportedFixes)
internal/i18n/               ビューア UI のメッセージカタログ (en/ja/zh-CN)
assets/                      template.html / viewer.css / viewer.js / schemas / examples (go:embed)
```

### 設計判断のメモ

- **スキーマ検証はコード生成** (§4): `assets/schemas/*.schema.json` が単一の真実の源。
  `go generate ./...` が `internal/schema/generated_validators.go` を再生成し、CI が差分ゼロを検証する。
- **CSP ハッシュは挿入内容そのものから計算する**: テンプレートとハッシュの
  ズレが構造的に起きない (生成 HTML に対するハッシュ一致テストで固定)。
- **座標は計算時点で丸める** (§9): `shared.Round2` / `shared.Coord` に一元化し、
  検証 (丸め前) と描画 (丸め後) のズレを作らない。
- **meta.output は検証のみ**: 実際の出力先は常に人間が打つ CLI 引数 (§10.0 の原則)。
- **オープナーには絶対パスを渡す**: `file://` URL は組み立てない (URL を使う機能を
  追加する場合は `url.URL` 型に任せる)。

## 開発

```bash
make test            # go test ./... -race -cover (セキュリティ回帰・決定論・ゴールデン込み)
make fuzz            # 4 ターゲット × 30 秒
make lint            # gofmt + go vet
make generate-check  # スキーマ・文字幅テーブルの同期検証
make deps-check      # 標準ライブラリのみ + GOEXPERIMENT 検証
make golden-update   # ゴールデンファイル再生成 (差分は必ずレビュー)
```

ゴールデンファイル (`internal/cli/testdata/`) がレンダラ変更時の唯一の防波堤。
ツールチェーン更新でゴールデンが変化した場合、**原因を特定するまで期待値を再生成しない**。

## やらないこと (§16 抜粋)

- 外部 Go モジュール依存 / WASM ビルド / 生成 HTML からの外部リソース読み込み
- バージョンチェック・テレメトリ等の自動的な外向き通信
- `eval` / `innerHTML` / `<foreignObject>` の混入 (CI の grep テストで強制)
- 環境変数によるセキュリティ機構の解除
- IR の内容によるツール挙動 (出力先・ネットワーク・サブプロセス) の制御
