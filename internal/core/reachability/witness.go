// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// WitnessService 入站见证服务
type WitnessService struct {
	coordinator *Coordinator

	ctx    context.Context
	cancel context.CancelFunc
}

// NewWitnessService 创建入站见证服务
func NewWitnessService(coordinator *Coordinator) *WitnessService {
	return &WitnessService{
		coordinator: coordinator,
	}
}

// Start 启动服务
func (s *WitnessService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	logger.Info("启动入站见证服务")
	return nil
}

// Stop 停止服务
func (s *WitnessService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// HandleWitnessReport 处理入站 witness 报告
func (s *WitnessService) HandleWitnessReport(
	report *interfaces.WitnessReport,
	remotePeerID string,
	remoteIP string,
) *interfaces.WitnessAck {
	ack := &interfaces.WitnessAck{
		Accepted: false,
	}

	if report == nil || remotePeerID == "" || remoteIP == "" {
		ack.Reason = "invalid input"
		return ack
	}

	// 上报见证给 Coordinator
	if s.coordinator != nil {
		s.coordinator.OnInboundWitness(report.DialedAddr, remotePeerID, remoteIP)
	}

	ack.Accepted = true
	return ack
}

// SendWitnessReport 发送见证报告（出站连接成功后调用）
func (s *WitnessService) SendWitnessReport(
	_ context.Context,
	stream io.ReadWriteCloser,
	dialedAddr string,
	targetID []byte,
) error {
	if stream == nil || dialedAddr == "" {
		return nil
	}

	// 构建 WitnessReport
	report := interfaces.WitnessReport{
		DialedAddr: dialedAddr,
		TargetID:   targetID,
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.Marshal(report)
	if err != nil {
		return err
	}

	// 发送报告
	if _, err := stream.Write(data); err != nil {
		return err
	}

	// 读取确认
	ackData := make([]byte, 1024)
	n, err := stream.Read(ackData)
	if err != nil && err != io.EOF {
		return err
	}

	var ack interfaces.WitnessAck
	if n > 0 {
		if err := json.Unmarshal(ackData[:n], &ack); err != nil {
			return err
		}
	}

	if !ack.Accepted {
		logger.Debug("witness 报告被拒绝", "reason", ack.Reason)
	} else {
		logger.Debug("witness 报告已接受", "dialedAddr", dialedAddr)
	}

	return nil
}

