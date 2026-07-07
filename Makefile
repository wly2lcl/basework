.PHONY: build test lint clean tidy

# 默认构建
build:
	go build -tags "sqlite memory" -o bin/basework ./cmd/basework

# 构建含可选模块版本
build-full:
	go build -tags memory -o bin/basework-full ./cmd/basework

# 运行测试
test:
	go test -tags "sqlite memory" ./...

# 运行测试（含可选模块）
test-all:
	go test -tags memory ./...

# lint
lint:
	golangci-lint run ./...

# 清理
clean:
	rm -rf bin/

# 整理依赖
tidy:
	go mod tidy

# 类型检查（不生成二进制）
vet:
	go vet ./...
