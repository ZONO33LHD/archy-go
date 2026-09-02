.PHONY: tidy fmt fmt-check vet lint test test-cover test-race fuzz generate generate-check deps-check build golden-update

VERSION ?= dev

tidy:
	go mod tidy

fmt:
	gofmt -l -w .

# CI 用: 整形されていないファイルがあれば一覧を出して失敗する。
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt が必要なファイル:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	go vet ./...

lint: fmt-check vet

test:
	go test ./... -race -cover

test-cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

# ファジング (ローカル用の短時間実行。CI は ci.yml が同等を実行する)。
fuzz:
	go test -fuzz='FuzzParseIR$$' -fuzztime=30s ./internal/ir
	go test -fuzz='FuzzTextUnits$$' -fuzztime=30s ./internal/render/shared
	go test -fuzz='FuzzOutputPath$$' -fuzztime=30s ./internal/outpath
	go test -fuzz='FuzzRoute$$' -fuzztime=30s ./internal/geometry

# スキーマバリデータと文字幅テーブルの再生成。
generate:
	go generate ./...

# CI 用: 再生成して差分が出ないことを検証する (§14)。
generate-check: generate
	@if ! git diff --exit-code -- internal/schema/generated_validators.go internal/render/shared/widthtable.go; then \
		echo "go generate の生成物がコミットと一致しません"; exit 1; \
	fi

# 依存が標準ライブラリのみであることの機械的保証 (§14)。
deps-check:
	@deps="$$(go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./... | grep -v '^github.com/ZONO33LHD/archy-go' | grep -v '^$$' || true)"; \
	if [ -n "$$deps" ]; then \
		echo "標準ライブラリ以外の依存が検出されました:"; echo "$$deps"; exit 1; \
	fi
	@case ":$$GOEXPERIMENT:" in *nojsonv2*) \
		echo "GOEXPERIMENT=nojsonv2 は設定してはいけません (§10.1)"; exit 1;; esac

# ゴールデンファイルの再生成。差分は必ずレビューすること。
golden-update:
	go test ./internal/cli -run TestGolden -update

# 決定論的ビルド (§14)。
build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=$(VERSION)" -o bin/veduta ./cmd/veduta
	@shasum -a 256 bin/veduta
