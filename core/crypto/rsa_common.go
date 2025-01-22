package crypto

import (
	"fmt"
	"os"
)

// WeakRsaKeyEnv 是一个环境变量,当设置时会将RSA密钥的最小所需位数降低到512位。
// 这应该仅在测试场景中使用。
const WeakRsaKeyEnv = "LIBP2P_ALLOW_WEAK_RSA_KEYS"

// MinRsaKeyBits 定义了RSA密钥的最小位数,默认为2048位
var MinRsaKeyBits = 2048

// maxRsaKeyBits 定义了RSA密钥的最大位数,为8192位
var maxRsaKeyBits = 8192

// ErrRsaKeyTooSmall 当尝试生成或解析小于MinRsaKeyBits位的RSA密钥时返回此错误
var ErrRsaKeyTooSmall error

// ErrRsaKeyTooBig 当尝试生成或解析大于maxRsaKeyBits位的RSA密钥时返回此错误
var ErrRsaKeyTooBig error = fmt.Errorf("RSA密钥必须小于等于%d位", maxRsaKeyBits)

// init 初始化函数
func init() {
	// 检查是否设置了WeakRsaKeyEnv环境变量
	if _, ok := os.LookupEnv(WeakRsaKeyEnv); ok {
		// 如果设置了环境变量,则将最小RSA密钥位数设置为512位
		MinRsaKeyBits = 512
	}

	// 初始化ErrRsaKeyTooSmall错误信息
	ErrRsaKeyTooSmall = fmt.Errorf("RSA密钥必须大于等于%d位才能使用", MinRsaKeyBits)
}
