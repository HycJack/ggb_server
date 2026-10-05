# 配置变量
BINARY_NAME = ggb_server
MAIN_FILE   = ./cmd/main.go
GO          = go
PLATFORMS   = linux darwin windows
ARCH        = amd64
BIN_DIR     = bin

# 默认目标
.PHONY: default
default: build

# 单平台构建
.PHONY: build
build:
	$(GO) build -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_FILE)

# 多平台构建
.PHONY: build-all
build-all:
	@for platform in $(PLATFORMS); do \
		GOOS=$$platform GOARCH=$(ARCH) $(GO) build -o $(BIN_DIR)/$(BINARY_NAME)-$$platform $(MAIN_FILE); \
	done

# 清理
.PHONY: clean
clean:
	rm -rf $(BIN_DIR)

# 测试
.PHONY: test
test:
	$(GO) test -race -cover ./...

# 帮助文档
.PHONY: help
help:
	@echo "Usage:"
	@echo "  build      : 编译当前平台二进制"
	@echo "  build-all  : 编译多平台二进制"
	@echo "  test       : 运行单元测试"
	@echo "  clean      : 清理构建产物"