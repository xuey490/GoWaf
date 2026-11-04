# GoWAF 构建与发布
#
# 关键约束：internal/web 用 go:embed 内嵌前端产物，而 go:embed 不能跨包引用
# 上级目录。前端构建产物已直接提交在 internal/web/dist —— clone 后 go build 即可，
# 不需要 node/pnpm。仅当要改界面时才需要 Frontend 源码（不在仓库内），
# 改完执行 make web 同步回 internal/web/dist。

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
	@echo "make build      - 构建本机二进制到 bin/$(APP)（直接用已提交的 internal/web/dist）"
	@echo "make build-all  - 交叉编译 linux/amd64 + linux/arm64 + windows/amd64"
	@echo "make package    - 交叉编译 linux/amd64 并打成 dist/gowaf-deploy-*.tar.gz"
	@echo "make web        - （需 Frontend 源码）构建前端并同步到 internal/web/dist"
	@echo "make sync-web   - （需 Frontend/dist）仅同步已有产物"
	@echo "make test       - 运行 go test"
	@echo "make test-adapter - Linux + Windows 适配器单测（Windows 项需要 pwsh）"
	@echo "make vet        - 运行 go vet"
	@echo "make fmt        - gofmt 全部源码"
	@echo "make run        - 用 configs/config.yaml 启动"
	@echo "make clean      - 清理构建产物"

# 前端源码不在仓库内时（视图很少改动，产物已直接提交），build 不依赖前端，
# 只要求 internal/web/dist 里有 index.html —— 没有就明确报错，而不是编译出空壳。
.PHONY: check-web
check-web:
	@if [ ! -f internal/web/dist/index.html ]; then \
		echo "错误：internal/web/dist/index.html 不存在。"; \
		echo "仓库应自带前端产物；若被误删，请取回 Frontend 源码后执行 make web 重新生成。"; \
		exit 1; \
	fi

.PHONY: web
web:
	@if [ ! -d Frontend ]; then \
		echo "错误：找不到 Frontend 源码目录（前端源码独立维护，不在本仓库内）。"; \
		echo "仅当你要修改控制台界面时才需要它；常规构建直接 make build 即可。"; \
		exit 1; \
	fi
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
build: check-web
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/gowaf

.PHONY: build-noweb
build-noweb:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/gowaf

.PHONY: build-all
build-all: check-web
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-amd64 ./cmd/gowaf
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-arm64 ./cmd/gowaf
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(APP)-windows-amd64.exe ./cmd/gowaf

.PHONY: test
test:
	go test ./...

.PHONY: test-adapter
test-adapter:
	bash deploy/test-nginx-adapter.sh
	@if command -v pwsh >/dev/null 2>&1; then pwsh -File deploy/test-windows-adapter.ps1; \
	else echo "skip Windows adapter tests (pwsh not found)"; fi

.PHONY: package
package:
	bash deploy/package.sh

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
