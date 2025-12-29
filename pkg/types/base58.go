// Package types 提供 Base58 编码/解码实现
//
// Base58 是 Bitcoin 风格的编码，避免了易混淆字符（0OIl）。
// 本实现不依赖外部库。
package types

import (
	"errors"
	"math/big"
)

// Base58 字母表（Bitcoin 风格）
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var (
	// ErrInvalidBase58Char 无效的 Base58 字符
	ErrInvalidBase58Char = errors.New("invalid base58 character")

	// ErrInvalidBase58Checksum 无效的 Base58 校验和
	ErrInvalidBase58Checksum = errors.New("invalid base58 checksum")

	// base58AlphabetMap 字符到值的映射
	base58AlphabetMap = func() map[rune]int64 {
		m := make(map[rune]int64)
		for i, c := range base58Alphabet {
			m[c] = int64(i)
		}
		return m
	}()

	bigRadix = big.NewInt(58)
	bigZero  = big.NewInt(0)
)

// Base58Encode 将字节切片编码为 Base58 字符串
func Base58Encode(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	// 计算前导零的数量
	leadingZeros := 0
	for _, b := range input {
		if b != 0 {
			break
		}
		leadingZeros++
	}

	// 将输入视为大整数
	x := new(big.Int).SetBytes(input)

	// 转换为 Base58
	result := make([]byte, 0, len(input)*136/100+1)
	mod := new(big.Int)

	for x.Cmp(bigZero) > 0 {
		x.DivMod(x, bigRadix, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// 添加前导 '1'（代表前导零字节）
	for i := 0; i < leadingZeros; i++ {
		result = append(result, '1')
	}

	// 反转结果
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// Base58Decode 将 Base58 字符串解码为字节切片
func Base58Decode(input string) ([]byte, error) {
	if len(input) == 0 {
		return nil, nil
	}

	// 计算前导 '1' 的数量（代表前导零字节）
	leadingOnes := 0
	for _, c := range input {
		if c != '1' {
			break
		}
		leadingOnes++
	}

	// 解码
	x := new(big.Int)
	for _, c := range input {
		val, ok := base58AlphabetMap[c]
		if !ok {
			return nil, ErrInvalidBase58Char
		}
		x.Mul(x, bigRadix)
		x.Add(x, big.NewInt(val))
	}

	// 转换为字节
	decoded := x.Bytes()

	// 添加前导零字节
	result := make([]byte, leadingOnes+len(decoded))
	copy(result[leadingOnes:], decoded)

	return result, nil
}

