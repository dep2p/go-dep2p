package dep2p

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCountLinesOfCode(t *testing.T) {
	totalLines := 0

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			lines := strings.Count(string(content), "\n") + 1
			totalLines += lines
		}

		return nil
	})

	if err != nil {
		t.Fatalf("遍历目录时出错: %v", err)
	}

	t.Logf("当前项目总共有 %d 行 Go 代码", totalLines)
}

func TestPrintProjectStructure2(t *testing.T) {
	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}

	fmt.Printf("\n项目文件结构：\n")

	// 忽略特定文件和目录
	ignoreItems := map[string]bool{
		".":            true,
		".git":         true,
		".DS_Store":    true,
		"node_modules": true,
		".idea":        true,
		".vscode":      true,
	}

	// 存储所有路径
	var paths []string

	// 遍历目录
	err = filepath.Walk(currentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 获取相对路径
		relPath, err := filepath.Rel(currentDir, path)
		if err != nil {
			return err
		}

		// 跳过忽略的项目
		if ignoreItems[info.Name()] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 添加非根目录的路径
		if relPath != "." {
			paths = append(paths, relPath)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("遍历目录失败: %v", err)
	}

	// 按字母顺序排序
	sort.Strings(paths)

	// 打印目录树
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// 计算深度
		depth := strings.Count(path, string(os.PathSeparator))

		// 构建缩进
		var indent string
		for d := 0; d < depth; d++ {
			if d == depth-1 {
				if isLastInDir(paths[i:], d) {
					indent += "└── "
				} else {
					indent += "├── "
				}
			} else {
				if isLastInDir(paths[i:], d) {
					indent += "    "
				} else {
					indent += "│   "
				}
			}
		}

		// 打印文件或目录
		if info.IsDir() {
			fmt.Printf("%s📁 %s\n", indent, info.Name())
		} else {
			fmt.Printf("%s📄 %s\n", indent, info.Name())
		}
	}
}

// 判断在指定深度是否是最后一个项目
func isLastInDir(paths []string, depth int) bool {
	if len(paths) == 0 {
		return true
	}

	currentParts := strings.Split(paths[0], string(os.PathSeparator))
	if depth >= len(currentParts) {
		return true
	}

	currentDir := currentParts[depth]

	for _, path := range paths[1:] {
		parts := strings.Split(path, string(os.PathSeparator))
		if depth >= len(parts) {
			continue
		}

		if parts[depth] == currentDir {
			return false
		}
	}

	return true
}
func TestPrintProjectStructure(t *testing.T) {
	// 创建输出文件
	file, err := os.Create("project_structure.md")
	if err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 写入标题
	file.WriteString("# Project Structure\n\n```\n")

	// 忽略特定文件和目录
	ignoreItems := map[string]bool{
		".":            true,
		".git":         true,
		".DS_Store":    true,
		"node_modules": true,
		".idea":        true,
		".vscode":      true,
	}

	// 存储所有路径
	var paths []string

	// 遍历目录
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 获取相对路径
		relPath, err := filepath.Rel(".", path)
		if err != nil {
			return err
		}

		// 跳过忽略的项目
		if ignoreItems[info.Name()] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 添加非根目录的路径
		if relPath != "." {
			paths = append(paths, relPath)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("遍历目录失败: %v", err)
	}

	// 按字母顺序排序
	sort.Strings(paths)

	// 打印目录树到文件
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// 计算深度
		depth := strings.Count(path, string(os.PathSeparator))

		// 构建缩进
		var indent string
		for d := 0; d < depth; d++ {
			if d == depth-1 {
				if isLastInDir(paths[i:], d) {
					indent += "└── "
				} else {
					indent += "├── "
				}
			} else {
				if isLastInDir(paths[i:], d) {
					indent += "    "
				} else {
					indent += "│   "
				}
			}
		}

		// 写入文件或目录
		if info.IsDir() {
			file.WriteString(fmt.Sprintf("%s📁 %s\n", indent, info.Name()))
		} else {
			file.WriteString(fmt.Sprintf("%s📄 %s\n", indent, info.Name()))
		}
	}

	// 写入结束标记
	file.WriteString("```\n")
}
