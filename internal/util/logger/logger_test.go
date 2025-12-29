package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetOutput(t *testing.T) {
	// 创建一个 buffer 来捕获日志输出
	buf := &bytes.Buffer{}
	
	// 设置输出到 buffer
	SetOutput(buf)
	
	// 创建一个 logger 并写入日志
	log := Logger("test")
	log.Info("test message", "key", "value")
	
	// 验证日志被写入 buffer
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected log message in buffer, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected key=value in buffer, got: %s", output)
	}
	if !strings.Contains(output, "subsystem=test") {
		t.Errorf("expected subsystem=test in buffer, got: %s", output)
	}
}

func TestSetOutput_ExistingLogger(t *testing.T) {
	// 创建一个 logger（输出到 stderr）
	log := Logger("test2")
	
	// 创建一个 buffer 并切换输出
	buf := &bytes.Buffer{}
	SetOutput(buf)
	
	// 使用已存在的 logger 写入日志
	log.Info("after switch", "key", "value")
	
	// 验证日志被写入 buffer（即使 logger 是在切换之前创建的）
	output := buf.String()
	if !strings.Contains(output, "after switch") {
		t.Errorf("expected log message in buffer, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected key=value in buffer, got: %s", output)
	}
}

