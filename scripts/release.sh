#!/usr/bin/env bash
#
# DeP2P 发布入口脚本
# 调用统一的发布脚本 sync_and_release.sh
#

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="$ROOT_DIR/design/publishing/release.config.yml"

# 发布工具脚本路径（需要统一发布工具支持）
# 如果 RELEASE_TOOLS_DIR 环境变量已设置，则使用它
# 默认路径：假设与 WES 项目在同一工作区
RELEASE_TOOLS_DIR="${RELEASE_TOOLS_DIR:-$ROOT_DIR/../../WES/weisyn.git/_dev/tools/release-tools}"

# 检查发布工具脚本是否存在
if [[ ! -f "$RELEASE_TOOLS_DIR/sync_and_release.sh" ]]; then
    echo "错误: 未找到发布工具脚本: $RELEASE_TOOLS_DIR/sync_and_release.sh" >&2
    echo "提示: 可以设置 RELEASE_TOOLS_DIR 环境变量指定发布工具路径" >&2
    exit 1
fi

# 调用统一的发布脚本
"$RELEASE_TOOLS_DIR/sync_and_release.sh" --config "$CONFIG" "$@"

