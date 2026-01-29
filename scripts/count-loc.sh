#!/bin/bash
# 统计 DeP2P 项目 Go 代码行数
# 优先使用 scc 工具：https://github.com/boyter/scc
# 如果 scc 不可用，则使用基础方法统计
#
# 安装 scc（可选，用于更详细的统计）：
#   brew install scc
#   或
#   go install github.com/boyter/scc/v3@latest
#
# 使用方法：
#   ./scripts/count-loc.sh

set -e

# 切换到项目根目录
cd "$(dirname "$0")/.."

# 检查 scc 是否安装（包括检查 GOPATH/bin）
SCC_CMD=""
if command -v scc &> /dev/null; then
    SCC_CMD="scc"
elif [ -f "$HOME/go/bin/scc" ]; then
    SCC_CMD="$HOME/go/bin/scc"
elif [ -n "$GOPATH" ] && [ -f "$GOPATH/bin/scc" ]; then
    SCC_CMD="$GOPATH/bin/scc"
fi

# 使用 scc 统计（如果可用）
if [ -n "$SCC_CMD" ]; then
    echo "正在使用 scc 统计 Go 代码行数..."
    "$SCC_CMD" --exclude-dir .git,.github,.gocache,.gomodcache,vendor,_dep2p \
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
            printf "\n📊 统计结果（使用 scc）：\n"
            printf "  Go 文件数: %d 个\n", files
            printf "  总行数: %d 行（约 %.1f 万行）\n", total, total/10000
            printf "  代码行: %d 行\n", code
            printf "  注释行: %d 行\n", comments
            printf "  空行: %d 行\n", blank
        }' && exit 0
fi

# 回退到基础统计方法
echo "scc 未安装，使用基础方法统计..."
echo "（提示：安装 scc 可获得更详细的统计，包括代码行、注释行、空行的区分）"
echo ""

# 使用 find 和 awk 统计
# 排除目录：.git, .github, .gocache, .gomodcache, vendor, _dep2p
TEMP_STATS=$(mktemp)
find . -name "*.go" -type f \
    -not -path "./.git/*" \
    -not -path "./.github/*" \
    -not -path "./vendor/*" \
    -not -path "./_dep2p/*" \
    -not -path "./.gocache/*" \
    -not -path "./.gomodcache/*" \
    2>/dev/null | while read -r file; do
    if [ -f "$file" ]; then
        wc -l < "$file" 2>/dev/null || echo "0"
    fi
done > "$TEMP_STATS"

# 统计文件数和总行数
FILES=$(find . -name "*.go" -type f \
    -not -path "./.git/*" \
    -not -path "./.github/*" \
    -not -path "./vendor/*" \
    -not -path "./_dep2p/*" \
    -not -path "./.gocache/*" \
    -not -path "./.gomodcache/*" \
    2>/dev/null | wc -l | tr -d ' ')

TOTAL=$(awk '{sum+=$1} END {print sum+0}' "$TEMP_STATS" 2>/dev/null || echo "0")
rm -f "$TEMP_STATS"

# 计算万行数
TOTAL_WAN=$(awk "BEGIN {printf \"%.1f\", $TOTAL/10000}" 2>/dev/null || echo "0")

# 输出结果
printf "\n📊 统计结果（基础方法）：\n"
printf "  Go 文件数: %d 个\n" "$FILES"
printf "  总行数: %d 行（约 %s 万行）\n" "$TOTAL" "$TOTAL_WAN"
echo ""
echo "💡 提示：安装 scc 工具可获得更详细的统计（代码行、注释行、空行）："
echo "   brew install scc"
echo "   或"
echo "   go install github.com/boyter/scc/v3@latest"

