// 日志迁移工具：zap -> slog
//
// 使用方法：
//   go run ./scripts/migrate-logger <目录>
//   go run ./scripts/migrate-logger <目录> --dry-run
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var dryRun = false

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使用方法: go run ./scripts/migrate-logger <目录> [--dry-run]")
		os.Exit(1)
	}

	targetDir := os.Args[1]
	if len(os.Args) > 2 && os.Args[2] == "--dry-run" {
		dryRun = true
		fmt.Println("=== DRY RUN MODE ===")
	}

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		fmt.Printf("目录不存在: %s\n", targetDir)
		os.Exit(1)
	}

	fmt.Printf("=== 迁移目录: %s ===\n", targetDir)

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil // 跳过测试文件
		}

		return processFile(path)
	})

	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== 迁移完成 ===")
	fmt.Println("请运行 'go build ./...' 检查错误并手动修复")
}

func processFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	original := string(content)
	modified := original

	// 1. 替换 zap 日志字段
	modified = replaceZapFields(modified)

	// 2. 替换 logger 方法调用
	modified = replaceLoggerCalls(modified)

	// 3. 移除 zap.NewNop() 逻辑
	modified = removeNopLogger(modified)

	if modified != original {
		fmt.Printf("修改: %s\n", path)
		if !dryRun {
			return os.WriteFile(path, []byte(modified), 0644)
		}
	}

	return nil
}

// replaceZapFields 替换 zap 日志字段
func replaceZapFields(content string) string {
	// zap.String("key", value) -> "key", value
	patterns := []struct {
		pattern string
		replace string
	}{
		{`zap\.String\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Int\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Int64\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Uint64\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Uint32\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Float64\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Bool\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Duration\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Time\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Any\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
		{`zap\.Error\(([^)]+)\)`, `"err", $1`},
		// zap.Strings 需要特殊处理（包含闭包）
		{`zap\.Strings\("([^"]+)",\s*([^)]+)\)`, `"$1", $2`},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		content = re.ReplaceAllString(content, p.replace)
	}

	return content
}

// replaceLoggerCalls 替换 logger 方法调用
func replaceLoggerCalls(content string) string {
	// x.logger.Info -> log.Info
	patterns := []struct {
		pattern string
		replace string
	}{
		{`(\w+)\.logger\.Info\(`, `log.Info(`},
		{`(\w+)\.logger\.Debug\(`, `log.Debug(`},
		{`(\w+)\.logger\.Warn\(`, `log.Warn(`},
		{`(\w+)\.logger\.Error\(`, `log.Error(`},
		{`\blogger\.Info\(`, `log.Info(`},
		{`\blogger\.Debug\(`, `log.Debug(`},
		{`\blogger\.Warn\(`, `log.Warn(`},
		{`\blogger\.Error\(`, `log.Error(`},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		content = re.ReplaceAllString(content, p.replace)
	}

	return content
}

// removeNopLogger 移除 NopLogger 逻辑
func removeNopLogger(content string) string {
	// 移除多行 if logger == nil { logger = zap.NewNop() }
	lines := strings.Split(content, "\n")
	var result []string
	skip := 0

	for i, line := range lines {
		if skip > 0 {
			skip--
			continue
		}

		// 检测 if logger == nil { 模式
		if strings.Contains(line, "if logger == nil {") {
			// 检查下一行是否是 zap.NewNop()
			if i+1 < len(lines) && strings.Contains(lines[i+1], "zap.NewNop()") {
				// 跳过 if 和 } 之间的行
				skip = 2 // 跳过 if 行、赋值行、} 行
				continue
			}
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// addPackageLogger 添加包级别 logger（需要手动调用）
func addPackageLogger(content, subsystem string) string {
	// 在 import 后添加 var log = logger.Logger("subsystem")
	importEnd := strings.Index(content, ")")
	if importEnd == -1 {
		return content
	}

	loggerLine := fmt.Sprintf("\n\n// 包级别日志实例\nvar log = logger.Logger(%q)\n", subsystem)

	return content[:importEnd+1] + loggerLine + content[importEnd+1:]
}

// updateImports 更新 import（需要手动调用）
func updateImports(content string) string {
	// 移除 zap import
	content = strings.Replace(content, `"go.uber.org/zap"`, "", 1)

	// 添加 logger import（如果不存在）
	if !strings.Contains(content, "internal/util/logger") && strings.Contains(content, "log.") {
		// 在 import 块中添加
		importStart := strings.Index(content, "import (")
		if importStart != -1 {
			insertPos := importStart + len("import (")
			content = content[:insertPos] + "\n\t\"github.com/dep2p/go-dep2p/internal/util/logger\"" + content[insertPos:]
		}
	}

	return content
}

// printInstructions 打印手动修复指南
func printInstructions() {
	instructions := `
=== 手动修复指南 ===

1. 为每个包添加 logger 变量：
   var log = logger.Logger("subsystem")

2. 更新 import：
   - 移除: "go.uber.org/zap"
   - 添加: "github.com/dep2p/go-dep2p/internal/util/logger"

3. 移除结构体中的 logger 字段：
   logger *zap.Logger  // 删除此行

4. 移除构造函数中的 logger 参数：
   func NewXxx(..., logger *zap.Logger) -> func NewXxx(...)

5. 移除 logger 初始化逻辑：
   if logger == nil {
       logger = zap.NewNop()
   }

6. 修复 logger.With() 调用：
   logger.With(zap.String("k", v)) -> log.With("k", v)

7. 更新 module.go：
   - 移除 Logger *zap.Logger 依赖
   - 移除 ProvideServices 中的 logger 处理

8. 运行 go build ./... 并修复剩余错误
`
	fmt.Println(instructions)
}

