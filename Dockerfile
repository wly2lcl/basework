# 构建阶段
FROM golang:1.23-alpine AS builder

WORKDIR /build

# 安装依赖
RUN apk add --no-cache git

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建二进制文件
RUN CGO_ENABLED=0 GOOS=linux go build -tags "sqlite memory" -ldflags="-s -w" -o /basework ./cmd/basework

# 运行阶段
FROM gcr.io/distroless/static-debian11

LABEL maintainer="basework"
LABEL description="AI Agent 框架和独立终端产品"

# 复制二进制文件
COPY --from=builder /basework /usr/local/bin/basework

# 设置工作目录
WORKDIR /root

# 暴露配置目录（用于挂载）
VOLUME ["/root/.config/basework"]

# 默认命令
ENTRYPOINT ["basework"]
CMD ["tui"]
