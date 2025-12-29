// Package app 测试文件
package app

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/core/address"
	endpointmod "github.com/dep2p/go-dep2p/internal/core/endpoint"
	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/internal/core/realm"
)

// TestFxNameTag_Address_NAT 验证 address 模块 NAT name 标签修复
// P0-1: name 标签从 "nat_service" 修复为 "nat"
func TestFxNameTag_Address_NAT(t *testing.T) {
	// 检查 address.ModuleInput 的 NATService 字段
	inputType := reflect.TypeOf(address.ModuleInput{})
	field, found := inputType.FieldByName("NATService")
	require.True(t, found, "NATService 字段应该存在")

	// 获取 name tag
	fxTag := field.Tag.Get("name")
	assert.Equal(t, "nat", fxTag, "address 模块 NATService 应该使用 name:\"nat\" 标签")

	// 验证 optional 标签
	optionalTag := field.Tag.Get("optional")
	assert.Equal(t, "true", optionalTag, "NATService 应该是可选的")
}

// TestFxNameTag_NAT_Output 验证 nat 模块输出 name 标签
func TestFxNameTag_NAT_Output(t *testing.T) {
	// 检查 nat.ModuleOutput 的 NATService 字段
	outputType := reflect.TypeOf(nat.ModuleOutput{})
	field, found := outputType.FieldByName("NATService")
	require.True(t, found, "NATService 字段应该存在")

	// 获取 name tag
	fxTag := field.Tag.Get("name")
	assert.Equal(t, "nat", fxTag, "nat 模块应该输出 name:\"nat\"")
}

// TestFxNameTag_Discovery_Realm 已废弃
// v1.1: Discovery 模块不再依赖 RealmManager，Realm 改为顶层强制模块
func TestFxNameTag_Discovery_Realm(t *testing.T) {
	t.Skip("v1.1: Realm is now a mandatory top-level module, discovery no longer depends on it")
}

// TestFxNameTag_Realm_Output 验证 realm 模块输出 name 标签
func TestFxNameTag_Realm_Output(t *testing.T) {
	// 检查 realm.ModuleOutput 的 RealmManager 字段
	outputType := reflect.TypeOf(realm.ModuleOutput{})
	field, found := outputType.FieldByName("RealmManager")
	require.True(t, found, "RealmManager 字段应该存在")

	// 获取 name tag
	fxTag := field.Tag.Get("name")
	assert.Equal(t, "realm_manager", fxTag, "realm 模块应该输出 name:\"realm_manager\"")
}

// TestMonitoringLayer_Setup 验证 Bootstrap 包含 Monitoring 层
// P0-3: bandwidth/netreport 模块应该被正确组装
func TestMonitoringLayer_Setup(t *testing.T) {
	b := &Bootstrap{}

	// 获取 Monitoring 层的 fx.Option
	monitoringLayer := b.setupMonitoringLayer()

	// 验证返回值不为 nil
	require.NotNil(t, monitoringLayer, "Monitoring 层应该被正确配置")
}

// TestEndpoint_AddressBook_Input 验证 endpoint 模块可以接收 address 依赖
// P1-1: endpoint 添加 address_book/address_manager 可选依赖
func TestEndpoint_AddressBook_Input(t *testing.T) {
	inputType := reflect.TypeOf(endpointmod.ModuleInput{})

	// 检查 AddressBook 字段
	t.Run("AddressBook", func(t *testing.T) {
		field, found := inputType.FieldByName("AddressBook")
		require.True(t, found, "AddressBook 字段应该存在")

		fxTag := field.Tag.Get("name")
		assert.Equal(t, "address_book", fxTag, "endpoint 应该使用 name:\"address_book\" 标签")

		optionalTag := field.Tag.Get("optional")
		assert.Equal(t, "true", optionalTag, "AddressBook 应该是可选的")
	})

	// 检查 AddressManager 字段
	t.Run("AddressManager", func(t *testing.T) {
		field, found := inputType.FieldByName("AddressManager")
		require.True(t, found, "AddressManager 字段应该存在")

		fxTag := field.Tag.Get("name")
		assert.Equal(t, "address_manager", fxTag, "endpoint 应该使用 name:\"address_manager\" 标签")

		optionalTag := field.Tag.Get("optional")
		assert.Equal(t, "true", optionalTag, "AddressManager 应该是可选的")
	})
}

// TestAddress_Output_Names 验证 address 模块输出的 name 标签
func TestAddress_Output_Names(t *testing.T) {
	outputType := reflect.TypeOf(address.ModuleOutput{})

	t.Run("AddressBook output", func(t *testing.T) {
		field, found := outputType.FieldByName("AddressBook")
		require.True(t, found, "AddressBook 字段应该存在")

		fxTag := field.Tag.Get("name")
		assert.Equal(t, "address_book", fxTag, "address 模块应该输出 name:\"address_book\"")
	})

	t.Run("AddressManager output", func(t *testing.T) {
		field, found := outputType.FieldByName("AddressManager")
		require.True(t, found, "AddressManager 字段应该存在")

		fxTag := field.Tag.Get("name")
		assert.Equal(t, "address_manager", fxTag, "address 模块应该输出 name:\"address_manager\"")
	})
}

// TestNameTagConsistency 验证所有 name 标签的一致性
func TestNameTagConsistency(t *testing.T) {
	testCases := []struct {
		name       string
		inputType  reflect.Type
		inputField string
		outputType reflect.Type
		outputField string
		expectedName string
	}{
		{
			name:        "NAT: address -> nat",
			inputType:   reflect.TypeOf(address.ModuleInput{}),
			inputField:  "NATService",
			outputType:  reflect.TypeOf(nat.ModuleOutput{}),
			outputField: "NATService",
			expectedName: "nat",
		},
		// v1.1: "Realm: discovery -> realm" 测试项已移除
		// Discovery 模块不再依赖 RealmManager，Realm 改为顶层强制模块
		{
			name:        "AddressBook: endpoint -> address",
			inputType:   reflect.TypeOf(endpointmod.ModuleInput{}),
			inputField:  "AddressBook",
			outputType:  reflect.TypeOf(address.ModuleOutput{}),
			outputField: "AddressBook",
			expectedName: "address_book",
		},
		{
			name:        "AddressManager: endpoint -> address",
			inputType:   reflect.TypeOf(endpointmod.ModuleInput{}),
			inputField:  "AddressManager",
			outputType:  reflect.TypeOf(address.ModuleOutput{}),
			outputField: "AddressManager",
			expectedName: "address_manager",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 验证输入端
			inputField, found := tc.inputType.FieldByName(tc.inputField)
			require.True(t, found, "输入字段 %s 应该存在", tc.inputField)
			inputName := inputField.Tag.Get("name")
			assert.Equal(t, tc.expectedName, inputName, "输入端 name 标签应该为 %s", tc.expectedName)

			// 验证输出端
			outputField, found := tc.outputType.FieldByName(tc.outputField)
			require.True(t, found, "输出字段 %s 应该存在", tc.outputField)
			outputName := outputField.Tag.Get("name")
			assert.Equal(t, tc.expectedName, outputName, "输出端 name 标签应该为 %s", tc.expectedName)

			// 验证一致性
			assert.Equal(t, inputName, outputName, "输入和输出的 name 标签应该一致")
		})
	}
}
