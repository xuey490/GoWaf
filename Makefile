# GoWAF 构建与发布
#
# 关键约束：internal/web 用 go:embed 内嵌前端产物，而 go:embed 不能跨包引用
# 上级目录，因此构建前必须把 Frontend/dist 同步到 internal/web/dist。
# 所有发布命令都依赖 sync-web，不要跳过。

APP        := gowaf
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILT_AT   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.commit=$(COMMIT)' \
	-X 'main.buildTime=$(BUILT_AT)'

GOOS       ?= $(shell go env GOOS)
GOARCH     ?= $(shell go env GOARCH)

# 数据面不依赖 CGO，纯静态编译便于交叉编译与容器分发
export CGO_ENABLED = 0

.PHONY: help
help:
	@echo "make web        - 构建前端并同步到 internal/web/dist"
	@echo "make sync-web   - 仅同步已有的 Frontend/dist"
	@echo "make build      - 构建本机二进制到 bin/$(APP)"
	@echo "make build-all  - 交叉编译 linux/amd64 + linux/arm64 + windows/amd64"
	@echo "make test       - 运行 go test"
	@echo "make vet        - 运行 go vet"
	@echo "make fmt        - gofmt 全部源码"
	@echo "make run        - 用 configs/config.yaml 启动"
	@echo "make clean      - 清理构建产物"

.PHONY: web
web:
	cd Frontend && pnpm install --ignore-workspace --frozen-lockfile=false && pnpm build
	$(MAKE) sync-web

.PHONY: sync-web
sync-web:
	@if [ ! -d Frontend/dist ]; then \
		echo "Frontend/dist 不存在，请先执行 make web"; exit 1; \
	fi
	@rm -rf internal/web/dist
	@mkdir -p internal/web/dist
	@cp -R Frontend/dist/. internal/web/dist/
	@touch internal/web/dist/.gitkeep
	@echo "synced Frontend/dist -> internal/web/dist"

.PHONY: build
build: sync-web
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/gowaf

.PHONY: build-noweb
build-noweb:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/gowaf

.PHONY: build-all
build-all: sync-web
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-amd64 ./cmd/gowaf
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-arm64 ./cmd/gowaf
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP)-windows-amd64.exe ./cmd/gowaf

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w ./cmd ./internal

.PHONY: run
run: build-noweb
	./bin/$(APP) -c configs/config.yaml

.PHONY: clean
clean:
	rm -rf bin internal/web/dist
	mkdir -p internal/web/dist
	touch internal/web/dist/.gitkeep
