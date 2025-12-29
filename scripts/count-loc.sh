#!/bin/bash
# 统计 DeP2P 项目 Go 代码行数
# 使用 scc 工具：https://github.com/boyter/scc
#
# 安装 scc：
#   brew install scc
#   或
#   go install github.com/boyter/scc/v3@latest
#
# 使用方法：
#   ./scripts/count-loc.sh

set -e

# 检查 scc 是否安装
if ! command -v scc &> /dev/null; then
    echo "错误: scc 未安装"
    echo ""
    echo "请先安装 scc："
    echo "  brew install scc"
    echo "  或"
    echo "  go install github.com/boyter/scc/v3@latest"
    exit 1
fi

# 切换到项目根目录
cd "$(dirname "$0")/.."

# 使用 scc 统计 Go 代码
# 排除：.git, .github, .gocache, .gomodcache, vendor, _dep2p
# 只统计 Go 文件
echo "正在统计 Go 代码行数..."
scc --exclude-dir .git,.github,.gocache,.gomodcache,vendor,_dep2p \
    --include-ext go \
    --no-cocomo \
    --no-complexity \
    --no-gitignore \
    . 2>/dev/null | grep "^Go" | awk '{
        # scc 输出格式：语言 | 文件数 | 总行数 | 注释行 | 空行 | 代码行
        files=$2
        total=$3
        comments=$4
        blank=$5
        code=$6
        printf "\n📊 统计结果：\n"
        printf "  Go 文件数: %d 个\n", files
        printf "  总行数: %d 行（约 %.1f 万行）\n", total, total/10000
        printf "  代码行: %d 行\n", code
        printf "  注释行: %d 行\n", comments
        printf "  空行: %d 行\n", blank
    }' || {
        echo "错误: 统计失败"
        exit 1
    }

