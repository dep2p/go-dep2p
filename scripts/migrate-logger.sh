#!/bin/bash

# 日志系统迁移脚本：zap -> slog
# 使用方法：
#   ./scripts/migrate-logger.sh <目录>           # 执行迁移
#   ./scripts/migrate-logger.sh <目录> --dry-run # 预览修改

TARGET_DIR="$1"
DRY_RUN=""

if [[ "$2" == "--dry-run" ]]; then
    DRY_RUN="1"
    echo "=== DRY RUN MODE ==="
fi

if [[ -z "$TARGET_DIR" ]]; then
    echo "Usage: $0 <directory> [--dry-run]"
    exit 1
fi

if [[ ! -d "$TARGET_DIR" ]]; then
    echo "Error: Directory '$TARGET_DIR' does not exist"
    exit 1
fi

# 从目录路径提取子系统名称
get_subsystem() {
    local dir="$1"
    echo "$dir" | sed 's|internal/core/||' | tr '/' '.'
}

SUBSYSTEM=$(get_subsystem "$TARGET_DIR")
echo "=== 迁移目录: $TARGET_DIR ==="
echo "=== 子系统名称: $SUBSYSTEM ==="
echo ""

# 统计
MODIFIED_FILES=0

# 替换函数 - macOS 兼容
do_sed() {
    local pattern="$1"
    local file="$2"
    if [[ -z "$DRY_RUN" ]]; then
        sed -i '' "$pattern" "$file" 2>/dev/null || true
    fi
}

echo "=== 处理源文件 ==="
find "$TARGET_DIR" -name "*.go" ! -name "*_test.go" | while read -r file; do
    echo "处理: $file"
    
    # 1. 替换 zap 字段
    if grep -q 'zap\.' "$file" 2>/dev/null; then
        echo "  - 替换 zap 字段"
        do_sed 's/zap\.String("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Int("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Int64("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Uint64("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Float64("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Error(\([^)]*\))/"err", \\1/g' "$file"
        do_sed 's/zap\.Duration("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Bool("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Strings("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Any("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Time("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        ((MODIFIED_FILES++)) || true
    fi
    
    # 2. 替换 logger 方法调用
    if grep -qE '\.logger\.(Info|Debug|Warn|Error)\(' "$file" 2>/dev/null; then
        echo "  - 替换 logger 方法"
        do_sed 's/\([a-z]\+\)\.logger\.Info(/log.Info(/g' "$file"
        do_sed 's/\([a-z]\+\)\.logger\.Debug(/log.Debug(/g' "$file"
        do_sed 's/\([a-z]\+\)\.logger\.Warn(/log.Warn(/g' "$file"
        do_sed 's/\([a-z]\+\)\.logger\.Error(/log.Error(/g' "$file"
    fi
    
    # 3. 替换独立 logger 调用
    if grep -qE '\blogger\.(Info|Debug|Warn|Error)\(' "$file" 2>/dev/null; then
        echo "  - 替换独立 logger"
        do_sed 's/\blogger\.Info(/log.Info(/g' "$file"
        do_sed 's/\blogger\.Debug(/log.Debug(/g' "$file"
        do_sed 's/\blogger\.Warn(/log.Warn(/g' "$file"
        do_sed 's/\blogger\.Error(/log.Error(/g' "$file"
    fi
done

echo ""
echo "=== 处理测试文件 ==="
find "$TARGET_DIR" -name "*_test.go" | while read -r file; do
    echo "处理: $file"
    
    # 替换 zap.NewNop() -> logger.Discard()
    if grep -q 'zap\.NewNop()' "$file" 2>/dev/null; then
        echo "  - 替换 zap.NewNop()"
        do_sed 's/zap\.NewNop()/logger.Discard()/g' "$file"
    fi
    
    # 替换 zap 字段
    if grep -q 'zap\.' "$file" 2>/dev/null; then
        echo "  - 替换 zap 字段"
        do_sed 's/zap\.String("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Int("\([^"]*\)", *\([^)]*\))/"\\1", \\2/g' "$file"
        do_sed 's/zap\.Error(\([^)]*\))/"err", \\1/g' "$file"
    fi
done

echo ""
echo "=== 迁移完成 ==="
echo ""
echo "=== 下一步（手动） ==="
echo "1. 在每个包添加: var log = logger.Logger(\"$SUBSYSTEM\")"
echo "2. 更新 import: 添加 logger，移除 zap"
echo "3. 移除结构体中的 logger 字段"
echo "4. 移除构造函数中的 logger 参数"
echo "5. 移除 if logger == nil { logger = zap.NewNop() }"
echo "6. 运行 go build ./... 检查错误"
