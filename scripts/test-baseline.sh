#!/bin/bash
# DeP2P 单元测试基线报告生成脚本
# 用法: ./scripts/test-baseline.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 输出目录
OUTPUT_DIR="test-reports"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_DIR="${OUTPUT_DIR}/${TIMESTAMP}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  DeP2P 单元测试基线报告生成${NC}"
echo -e "${BLUE}  时间: $(date)${NC}"
echo -e "${BLUE}========================================${NC}"

# 创建输出目录
mkdir -p "${REPORT_DIR}"

# 步骤 1: 检查 Go 版本
echo -e "\n${YELLOW}[1/7] 检查环境...${NC}"
GO_VERSION=$(go version)
echo "Go 版本: ${GO_VERSION}"
echo "${GO_VERSION}" > "${REPORT_DIR}/go-version.txt"

# 步骤 2: 检查是否可以编译
echo -e "\n${YELLOW}[2/7] 检查编译...${NC}"
if go build ./... 2>&1 | tee "${REPORT_DIR}/build.log"; then
    echo -e "${GREEN}编译成功${NC}"
else
    echo -e "${RED}编译失败，请先修复编译错误${NC}"
    exit 1
fi

# 步骤 3: 运行快速测试
echo -e "\n${YELLOW}[3/7] 运行快速测试 (-short)...${NC}"
go test -short ./internal/... ./pkg/... 2>&1 | tee "${REPORT_DIR}/test-short.log" || true

# 统计结果
PASSED=$(grep -c "^ok" "${REPORT_DIR}/test-short.log" || echo "0")
FAILED=$(grep -c "^FAIL" "${REPORT_DIR}/test-short.log" || echo "0")
SKIPPED=$(grep -c "^\[no test" "${REPORT_DIR}/test-short.log" || echo "0")
echo -e "结果: ${GREEN}通过: ${PASSED}${NC}, ${RED}失败: ${FAILED}${NC}, 跳过: ${SKIPPED}"

# 步骤 4: 获取覆盖率
echo -e "\n${YELLOW}[4/7] 获取覆盖率...${NC}"
go test ./internal/... ./pkg/... -coverprofile="${REPORT_DIR}/coverage.out" -timeout 10m 2>&1 | tee "${REPORT_DIR}/test-coverage.log" || true

# 生成覆盖率摘要
if [ -f "${REPORT_DIR}/coverage.out" ]; then
    go tool cover -func="${REPORT_DIR}/coverage.out" > "${REPORT_DIR}/coverage-func.txt"
    TOTAL_COVERAGE=$(grep "total:" "${REPORT_DIR}/coverage-func.txt" | awk '{print $3}')
    echo -e "总覆盖率: ${GREEN}${TOTAL_COVERAGE}${NC}"
    
    # 生成 HTML 报告
    go tool cover -html="${REPORT_DIR}/coverage.out" -o "${REPORT_DIR}/coverage.html" 2>/dev/null || true
fi

# 步骤 5: 按模块统计覆盖率
echo -e "\n${YELLOW}[5/7] 按模块统计覆盖率...${NC}"
if [ -f "${REPORT_DIR}/coverage-func.txt" ]; then
    # 提取各模块覆盖率
    echo "| 模块 | 覆盖率 |" > "${REPORT_DIR}/coverage-by-module.md"
    echo "|------|--------|" >> "${REPORT_DIR}/coverage-by-module.md"
    
    # Core 层
    for module in identity security transport host swarm peerstore connmgr muxer eventbus metrics resourcemgr nat relay recovery reachability; do
        coverage=$(grep "internal/core/${module}" "${REPORT_DIR}/coverage-func.txt" 2>/dev/null | tail -1 | awk '{print $3}' || echo "N/A")
        if [ -n "$coverage" ] && [ "$coverage" != "N/A" ]; then
            echo "| core/${module} | ${coverage} |" >> "${REPORT_DIR}/coverage-by-module.md"
        fi
    done
    
    # Discovery 层
    for module in dht mdns bootstrap rendezvous dns coordinator; do
        coverage=$(grep "internal/discovery/${module}" "${REPORT_DIR}/coverage-func.txt" 2>/dev/null | tail -1 | awk '{print $3}' || echo "N/A")
        if [ -n "$coverage" ] && [ "$coverage" != "N/A" ]; then
            echo "| discovery/${module} | ${coverage} |" >> "${REPORT_DIR}/coverage-by-module.md"
        fi
    done
    
    # Realm 层
    for module in auth member connector gateway routing protocol; do
        coverage=$(grep "internal/realm/${module}" "${REPORT_DIR}/coverage-func.txt" 2>/dev/null | tail -1 | awk '{print $3}' || echo "N/A")
        if [ -n "$coverage" ] && [ "$coverage" != "N/A" ]; then
            echo "| realm/${module} | ${coverage} |" >> "${REPORT_DIR}/coverage-by-module.md"
        fi
    done
    
    # Protocol 层
    for module in pubsub streams messaging liveness; do
        coverage=$(grep "internal/protocol/${module}" "${REPORT_DIR}/coverage-func.txt" 2>/dev/null | tail -1 | awk '{print $3}' || echo "N/A")
        if [ -n "$coverage" ] && [ "$coverage" != "N/A" ]; then
            echo "| protocol/${module} | ${coverage} |" >> "${REPORT_DIR}/coverage-by-module.md"
        fi
    done
    
    cat "${REPORT_DIR}/coverage-by-module.md"
