#!/usr/bin/env bash
#
# DeP2P 代码同步脚本
# 将开发目录内容复制到 _dep2p 发布目录
# 根据 release.config.yml 的配置排除不需要的文件
#

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="$ROOT_DIR/design/publishing/release.config.yml"
DEV_DIR="$ROOT_DIR"
PUB_DIR="$ROOT_DIR/../_dep2p"

# 检查配置文件是否存在
if [[ ! -f "$CONFIG" ]]; then
    echo "错误: 未找到配置文件: $CONFIG" >&2
    exit 1
fi

# 解析配置文件，提取 pub_path
# 如果 pub_path 是相对路径，则相对于工作区根目录
PUB_PATH=$(grep "^pub_path:" "$CONFIG" | awk '{print $2}' | tr -d '"')
if [[ -z "$PUB_PATH" ]]; then
    echo "错误: 配置文件中未找到 pub_path" >&2
    exit 1
fi

# 如果 pub_path 是相对路径，则相对于工作区根目录
if [[ "$PUB_PATH" != /* ]]; then
    PUB_DIR="$ROOT_DIR/../$PUB_PATH"
else
    PUB_DIR="$PUB_PATH"
fi

echo "=========================================="
echo "DeP2P 代码同步"
echo "=========================================="
echo "开发目录: $DEV_DIR"
echo "发布目录: $PUB_DIR"
echo "配置文件: $CONFIG"
echo ""

# 创建发布目录（如果不存在）
mkdir -p "$PUB_DIR"

# 先清理不应该存在的文件/目录（确保排除规则生效）
echo "清理不应存在的文件/目录..."
CLEANUP_ITEMS=(
    ".gocache"
    ".gomodcache"
    ".tmp"
    ".cursor"
    ".staticcheck.conf"
    ".golangci.yml"
    ".gitignore"
    "design/publishing"
    "_docs"
    "tests"
    "go-dep2p"
)
for item in "${CLEANUP_ITEMS[@]}"; do
    if [[ "$item" == *"*"* ]]; then
        # 通配符模式，使用 find 删除
        find "$PUB_DIR" -maxdepth 1 -name "$item" -type f -delete 2>/dev/null || true
    else
        # 普通路径，先尝试修改权限再删除（处理权限问题）
        if [[ -e "$PUB_DIR/$item" ]]; then
            chmod -R u+w "$PUB_DIR/$item" 2>/dev/null || true
            rm -rf "$PUB_DIR/$item" 2>/dev/null || true
        fi
    fi
done

# 构建 rsync exclude 参数
EXCLUDE_ARGS=()
while IFS= read -r line; do
    # 跳过注释和空行
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "${line// }" ]] && continue
    
    # 提取 exclude 项（去掉前导空格和破折号）
    exclude_item=$(echo "$line" | sed 's/^[[:space:]]*-[[:space:]]*//' | tr -d '"' | sed 's/[[:space:]]*#.*$//')
    if [[ -n "$exclude_item" ]]; then
        # rsync 排除规则：如果以 / 开头，则只匹配根目录；否则匹配任何位置
        # 为了更精确，我们统一使用不带前导 / 的格式
        EXCLUDE_ARGS+=("--exclude=$exclude_item")
    fi
done < <(awk '/^exclude:/{flag=1; next} /^[^ ]/ && flag{exit} flag' "$CONFIG")

# 显示排除规则
echo "排除规则:"
for arg in "${EXCLUDE_ARGS[@]}"; do
    echo "  ${arg#--exclude=}"
done
echo ""

# 使用 rsync 同步文件
echo "开始同步..."
rsync -av --delete \
    "${EXCLUDE_ARGS[@]}" \
    "$DEV_DIR/" \
    "$PUB_DIR/"

echo ""
echo "=========================================="
echo "同步完成！"
echo "=========================================="
echo "发布目录: $PUB_DIR"
echo ""
echo "下一步："
echo "  1. 检查发布目录内容"
echo "  2. 提交到 GitHub（如果需要）"
echo ""

