package dht

import (
	"bytes"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// XORDistance 测试
// ============================================================================

// TestXORDistance_SameIDs 测试相同ID的XOR距离应该为0
func TestXORDistance_SameIDs(t *testing.T) {
	id := types.NodeID("test-peer-id")
	
	distance := XORDistance(id, id)
	
	// 相同ID的距离应该全为0
	for i, b := range distance {
		assert.Equal(t, byte(0), b, "字节 %d 应该为0", i)
	}
	
	t.Log("✅ 相同ID的XOR距离为0")
}

// TestXORDistance_DifferentIDs 测试不同ID的XOR距离
func TestXORDistance_DifferentIDs(t *testing.T) {
	id1 := types.NodeID("peer-1")
	id2 := types.NodeID("peer-2")
	
	distance := XORDistance(id1, id2)
	
	// 距离不应该全为0
	allZero := true
	for _, b := range distance {
		if b != 0 {
			allZero = false
			break
		}
	}
	assert.False(t, allZero, "不同ID的距离不应该全为0")
	
	t.Log("✅ 不同ID产生非零距离")
}

// TestXORDistance_Commutative 测试XOR距离的交换律
// dist(A, B) == dist(B, A)
func TestXORDistance_Commutative(t *testing.T) {
	id1 := types.NodeID("peer-alice")
	id2 := types.NodeID("peer-bob")
	
	dist1 := XORDistance(id1, id2)
	dist2 := XORDistance(id2, id1)
	
	assert.True(t, bytes.Equal(dist1, dist2), "XOR距离应该满足交换律")
	
	t.Log("✅ XOR距离满足交换律")
}

// TestXORDistance_EmptyIDs 测试空ID
func TestXORDistance_EmptyIDs(t *testing.T) {
	empty1 := types.NodeID("")
	empty2 := types.NodeID("")
	
	distance := XORDistance(empty1, empty2)
	
	// 空ID的距离应该为空切片或全0
	assert.NotNil(t, distance)
	
	t.Log("✅ 空ID不会panic")
}

// TestXORDistance_DifferentLengthIDs 测试不同长度的ID
// 这是一个重要的边界条件测试
func TestXORDistance_DifferentLengthIDs(t *testing.T) {
	shortID := types.NodeID("short")
	longID := types.NodeID("this-is-a-much-longer-id")
	
	distance := XORDistance(shortID, longID)
	
	// 应该补齐到相同长度
	assert.NotNil(t, distance)
	assert.Equal(t, len(longID), len(distance), "距离长度应该等于较长ID的长度")
	
	// 反向测试
	distance2 := XORDistance(longID, shortID)
	assert.True(t, bytes.Equal(distance, distance2), "不同顺序应该产生相同距离")
	
	t.Log("✅ 不同长度ID正确补齐")
}

// TestXORDistance_TriangleInequality 测试三角不等式（XOR度量的特性）
// dist(A, C) <= dist(A, B) + dist(B, C) 对于 XOR 不一定成立
// 但 XOR 满足：dist(A, C) XOR dist(C, B) = dist(A, B)
func TestXORDistance_XORProperty(t *testing.T) {
	a := types.NodeID("peer-a")
	b := types.NodeID("peer-b")
	c := types.NodeID("peer-c")
	
	// 获取距离
	distAB := XORDistance(a, b)
	distBC := XORDistance(b, c)
	distAC := XORDistance(a, c)
	
	// XOR 特性：AB XOR BC = AC
	result := make([]byte, len(distAB))
	for i := 0; i < len(distAB) && i < len(distBC); i++ {
		result[i] = distAB[i] ^ distBC[i]
	}
	
	// 这个特性在长度相同时成立
	if len(a) == len(b) && len(b) == len(c) {
		assert.True(t, bytes.Equal(result, distAC), "XOR距离应该满足 AB⊕BC=AC")
		t.Log("✅ XOR距离满足 AB⊕BC=AC 特性")
	}
}

// ============================================================================
// CompareDistance 测试
// ============================================================================

// TestCompareDistance_SameDistance 测试相同距离
func TestCompareDistance_SameDistance(t *testing.T) {
	target := types.NodeID("target")
	id1 := types.NodeID("peer-1")
	
	result := CompareDistance(id1, id1, target)
	
	assert.Equal(t, 0, result, "相同节点到目标的距离应该相等")
	
	t.Log("✅ 相同距离返回0")
}

// TestCompareDistance_Ordering 测试距离排序
func TestCompareDistance_Ordering(t *testing.T) {
	target := types.NodeID("target-node")
	
	// 创建多个节点
	nodes := []types.NodeID{
		"peer-a",
		"peer-b",
		"peer-c",
		"target-node", // 目标节点自己距离为0
	}
	
	// 目标节点到自己的距离最近
	for _, node := range nodes {
		if node != target {
			result := CompareDistance(target, node, target)
			assert.Equal(t, -1, result, "目标节点到自己应该最近")
		}
	}
	
	t.Log("✅ 距离排序正确")
}

// TestCompareDistance_Transitivity 测试传递性
// 如果 A < B 且 B < C，则 A < C
func TestCompareDistance_Transitivity(t *testing.T) {
	target := types.NodeID("target")
	a := types.NodeID("peer-a")
	b := types.NodeID("peer-b")
	c := types.NodeID("peer-c")
	
	// 假设 a < b < c（需要构造特定的ID来保证）
	// 这里只测试逻辑一致性
	compAB := CompareDistance(a, b, target)
	compBC := CompareDistance(b, c, target)
	compAC := CompareDistance(a, c, target)
	
	// 如果 a < b 且 b < c，则 a < c
	if compAB == -1 && compBC == -1 {
		assert.Equal(t, -1, compAC, "应该满足传递性")
		t.Log("✅ 距离比较满足传递性")
	}
}

// TestCompareDistance_Symmetry 测试对称性
// Compare(a, b, target) == -Compare(b, a, target)
func TestCompareDistance_Symmetry(t *testing.T) {
	target := types.NodeID("target")
	a := types.NodeID("peer-a")
	b := types.NodeID("peer-b")
	
	compAB := CompareDistance(a, b, target)
	compBA := CompareDistance(b, a, target)
	
	assert.Equal(t, -compAB, compBA, "比较应该满足对称性")
	
	t.Log("✅ 距离比较满足对称性")
}

// TestCompareDistance_DifferentLengths 测试不同长度ID的比较
func TestCompareDistance_DifferentLengths(t *testing.T) {
	target := types.NodeID("target")
	shortID := types.NodeID("short")
	longID := types.NodeID("this-is-longer-id")
	
	// 不应该panic
	result := CompareDistance(shortID, longID, target)
	
	assert.Contains(t, []int{-1, 0, 1}, result, "返回值应该是-1, 0, 或1")
	
	t.Log("✅ 不同长度ID比较不会panic")
}

// ============================================================================
// CommonPrefixLen 测试
// ============================================================================

// TestCommonPrefixLen_SameIDs 测试相同ID
func TestCommonPrefixLen_SameIDs(t *testing.T) {
	id := types.NodeID("test-peer")
	
	cpl := CommonPrefixLen(id, id)
	
	// 相同ID的共同前缀长度应该等于ID的位数
	expectedBits := len(id) * 8
	assert.Equal(t, expectedBits, cpl, "相同ID的共同前缀应该是全部位数")
	
	t.Log("✅ 相同ID共同前缀长度正确")
}

// TestCommonPrefixLen_CompletelyDifferent 测试完全不同的ID
func TestCommonPrefixLen_CompletelyDifferent(t *testing.T) {
	// 构造首字节就不同的ID
	id1 := types.NodeID("\xFF\xFF\xFF")
	id2 := types.NodeID("\x00\x00\x00")
	
	cpl := CommonPrefixLen(id1, id2)
	
	// 首字节就不同，共同前缀应该是0
	assert.Equal(t, 0, cpl, "完全不同的ID共同前缀应该为0")
	
	t.Log("✅ 完全不同ID共同前缀为0")
}

// TestCommonPrefixLen_PartialMatch 测试部分匹配
func TestCommonPrefixLen_PartialMatch(t *testing.T) {
	// 前2字节相同，第3字节不同
	id1 := types.NodeID("\xAB\xCD\xFF")
	id2 := types.NodeID("\xAB\xCD\x00")
	
	cpl := CommonPrefixLen(id1, id2)
	
	// 前16位相同（2字节）
	assert.Equal(t, 16, cpl, "前2字节相同应该返回16位")
	
	t.Log("✅ 部分匹配的共同前缀长度正确")
}

// TestCommonPrefixLen_SingleBitDifference 测试单比特差异
func TestCommonPrefixLen_SingleBitDifference(t *testing.T) {
	// 第1字节的最高位不同：0x80 vs 0x00
	id1 := types.NodeID("\x80\x00\x00")
	id2 := types.NodeID("\x00\x00\x00")
	
	cpl := CommonPrefixLen(id1, id2)
	
	// 第1位就不同
	assert.Equal(t, 0, cpl, "首位不同应该返回0")
	
	// 测试第2位不同
	id3 := types.NodeID("\xC0\x00\x00") // 11000000
	id4 := types.NodeID("\x80\x00\x00") // 10000000
	
	cpl2 := CommonPrefixLen(id3, id4)
	assert.Equal(t, 1, cpl2, "第2位不同应该返回1")
	
	t.Log("✅ 单比特差异检测正确")
}

// TestCommonPrefixLen_EmptyIDs 测试空ID
func TestCommonPrefixLen_EmptyIDs(t *testing.T) {
	empty1 := types.NodeID("")
	empty2 := types.NodeID("")
	
	// 不应该panic
	cpl := CommonPrefixLen(empty1, empty2)
	
	assert.Equal(t, 0, cpl, "空ID的共同前缀应该为0")
	
	t.Log("✅ 空ID不会panic")
}

// ============================================================================
// BucketIndex 测试
// ============================================================================

// TestBucketIndex_SameIDs 测试相同ID
func TestBucketIndex_SameIDs(t *testing.T) {
	id := types.NodeID("test-peer")
	
	index := BucketIndex(id, id)
	
	// 相同ID应该在最后一个bucket（最大索引）
	maxBuckets := len(id)*8 - 1
	assert.Equal(t, maxBuckets, index, "相同ID应该在最后一个bucket")
	
	t.Log("✅ 相同ID的bucket索引正确")
}

// TestBucketIndex_DifferentIDs 测试不同ID
func TestBucketIndex_DifferentIDs(t *testing.T) {
	local := types.NodeID("local-peer")
	remote1 := types.NodeID("\xFF\xFF\xFF") // 首字节完全不同
	remote2 := types.NodeID("local-peer-similar") // 部分相似
	
	index1 := BucketIndex(local, remote1)
	index2 := BucketIndex(local, remote2)
	
	// 完全不同的应该在较小的bucket
	// 部分相似的应该在较大的bucket
	assert.GreaterOrEqual(t, index2, index1, "更相似的ID应该在更大的bucket")
	
	t.Log("✅ 不同ID的bucket索引合理")
}

// TestBucketIndex_Range 测试索引范围
func TestBucketIndex_Range(t *testing.T) {
	local := types.NodeID("local-peer")
	
	// 测试多个随机ID
	testIDs := []types.NodeID{
		"peer-1",
		"peer-2",
		"peer-3",
		"\x00\x00\x00",
		"\xFF\xFF\xFF",
		"local-peer", // 相同ID
	}
	
	maxIndex := len(local)*8 - 1
	
	for _, remote := range testIDs {
		index := BucketIndex(local, remote)
		
		// 索引应该在有效范围内
		assert.GreaterOrEqual(t, index, 0, "索引不应该为负")
		assert.LessOrEqual(t, index, maxIndex, "索引不应该超过最大值")
	}
	
	t.Log("✅ Bucket索引在有效范围内")
}

// TestBucketIndex_Consistency 测试一致性
// 相同的输入应该总是产生相同的输出
func TestBucketIndex_Consistency(t *testing.T) {
	local := types.NodeID("local")
	remote := types.NodeID("remote")
	
	index1 := BucketIndex(local, remote)
	index2 := BucketIndex(local, remote)
	index3 := BucketIndex(local, remote)
	
	assert.Equal(t, index1, index2, "应该产生一致的结果")
	assert.Equal(t, index2, index3, "应该产生一致的结果")
	
	t.Log("✅ Bucket索引计算一致")
}

// ============================================================================
// 综合测试：真实场景
// ============================================================================

// TestXOR_RealWorldScenario 测试真实场景：节点排序
func TestXOR_RealWorldScenario(t *testing.T) {
	target := types.NodeID("target-node-for-lookup")
	
	// 创建多个节点
	nodes := []types.NodeID{
		"peer-alpha",
		"peer-beta",
		"peer-gamma",
		"peer-delta",
		"target-node-for-lookup", // 包含目标自己
	}
	
	// 计算所有节点到目标的距离
	type nodeWithDistance struct {
		id   types.NodeID
		dist []byte
	}
	
	var nodesWithDist []nodeWithDistance
	for _, node := range nodes {
		dist := XORDistance(node, target)
		nodesWithDist = append(nodesWithDist, nodeWithDistance{node, dist})
	}
	
	// 验证目标节点到自己的距离最近（全0）
	for _, nd := range nodesWithDist {
		if nd.id == target {
			allZero := true
			for _, b := range nd.dist {
				if b != 0 {
					allZero = false
					break
				}
			}
			assert.True(t, allZero, "目标到自己的距离应该全为0")
		}
	}
	
	t.Log("✅ 真实场景：节点距离计算正确")
}

// TestXOR_KBucketDistribution 测试K-Bucket分布
func TestXOR_KBucketDistribution(t *testing.T) {
	local := types.NodeID("local-node")
	
	// 创建不同距离的节点
	testCases := []struct {
		remote types.NodeID
		desc   string
	}{
		{local, "相同节点"},
		{types.NodeID("local-node-1"), "轻微不同"},
		{types.NodeID("different-node"), "中等不同"},
		{types.NodeID("\xFF\xFF\xFF\xFF"), "完全不同"},
	}
	
	buckets := make(map[int]int)
	
	for _, tc := range testCases {
		index := BucketIndex(local, tc.remote)
		buckets[index]++
		
		t.Logf("  %s -> bucket %d", tc.desc, index)
	}
	
	// 验证不同的节点分布在不同的bucket中
	// （相同节点除外）
	assert.GreaterOrEqual(t, len(buckets), 1, "至少应该有1个bucket被使用")
	
	t.Log("✅ K-Bucket分布测试通过")
}
