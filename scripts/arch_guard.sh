#!/bin/bash
# arch_guard.sh - 架构守卫脚本
#
# 检查代码是否违反组件边界规则：
# 1. 非 internal/** 的生产代码禁止 import internal/core/**
# 2. internal/core/<x> 的生产代码禁止 import internal/core/<y>（x!=y）
# 3. *_test.go 文件豁免上述规则
#
# 用法：
#   ./scripts/arch_guard.sh
#
# 退出码：
#   0 - 无违规
#   1 - 发现违规

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=========================================="
echo "  架构守卫检查 (Architecture Guard)"
echo "=========================================="
echo ""

violations=0

# 规则 1: 检查根目录/examples/tests 等非 internal 的生产代码是否 import internal/core
echo "规则 1: 检查非 internal 代码对 internal/core 的引用..."

# 查找非 internal 目录下的 .go 文件（排除 _test.go）
while IFS= read -r -d '' file; do
    # 跳过测试文件
    if [[ "$file" == *"_test.go" ]]; then
        continue
    fi

    # 检查是否 import internal/core
    if grep -q '"github.com/dep2p/go-dep2p/internal/core/' "$file" 2>/dev/null; then
        echo -e "${RED}违规${NC}: $file"
        grep -n '"github.com/dep2p/go-dep2p/internal/core/' "$file" | head -5
        ((violations++)) || true
    fi
done < <(find "$PROJECT_ROOT" -type f -name "*.go" -not -path "*/internal/*" -not -path "*/.git/*" -print0 2>/dev/null)

echo ""

# 规则 2: 检查 internal/core/<x> 的生产代码是否 import internal/core/<y>（x!=y）
echo "规则 2: 检查 internal/core 组件间的跨依赖..."

# 获取所有 internal/core 下的一级目录（组件）
if [ -d "$PROJECT_ROOT/internal/core" ]; then
    for component_dir in "$PROJECT_ROOT/internal/core"/*/; do
        if [ ! -d "$component_dir" ]; then
            continue
        fi
        
        component_name=$(basename "$component_dir")
        
        # 遍历该组件下的所有 .go 文件（排除 _test.go）
        while IFS= read -r -d '' file; do
            # 跳过测试文件
            if [[ "$file" == *"_test.go" ]]; then
                continue
            fi
            
            # 检查是否 import 其他 internal/core 组件
            while IFS= read -r import_line; do
                # 提取被 import 的组件名
                # 格式: "github.com/dep2p/go-dep2p/internal/core/<other_component>/..."
                imported_component=$(echo "$import_line" | sed -n 's|.*"github.com/dep2p/go-dep2p/internal/core/\([^/"]*\).*|\1|p')
                
                if [ -n "$imported_component" ] && [ "$imported_component" != "$component_name" ]; then
                    echo -e "${RED}违规${NC}: $file"
                    echo "  组件 '$component_name' 不应直接依赖 '$imported_component' 的实现"
                    echo "  $import_line"
                    ((violations++)) || true
                fi
            done < <(grep '"github.com/dep2p/go-dep2p/internal/core/' "$file" 2>/dev/null || true)
            
        done < <(find "$component_dir" -type f -name "*.go" -print0 2>/dev/null)
    done
fi

echo ""
echo "=========================================="

if [ $violations -eq 0 ]; then
    echo -e "${GREEN}✓ 架构检查通过，无违规${NC}"
    exit 0
else
    echo -e "${RED}✗ 发现 $violations 处架构违规${NC}"
    echo ""
    echo "修复建议："
    echo "  1. 组件间交互应通过 pkg/interfaces/* 接口"
    echo "  2. 通用实现可放到 pkg/* 公共层"
    echo "  3. 使用 Fx 依赖注入代替直接 import"
    echo ""
    echo "如需豁免（仅限测试），请确保文件名以 _test.go 结尾"
    exit 1
fi

