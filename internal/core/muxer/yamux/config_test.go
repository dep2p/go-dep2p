package yamux

import (
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

func TestDefaultYamuxConfig(t *testing.T) {
	cfg := DefaultYamuxConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, 256, cfg.AcceptBacklog)
	assert.True(t, cfg.EnableKeepAlive)
	assert.Equal(t, 30*time.Second, cfg.KeepAliveInterval)
	assert.Equal(t, uint32(256*1024), cfg.MaxStreamWindowSize)
}

func TestConfigToYamux(t *testing.T) {
	muxerCfg := muxerif.Config{
		MaxStreams:          100,
		MaxStreamWindowSize: 512 * 1024,
		KeepAliveInterval:   60 * time.Second,
		KeepAliveTimeout:    15 * time.Second,
		EnableKeepAlive:     true,
	}

	yamuxCfg := ConfigToYamux(muxerCfg)
	assert.NotNil(t, yamuxCfg)
	assert.Equal(t, 100, yamuxCfg.AcceptBacklog)
	assert.Equal(t, uint32(512*1024), yamuxCfg.MaxStreamWindowSize)
	assert.Equal(t, 60*time.Second, yamuxCfg.KeepAliveInterval)
	assert.Equal(t, 15*time.Second, yamuxCfg.ConnectionWriteTimeout)
	assert.True(t, yamuxCfg.EnableKeepAlive)
}

func TestConfigToYamuxDefault(t *testing.T) {
	// 测试使用默认配置
	muxerCfg := muxerif.Config{} // 所有字段为零值

	yamuxCfg := ConfigToYamux(muxerCfg)
	assert.NotNil(t, yamuxCfg)
	// 应该使用默认值
	assert.Equal(t, 256, yamuxCfg.AcceptBacklog) // DefaultYamuxConfig 的默认值
}

func TestYamuxToConfig(t *testing.T) {
	yamuxCfg := &yamux.Config{
		AcceptBacklog:          200,
		MaxStreamWindowSize:    1024 * 1024,
		KeepAliveInterval:      45 * time.Second,
		ConnectionWriteTimeout: 20 * time.Second,
		EnableKeepAlive:        false,
	}

	muxerCfg := YamuxToConfig(yamuxCfg)
	assert.Equal(t, 200, muxerCfg.MaxStreams)
	assert.Equal(t, uint32(1024*1024), muxerCfg.MaxStreamWindowSize)
	assert.Equal(t, 45*time.Second, muxerCfg.KeepAliveInterval)
	assert.Equal(t, 20*time.Second, muxerCfg.KeepAliveTimeout)
	assert.False(t, muxerCfg.EnableKeepAlive)
}

func TestConfigRoundTrip(t *testing.T) {
	// 测试配置转换往返
	original := muxerif.Config{
		MaxStreams:          128,
		MaxStreamWindowSize: 512 * 1024,
		KeepAliveInterval:   30 * time.Second,
		KeepAliveTimeout:    10 * time.Second,
		EnableKeepAlive:     true,
	}

	yamuxCfg := ConfigToYamux(original)
	converted := YamuxToConfig(yamuxCfg)

	assert.Equal(t, original.MaxStreams, converted.MaxStreams)
	assert.Equal(t, original.MaxStreamWindowSize, converted.MaxStreamWindowSize)
	assert.Equal(t, original.KeepAliveInterval, converted.KeepAliveInterval)
	assert.Equal(t, original.KeepAliveTimeout, converted.KeepAliveTimeout)
	assert.Equal(t, original.EnableKeepAlive, converted.EnableKeepAlive)
}

