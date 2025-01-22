package httpping

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	logging "github.com/dep2p/log"
)

var log = logging.Logger("http-ping")

// pingSize 定义了ping消息体的大小为32字节
const pingSize = 32

// PingProtocolID 定义了HTTP ping协议的标识符
const PingProtocolID = "/http-ping/1"

// Ping 结构体实现了HTTP ping服务
type Ping struct{}

// 确保Ping实现了http.Handler接口
var _ http.Handler = Ping{}

// ServeHTTP 实现了http.Handler接口
// 参数:
//   - w: HTTP响应写入器
//   - r: HTTP请求对象
func (Ping) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 创建固定大小的字节数组用于存储请求体
	body := [pingSize]byte{}
	// 从请求体中读取指定大小的数据
	_, err := io.ReadFull(r.Body, body[:])
	if err != nil {
		// 如果读取失败，返回400错误
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// 设置响应头的内容类型为二进制流
	w.Header().Set("Content-Type", "application/octet-stream")
	// 设置响应体的长度
	w.Header().Set("Content-Length", strconv.Itoa(pingSize))
	// 将请求体原样写回响应
	w.Write(body[:])
}

// SendPing 通过HTTP发送ping请求
// 参数:
//   - client: HTTP客户端，应该针对Ping协议进行配置
//
// 返回值:
//   - error: 如果发生错误则返回错误信息
func SendPing(client http.Client) error {
	// 创建固定大小的字节数组用于存储随机数据
	body := [pingSize]byte{}
	// 生成随机数据
	_, err := io.ReadFull(rand.Reader, body[:])
	if err != nil {
		log.Debugf("生成随机数据失败: %v", err)
		return err
	}
	// 创建POST请求
	req, err := http.NewRequest("POST", "/", bytes.NewReader(body[:]))
	// 设置请求头的内容类型为二进制流
	req.Header.Set("Content-Type", "application/octet-stream")
	// 设置请求体的长度
	req.Header.Set("Content-Length", strconv.Itoa(pingSize))
	if err != nil {
		log.Debugf("创建HTTP请求失败: %v", err)
		return err
	}
	// 发送HTTP请求
	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("发送HTTP请求失败: %v", err)
		return err
	}
	// 确保响应体被关闭
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		log.Debugf("意外的状态码: %d", resp.StatusCode)
		return fmt.Errorf("意外的状态码: %d", resp.StatusCode)
	}

	// 创建固定大小的字节数组用于存储响应体
	rBody := [pingSize]byte{}
	// 读取响应体
	_, err = io.ReadFull(resp.Body, rBody[:])
	if err != nil {
		log.Debugf("读取响应体失败: %v", err)
		return err
	}

	// 比较请求体和响应体是否相同
	if !bytes.Equal(body[:], rBody[:]) {
		log.Debugf("ping消息体不匹配")
		return errors.New("ping消息体不匹配")
	}
	return nil
}
