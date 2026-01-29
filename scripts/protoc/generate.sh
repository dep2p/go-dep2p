#!/bin/bash
# 生成 Protobuf Go 代码

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PROTO_DIR="$ROOT_DIR/pkg/proto"

echo "Generating protobuf Go code..."

# 检查 protoc 是否安装
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc is not installed"
    exit 1
fi

# 检查 protoc-gen-go 是否安装
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Error: protoc-gen-go is not installed"
    echo "Run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
    exit 1
fi

# 生成各个 proto 文件
for dir in "$PROTO_DIR"/*/; do
    if [ -d "$dir" ]; then
        for proto in "$dir"*.proto; do
            if [ -f "$proto" ]; then
                echo "Generating: $proto"
                protoc --go_out=. --go_opt=paths=source_relative "$proto"
            fi
        done
    fi
done

echo "Done!"
