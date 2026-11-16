.PHONY: build test lint clean tidy vet check-arch check-docs check-vuln test-vuln-script check-api test-api-script stats deps gen coverage progress docker-candidate

# 默认构建
build:
	go build -tags "sqlite memory" -o bin/basework ./cmd/basework

# 构建含可选模块版本
build-full:
	go build -tags "sqlite memory" -o bin/basework-full ./cmd/basework

# 运行测试
test:
	go test -tags "sqlite memory" ./...

# 运行测试（含可选模块）
test-all:
	go test -tags "sqlite memory" ./...

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
	go vet -tags "sqlite memory" ./...

# 架构边界检查：pkg 层不得依赖 internal 层
check-arch:
	go test ./tests/ -run TestArch -count=1 -v

# 文档校验：集中布局、ADR、链接、包契约、任务依赖与证据
check-docs:
	go run ./scripts/doccheck

# 默认与 sqlite,memory 构建都检查整个模块；漏洞和扫描错误均阻断。
check-vuln:
	bash ./scripts/check-vuln.sh source

# 检查扫描失败、错误输入与路径边界，不请求网络。
test-vuln-script:
	bash ./scripts/check-vuln_test.sh

# API 兼容门禁（ADR 0009）：稳定层 pkg 相对最近发布 tag 不得有不兼容变更。
check-api:
	bash ./scripts/check-api.sh

# 用临时 git 仓库验证门禁的放行/拦截/警告/基线缺失协议。
test-api-script:
	bash ./scripts/check-api_test.sh

# 生成代码统计（写入 docs/STATS.md）
# 刻意不带 -cover：`go test -cover` 对含并发的包不可复现（同一代码会抖动 ±0.5%），
# 抖动的数字进入受 CI「新鲜度门禁」的生成物会让门禁永久失败。需要覆盖率见 `make coverage`。
stats:
	go run ./scripts/docstats -no-cover

# 生成模块依赖图（写入 docs/DEPGRAPH.md）
deps:
	go run ./scripts/gendeps

# 查看测试覆盖率（仅打印，不写文件、不参与门禁——覆盖率本身不可复现）
coverage:
	go test -tags "sqlite memory" -cover ./...

# 刷新全部生成物（改完代码后运行，CI 会校验生成物是否最新）
gen: deps stats

# 从唯一任务看板读取进度，不另存完成率
progress:
	go run ./scripts/doccheck -progress

# 构建双架构候选 OCI 镜像；交叉编译产物只放在临时 Docker context 中。
docker-candidate:
	./scripts/build_docker_candidate.sh
