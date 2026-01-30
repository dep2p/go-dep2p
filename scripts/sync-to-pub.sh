#!/usr/bin/env bash
#
# DeP2P 发布同步脚本
#
# 用法:
#   ./scripts/sync-to-pub.sh [目标目录]
#
# 示例:
#   ./scripts/sync-to-pub.sh                          # 同步到默认目录 ../_dep2p
#   ./scripts/sync-to-pub.sh /path/to/pub             # 同步到指定目录
#   ./scripts/sync-to-pub.sh ~/projects/dep2p-release # 同步到自定义目录
#

set -e

# ═══════════════════════════════════════════════════════════════════════════════
# 帮助信息
# ═══════════════════════════════════════════════════════════════════════════════

show_help() {
    cat << EOF
DeP2P 发布同步脚本

用法:
  ./scripts/sync-to-pub.sh [选项] [目标目录]

选项:
  -h, --help    显示帮助信息
  -n, --dry-run 仅预览，不实际同步

示例:
  ./scripts/sync-to-pub.sh                          # 同步到默认目录 ../_dep2p
  ./scripts/sync-to-pub.sh /path/to/pub             # 同步到指定目录
  ./scripts/sync-to-pub.sh -n                       # 预览同步（不实际执行）

配置文件:
  design/04_delivery/release/publishing/sync.config.yml

EOF
    exit 0
}

# ═══════════════════════════════════════════════════════════════════════════════
# 参数解析
# ═══════════════════════════════════════════════════════════════════════════════

DRY_RUN=""
PUB_DIR=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            ;;
        -n|--dry-run)
            DRY_RUN="--dry-run"
            shift
            ;;
        -*)
            echo "错误: 未知选项 $1" >&2
            echo "使用 --help 查看帮助" >&2
            exit 1
            ;;
        *)
            PUB_DIR="$1"
            shift
            ;;
    esac
done

# ═══════════════════════════════════════════════════════════════════════════════
# 配置
# ═══════════════════════════════════════════════════════════════════════════════

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="$ROOT_DIR/design/04_delivery/release/publishing/sync.config.yml"
DEV_DIR="$ROOT_DIR"

# 目标目录：优先使用命令行参数，否则使用默认值
if [[ -z "$PUB_DIR" ]]; then
    PUB_DIR="$ROOT_DIR/../_dep2p"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# 检查
# ═══════════════════════════════════════════════════════════════════════════════

if [[ ! -f "$CONFIG" ]]; then
    echo "错误: 未找到配置文件: $CONFIG" >&2
    exit 1
fi

# ═══════════════════════════════════════════════════════════════════════════════
# 排除规则（从配置文件读取）
# ═══════════════════════════════════════════════════════════════════════════════

EXCLUDE_ARGS=()
while IFS= read -r line; do
    # 跳过注释和空行
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// }" ]] && continue
    
    # 提取 exclude 项
    exclude_item=$(echo "$line" | sed 's/^[[:space:]]*-[[:space:]]*//' | tr -d '"' | sed 's/[[:space:]]*#.*$//')
    if [[ -n "$exclude_item" ]]; then
        EXCLUDE_ARGS+=("--exclude=$exclude_item")
    fi
done < <(awk '/^exclude:/{flag=1; next} /^[^ ]/ && flag{exit} flag' "$CONFIG")

# ═══════════════════════════════════════════════════════════════════════════════
# 执行同步
# ═══════════════════════════════════════════════════════════════════════════════

echo "══════════════════════════════════════════"
echo "DeP2P 发布同步"
echo "══════════════════════════════════════════"
echo "源目录: $DEV_DIR"
echo "目标:   $PUB_DIR"
if [[ -n "$DRY_RUN" ]]; then
    echo "模式:   预览（dry-run）"
fi
echo ""

# 创建目标目录（非 dry-run 模式）
if [[ -z "$DRY_RUN" ]]; then
    mkdir -p "$PUB_DIR"
fi

# 执行同步
echo "同步中..."
rsync -av --delete $DRY_RUN "${EXCLUDE_ARGS[@]}" "$DEV_DIR/" "$PUB_DIR/"

echo ""
echo "══════════════════════════════════════════"
if [[ -n "$DRY_RUN" ]]; then
    echo "✓ 预览完成（未实际同步）"
else
    echo "✓ 同步完成"
fi
echo "══════════════════════════════════════════"
echo ""
echo "目标目录: $PUB_DIR"
echo ""
if [[ -z "$DRY_RUN" ]]; then
    echo "下一步: 在 GitHub Desktop 中打开目标目录并提交"
fi
echo ""
