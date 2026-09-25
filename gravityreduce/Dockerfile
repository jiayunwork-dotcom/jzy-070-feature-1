# syntax=docker/dockerfile:1
#
# 单阶段构建：只依赖一个本地已有的 Go 基础镜像即可一次构建成功，
# 依赖已通过 `go mod vendor` 固化到 ./vendor，构建过程不需要联网拉模块（GOPROXY=off）。
#
# 若本地镜像标签不同，可在构建时覆盖，例如：
#   docker build -t gravity-reduce --build-arg GO_IMAGE=golang:1.22.12-bookworm .
#   docker build -t gravity-reduce --build-arg GO_IMAGE=registry.example.local/library/golang:1.22 .
ARG GO_IMAGE=golang:1.22
FROM ${GO_IMAGE} AS build

WORKDIR /src

# 先拷模块定义与 vendor，利用构建缓存。
COPY go.mod go.sum ./
COPY vendor/ ./vendor/

# 再拷源码。
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# 静态编译：禁用 CGO，纯 Go 二进制，可直接放进任意基础镜像/用非 root 运行。
# -mod=vendor + GOPROXY=off 保证完全离线、只用 vendored 依赖。
ENV CGO_ENABLED=0 GOFLAGS=-mod=vendor GOPROXY=off
RUN go build -trimpath -ldflags="-s -w" -o /out/gravity-server ./cmd/server

# 运行期仍以同一本地 Go 基础镜像为根文件系统（避免再去拉取额外的运行时镜像）。
# 这样整套构建只需要本地存在这一个镜像。
FROM ${GO_IMAGE}

# 非 root 运行。
RUN useradd --system --create-home --uid 10001 appuser
COPY --from=build /out/gravity-server /usr/local/bin/gravity-server
USER appuser

ENV PORT=8080 GIN_MODE=release
EXPOSE 8080

# 容器内自检（镜像自带 wget；若精简镜像无 wget，可用 docker healthcheck 的 CMD-SHELL 覆盖）。
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/gravity-server"]