fi

# 步骤 6: 统计 t.Skip 使用
echo -e "\n${YELLOW}[6/7] 统计 t.Skip 使用...${NC}"
grep -rn "t\.Skip" internal/ --include="*_test.go" > "${REPORT_DIR}/t-skip-list.txt" 2>/dev/null || true
SKIP_COUNT=$(wc -l < "${REPORT_DIR}/t-skip-list.txt" | tr -d ' ')
echo -e "t.Skip 使用: ${YELLOW}${SKIP_COUNT} 处${NC}"

# 步骤 7: 统计测试函数
echo -e "\n${YELLOW}[7/7] 统计测试函数...${NC}"
TEST_FUNC_COUNT=$(grep -r "func Test" internal/ --include="*_test.go" | wc -l | tr -d ' ')
BENCH_FUNC_COUNT=$(grep -r "func Benchmark" internal/ --include="*_test.go" | wc -l | tr -d ' ')
TEST_FILE_COUNT=$(find internal/ -name "*_test.go" | wc -l | tr -d ' ')

echo "测试文件: ${TEST_FILE_COUNT}"
echo "测试函数: ${TEST_FUNC_COUNT}"
echo "基准函数: ${BENCH_FUNC_COUNT}"

# 生成报告
echo -e "\n${YELLOW}生成报告...${NC}"
cat > "${REPORT_DIR}/BASELINE_REPORT.md" << EOF
# DeP2P 单元测试基线报告

**生成日期**: $(date +%Y-%m-%d)  
**生成时间**: $(date +%H:%M:%S)  
**Go 版本**: ${GO_VERSION}  
**测试范围**: internal/, pkg/

---

## 一、总体统计

| 指标 | 数值 |
|------|------|
| 测试文件数 | ${TEST_FILE_COUNT} |
| 测试函数数 | ${TEST_FUNC_COUNT} |
| 基准函数数 | ${BENCH_FUNC_COUNT} |
| 通过包数 | ${PASSED} |
| 失败包数 | ${FAILED} |
| 总覆盖率 | ${TOTAL_COVERAGE:-N/A} |
| t.Skip 使用 | ${SKIP_COUNT} 处 |

## 二、按模块覆盖率

$(cat "${REPORT_DIR}/coverage-by-module.md" 2>/dev/null || echo "（覆盖率数据不可用）")

## 三、失败测试

\`\`\`
$(grep "^FAIL" "${REPORT_DIR}/test-short.log" 2>/dev/null || echo "无失败测试")
\`\`\`

## 四、t.Skip 清单

\`\`\`
$(head -20 "${REPORT_DIR}/t-skip-list.txt" 2>/dev/null || echo "无 t.Skip 使用")
\`\`\`

$(if [ "${SKIP_COUNT}" -gt 20 ]; then echo "... 共 ${SKIP_COUNT} 处（仅显示前 20 条）"; fi)

## 五、生成的文件

- \`test-short.log\` - 快速测试日志
- \`test-coverage.log\` - 覆盖率测试日志
- \`coverage.out\` - 覆盖率数据文件
- \`coverage.html\` - 覆盖率 HTML 报告
- \`coverage-func.txt\` - 函数级覆盖率
- \`coverage-by-module.md\` - 模块级覆盖率
- \`t-skip-list.txt\` - t.Skip 使用列表

---

**报告目录**: ${REPORT_DIR}
EOF

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}  报告生成完成！${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "报告目录: ${BLUE}${REPORT_DIR}${NC}"
echo -e "主报告: ${BLUE}${REPORT_DIR}/BASELINE_REPORT.md${NC}"
echo -e "覆盖率报告: ${BLUE}${REPORT_DIR}/coverage.html${NC}"
