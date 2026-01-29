package dht

import (
	"github.com/dep2p/go-dep2p/pkg/types"
)

// XORDistance 计算两个 NodeID 的 XOR 距离
// 返回距离的字节表示（大端序）
func XORDistance(a, b types.NodeID) []byte {
	aBytes := []byte(a)
	bBytes := []byte(b)

	// 确保长度一致
	maxLen := len(aBytes)
	if len(bBytes) > maxLen {
		maxLen = len(bBytes)
	}

	// 补齐到相同长度
	if len(aBytes) < maxLen {
		padded := make([]byte, maxLen)
		copy(padded[maxLen-len(aBytes):], aBytes)
		aBytes = padded
	}
	if len(bBytes) < maxLen {
		padded := make([]byte, maxLen)
		copy(padded[maxLen-len(bBytes):], bBytes)
		bBytes = padded
	}

	// XOR 运算
	distance := make([]byte, maxLen)
	for i := 0; i < maxLen; i++ {
		distance[i] = aBytes[i] ^ bBytes[i]
	}

	return distance
}

// CompareDistance 比较 a 和 b 到 target 的距离
// 返回：
//   -1 如果 dist(a, target) < dist(b, target)
//    0 如果 dist(a, target) == dist(b, target)
//    1 如果 dist(a, target) > dist(b, target)
func CompareDistance(a, b, target types.NodeID) int {
	distA := XORDistance(a, target)
	distB := XORDistance(b, target)

	for i := 0; i < len(distA) && i < len(distB); i++ {
		if distA[i] < distB[i] {
			return -1
		}
		if distA[i] > distB[i] {
			return 1
		}
	}

	// 长度不同的情况
	if len(distA) < len(distB) {
		return -1
	}
	if len(distA) > len(distB) {
		return 1
	}

	return 0
}

// CommonPrefixLen 计算两个 NodeID 的共同前缀长度（按位计数）
func CommonPrefixLen(a, b types.NodeID) int {
	distance := XORDistance(a, b)

	// 计算前导零位数
	zeroBits := 0
	for _, b := range distance {
		if b == 0 {
			zeroBits += 8
		} else {
			// 计算该字节的前导零位数
			for mask := byte(0x80); mask > 0; mask >>= 1 {
				if b&mask != 0 {
					return zeroBits
				}
				zeroBits++
			}
			return zeroBits
		}
	}

	return zeroBits
}

// BucketIndex 计算 NodeID 应该放入哪个 K-Bucket
// 返回 K-Bucket 索引（0-255）
func BucketIndex(local, remote types.NodeID) int {
	cpl := CommonPrefixLen(local, remote)
	// K-Bucket 数量 = ID 长度（字节数）* 8
	maxBuckets := len([]byte(local)) * 8
	if cpl >= maxBuckets {
		return maxBuckets - 1
	}
	return cpl
}
