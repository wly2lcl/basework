# GoReleaser 预编译二进制，直接打包为最小镜像
FROM alpine:3.23

LABEL maintainer="basework"
LABEL description="AI Agent 框架和独立终端产品"

# GoReleaser 将预编译的 basework 二进制放入构建上下文
COPY basework /usr/local/bin/basework

WORKDIR /root

# 配置目录（用于挂载）
VOLUME ["/root/.config/basework"]

ENTRYPOINT ["basework"]
CMD ["tui"]
