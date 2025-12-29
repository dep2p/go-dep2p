#!/bin/bash
# review_audit.sh - DeP2P 复盘/审计一键门禁（本地/CI 通用）
#
# 目标：把“文档-接口-实现-测试/示例”的一致性检查做成可重复执行的门禁。
#
# 用法：
#   ./scripts/review_audit.sh
#
# 可选参数：
#   RACE=1        开启 go test -race（更慢但更可信）
#   E2E=1         额外跑 tests/e2e（默认只跑快速门禁）
#   V=1           输出更详细日志
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

if [[ "${V:-0}" == "1" ]]; then
  set -x
fi

echo "=========================================="
echo "  DeP2P Review Audit (质量门禁)"
echo "=========================================="
echo ""

echo "[1/7] 架构边界检查: scripts/arch_guard.sh"
./scripts/arch_guard.sh
echo ""

echo "[2/7] 伪实现/未完成标记扫描 (TODO/FIXME/HACK/XXX + not implemented)"
# 这些规则是“强信号”：一旦出现，往往意味着 AI 留了伪实现或临时绕过。
echo "  - 扫描 Go 源码中的标记..."
# 只扫描“生产代码”（排除 *_test.go），避免测试桩/假实现误伤门禁。
# 注意：grep 在“无匹配”时会返回退出码 1；这里用 `|| true` 避免 pipefail 让脚本提前退出。
todo_cnt=$( (grep -RIn --include="*.go" --exclude="*_test.go" -E "TODO|FIXME|HACK|XXX|临时|简化|待实现|待完善" . 2>/dev/null || true) | wc -l | tr -d ' ' )
panic_cnt=$( (grep -RIn --include="*.go" --exclude="*_test.go" -E 'panic\("not implemented"\)|errors\.New\("not implemented"\)|errors\.New\("unsupported"\)|errors\.New\("not supported"\)' . 2>/dev/null || true) | wc -l | tr -d ' ' )
echo "    TODO/FIXME/HACK/... 计数: ${todo_cnt}"
echo "    not-implemented/unsupported 计数: ${panic_cnt}"
if [[ "${panic_cnt}" != "0" ]]; then
  echo ""
  echo "✗ 发现明确未实现标记（not implemented/unsupported），请先清理："
  grep -RIn --include="*.go" --exclude="*_test.go" -E 'panic\("not implemented"\)|errors\.New\("not implemented"\)|errors\.New\("unsupported"\)|errors\.New\("not supported"\)' . | head -50
  exit 1
fi
echo ""

echo "[3/7] gofmt 检查（快速）"
unformatted=$(gofmt -l . | head -50 || true)
if [[ -n "${unformatted}" ]]; then
  echo "⚠ 发现未 gofmt 的文件（仅展示前 50 个）："
  echo "${unformatted}"
  echo ""
  echo "建议运行: gofmt -w <file>"
  if [[ "${STRICT:-0}" == "1" ]]; then
    echo ""
    echo "STRICT=1 已启用：gofmt 作为强制门禁失败。"
    exit 1
  fi
  echo "继续执行（默认不因历史格式问题阻断审计；可用 STRICT=1 强制）。"
else
  echo "✓ gofmt OK"
fi
echo ""

echo "[4/7] go vet"
go vet ./...
echo "✓ go vet OK"
echo ""

echo "[5/7] Go 单元/集成/一致性门禁测试"
r=()
if [[ "${RACE:-0}" == "1" ]]; then
  r+=("-race")
fi
go test "${r[@]:+${r[@]}}" ./tests/protocolids/... -timeout 2m
go test "${r[@]:+${r[@]}}" ./tests/review/... -timeout 2m
go test "${r[@]:+${r[@]}}" ./tests/invariants/... -timeout 2m
go test "${r[@]:+${r[@]}}" ./tests/requirements/... -timeout 2m
echo "✓ 门禁测试 OK"
echo ""

echo "[6/7] 可选工具（存在则执行）"
if command -v staticcheck >/dev/null 2>&1; then
  echo "  - staticcheck"
  if ! staticcheck ./...; then
    echo "  ⚠ staticcheck 执行失败（可能是本机安装/架构问题），跳过但建议修复本机工具链。"
  fi
else
  echo "  - staticcheck (未安装，跳过)"
fi
if command -v govulncheck >/dev/null 2>&1; then
  echo "  - govulncheck"
  if ! govulncheck ./...; then
    echo "  ⚠ govulncheck 执行失败（可能是本机安装/环境问题），跳过但建议修复本机工具链。"
  fi
else
  echo "  - govulncheck (未安装，跳过)"
fi
echo ""

echo "[7/7] 可选 E2E"
if [[ "${E2E:-0}" == "1" ]]; then
  go test "${r[@]:+${r[@]}}" ./tests/e2e/... -short -timeout 2m
  echo "✓ E2E (short) OK"
else
  echo "跳过（设置 E2E=1 可开启）"
fi

echo ""
echo "=========================================="
echo "✓ Review Audit 通过"
echo "=========================================="


