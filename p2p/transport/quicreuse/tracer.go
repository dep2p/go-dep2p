package quicreuse

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	golog "github.com/dep2p/log"
	"github.com/klauspost/compress/zstd"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"
	"github.com/quic-go/quic-go/qlog"
)

// log 用于记录 quic 相关的日志
var log = golog.Logger("p2p-transport-quic")

// QLOGTracer 保存 qlog tracer 目录,如果启用了 qlogging(通过 QLOGDIR 环境变量启用)。
// 否则为空字符串。
var qlogTracerDir string

// init 初始化 qlogTracerDir
func init() {
	qlogTracerDir = os.Getenv("QLOGDIR")
}

// qloggerForDir 为指定目录创建一个连接跟踪器
// 参数:
//   - qlogDir: string qlog 目录路径
//   - p: logging.Perspective 连接视角(客户端/服务端)
//   - ci: quic.ConnectionID 连接 ID
//
// 返回值:
//   - *logging.ConnectionTracer 连接跟踪器对象
func qloggerForDir(qlogDir string, p logging.Perspective, ci quic.ConnectionID) *logging.ConnectionTracer {
	// 如果目录不存在则创建
	if err := os.MkdirAll(qlogDir, 0777); err != nil {
		log.Errorf("创建 QLOGDIR 失败: %s", err)
		return nil
	}
	return qlog.NewConnectionTracer(newQlogger(qlogDir, p, ci), p, ci)
}

// qlogger 将 qlog 事件记录到临时文件: .<name>.qlog.swp。
// 当关闭时,它会压缩临时文件并将其保存为 <name>.qlog.zst。
// 无法实时压缩,因为压缩算法保持大量内部状态,在并行运行数百个 QUIC 连接时很容易耗尽主机系统的内存。
type qlogger struct {
	f             *os.File // QLOGDIR/.log_xxx.qlog.swp
	filename      string   // QLOGDIR/log_xxx.qlog.zst
	*bufio.Writer          // 缓冲 f
}

// newQlogger 创建一个新的 qlog 记录器
// 参数:
//   - qlogDir: string qlog 目录路径
//   - role: logging.Perspective 连接角色(客户端/服务端)
//   - connID: quic.ConnectionID 连接 ID
//
// 返回值:
//   - io.WriteCloser qlog 记录器对象
func newQlogger(qlogDir string, role logging.Perspective, connID quic.ConnectionID) io.WriteCloser {
	t := time.Now().UTC().Format("2006-01-02T15-04-05.999999999UTC")
	r := "server"
	if role == logging.PerspectiveClient {
		r = "client"
	}
	finalFilename := fmt.Sprintf("%s%clog_%s_%s_%s.qlog.zst", qlogDir, os.PathSeparator, t, r, connID)
	filename := fmt.Sprintf("%s%c.log_%s_%s_%s.qlog.swp", qlogDir, os.PathSeparator, t, r, connID)
	f, err := os.Create(filename)
	if err != nil {
		log.Errorf("无法创建 qlog 文件 %s: %s", filename, err)
		return nil
	}
	return &qlogger{
		f:        f,
		filename: finalFilename,
		// 原始文件下载的 qlog 文件大小约为传输数据量的 2/3。
		// bufio.NewWriter 创建的缓冲区只有 4 kB,导致大量系统调用。
		Writer: bufio.NewWriterSize(f, 128<<10),
	}
}

// Close 关闭 qlog 记录器并压缩日志文件
// 返回值:
//   - error 关闭过程中的错误
func (l *qlogger) Close() error {
	defer os.Remove(l.f.Name())
	defer l.f.Close()
	if err := l.Writer.Flush(); err != nil {
		log.Debugf("刷新qlog缓冲区时出错: %s", err)
		return err
	}
	if _, err := l.f.Seek(0, io.SeekStart); err != nil { // 将读取位置设置到文件开头
		log.Debugf("将读取位置设置到文件开头时出错: %s", err)
		return err
	}
	f, err := os.Create(l.filename)
	if err != nil {
		log.Debugf("创建qlog文件时出错: %s", err)
		return err
	}
	defer f.Close()
	buf := bufio.NewWriterSize(f, 128<<10)
	c, err := zstd.NewWriter(buf, zstd.WithEncoderLevel(zstd.SpeedFastest), zstd.WithWindowSize(32*1024))
	if err != nil {
		log.Debugf("创建zstd编码器时出错: %s", err)
		return err
	}
	if _, err := io.Copy(c, l.f); err != nil {
		log.Debugf("复制qlog文件时出错: %s", err)
		return err
	}
	if err := c.Close(); err != nil {
		log.Debugf("关闭zstd编码器时出错: %s", err)
		return err
	}
	return buf.Flush()
}
