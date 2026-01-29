package muxer

import (
	"context"
	"io"
	"sync"
	"testing"
)

// ============================================================================
// 并发测试
// ============================================================================

// TestConcurrent_MultipleStreams 测试多个并发流
func TestConcurrent_MultipleStreams(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer server.Close()

	numStreams := 10
	var wg sync.WaitGroup
	wg.Add(numStreams * 2)

	// 客户端打开多个流
	for i := 0; i < numStreams; i++ {
		go func(id int) {
			defer wg.Done()

			stream, err := client.OpenStream(context.Background())
			if err != nil {
				t.Errorf("OpenStream() %d failed: %v", id, err)
				return
			}
			defer stream.Close()

			// 写入数据
			data := []byte("test")
			_, err = stream.Write(data)
			if err != nil {
				t.Errorf("Write() %d failed: %v", id, err)
			}
		}(i)
	}

	// 服务端接受多个流
	for i := 0; i < numStreams; i++ {
		go func(id int) {
			defer wg.Done()

			stream, err := server.AcceptStream()
			if err != nil {
				t.Errorf("AcceptStream() %d failed: %v", id, err)
				return
			}
			defer stream.Close()

			// 读取数据
			buf := make([]byte, 100)
			_, err = stream.Read(buf)
			if err != nil && err != io.EOF {
				t.Errorf("Read() %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrent_ParallelOpenStream 测试并发打开流
func TestConcurrent_ParallelOpenStream(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	numGoroutines := 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			stream, err := client.OpenStream(context.Background())
			if err != nil {
				errors <- err
				return
			}
			defer stream.Close()
		}()
	}

	wg.Wait()
	close(errors)

	// 检查错误
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent OpenStream() failed: %v", err)
		}
	}
}

// TestConcurrent_ParallelAcceptStream 测试并发接受流
func TestConcurrent_ParallelAcceptStream(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer server.Close()

	numStreams := 20
	var wg sync.WaitGroup
	wg.Add(numStreams * 2)

	// 客户端并发打开流
	for i := 0; i < numStreams; i++ {
		go func() {
			defer wg.Done()
			stream, err := client.OpenStream(context.Background())
			if err != nil {
				t.Errorf("OpenStream() failed: %v", err)
				return
			}
			defer stream.Close()
		}()
	}

	// 服务端并发接受流
	for i := 0; i < numStreams; i++ {
		go func() {
			defer wg.Done()
			stream, err := server.AcceptStream()
			if err != nil {
				t.Errorf("AcceptStream() failed: %v", err)
				return
			}
			defer stream.Close()
		}()
	}

	wg.Wait()
}

// TestConcurrent_RaceDetection 测试竞态条件
// 运行 go test -race 时检测竞态
func TestConcurrent_RaceDetection(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer server.Close()

	numOps := 20
	var wg sync.WaitGroup
	wg.Add(numOps * 2)

	// 并发打开流
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			stream, err := client.OpenStream(context.Background())
			if err != nil {
				return
			}
			defer stream.Close()

			// 写入一些数据
			stream.Write([]byte("test"))
		}()
	}

	// 并发接受流
	for i := 0; i < numOps; i++ {
		go func() {
			defer wg.Done()
			stream, err := server.AcceptStream()
			if err != nil {
				return
			}
			defer stream.Close()

			// 读取一些数据
			buf := make([]byte, 10)
			stream.Read(buf)
		}()
	}

	wg.Wait()
}
