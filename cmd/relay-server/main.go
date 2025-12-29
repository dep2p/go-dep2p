// Package main 提供独立的 Relay 服务器
//
// Relay 服务器用于帮助 NAT 后的节点建立连接。
// 它充当中间人，转发两个无法直接连接的节点之间的流量。
//
// 使用方法:
//
//	go run main.go -port 4001
//
// 或使用 Docker:
//
//	docker build -t dep2p-relay .
//	docker run -p 4001:4001 dep2p-relay
//
// 参考: go-libp2p examples/relay
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("❌ 错误: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 解析命令行参数
	port := flag.Int("port", 4001, "监听端口")
	maxConns := flag.Int("max-conns", 1000, "最大连接数")
	maxReservations := flag.Int("max-reservations", 128, "最大预留数")
	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║            DeP2P Relay Server                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 捕获中断信号
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signalCh
		fmt.Printf("\n收到信号 %v，正在关闭...\n", sig)
		cancel()
	}()

	// 配置 Relay 服务器（使用 QUIC）
	opts := []dep2p.Option{
		dep2p.WithPreset(dep2p.PresetServer),
		dep2p.WithListenPort(*port),
		dep2p.WithRelay(true),
		dep2p.WithRelayServer(true),
		dep2p.WithConnectionLimits(*maxConns/2, *maxConns),
	}

	// 创建并启动节点
	node, err := dep2p.Start(ctx, opts...)
	if err != nil {
		return fmt.Errorf("启动 Relay 服务器失败: %w", err)
	}
	defer func() { _ = node.Close() }()

	// 打印服务器信息
	printServerInfo(node, *maxReservations)

	// 启动统计报告
	go reportStats(ctx, node)

	// 等待关闭
	<-ctx.Done()

	fmt.Println("\n正在关闭 Relay 服务器...")
	fmt.Println("再见! 👋")
	return nil
}

// printServerInfo 打印服务器信息
func printServerInfo(node dep2p.Endpoint, maxReservations int) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║                    服务器信息                         ║")
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Printf("║ 节点 ID: %s\n", node.ID())
	fmt.Println("║")
	fmt.Println("║ 监听地址:")
	for _, addr := range node.ListenAddrs() {
		fmt.Printf("║   • %s\n", addr)
	}
	fmt.Println("║")
	fmt.Printf("║ 最大预留数: %d\n", maxReservations)
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Println("客户端可以使用以下地址连接:")
	for _, addr := range node.ListenAddrs() {
		fmt.Printf("  %s/p2p/%s\n", addr, node.ID())
	}
	fmt.Println()

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Relay 服务器已启动，等待客户端连接...")
	fmt.Println("按 Ctrl+C 停止服务器")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// reportStats 定期报告统计信息
func reportStats(ctx context.Context, node dep2p.Endpoint) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			connCount := node.ConnectionCount()
			fmt.Printf("[Stats] 当前连接数: %d\n", connCount)
		}
	}
}

