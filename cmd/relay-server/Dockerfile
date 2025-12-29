# DeP2P Relay Server Dockerfile
#
# 构建:
#   docker build -t dep2p-relay .
#
# 运行:
#   docker run -p 4001:4001 dep2p-relay

FROM golang:1.21-alpine AS builder

WORKDIR /app

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=0 GOOS=linux go build -o relay-server ./cmd/relay-server

# 运行时镜像
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/relay-server .

EXPOSE 4001

ENTRYPOINT ["./relay-server"]
CMD ["-port", "4001"]

