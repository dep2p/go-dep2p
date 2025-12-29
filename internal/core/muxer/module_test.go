package muxer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

func TestModule(t *testing.T) {
	var muxerFactory muxerif.MuxerFactory

	app := fxtest.New(t,
		Module(),
		fx.Populate(
			fx.Annotate(&muxerFactory, fx.ParamTags(`name:"muxer_factory"`)),
		),
	)
	defer app.RequireStart().RequireStop()

	assert.NotNil(t, muxerFactory)
	assert.Equal(t, "composite", muxerFactory.Protocol())
}

func TestModuleWithConfig(t *testing.T) {
	customConfig := muxerif.Config{
		MaxStreams:          100,
		MaxStreamWindowSize: 512 * 1024,
	}

	var muxerFactory muxerif.MuxerFactory

	app := fxtest.New(t,
		fx.Provide(func() *muxerif.Config { return &customConfig }),
		Module(),
		fx.Populate(
			fx.Annotate(&muxerFactory, fx.ParamTags(`name:"muxer_factory"`)),
		),
	)
	defer app.RequireStart().RequireStop()

	assert.NotNil(t, muxerFactory)
}

func TestModuleLifecycle(t *testing.T) {
	var muxerFactory muxerif.MuxerFactory

	app := fxtest.New(t,
		Module(),
		fx.Populate(
			fx.Annotate(&muxerFactory, fx.ParamTags(`name:"muxer_factory"`)),
		),
	)

	// 启动应用
	err := app.Start(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, muxerFactory)

	// 停止应用
	err = app.Stop(context.Background())
	require.NoError(t, err)
}

func TestProvideServicesDirectly(t *testing.T) {
	input := ModuleInput{}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.MuxerFactory)
	assert.Equal(t, "composite", output.MuxerFactory.Protocol())
}

func TestProvideServicesWithConfig(t *testing.T) {
	config := muxerif.Config{
		MaxStreams:          50,
		MaxStreamWindowSize: 256 * 1024,
	}

	input := ModuleInput{
		Config: &config,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.MuxerFactory)
}

func TestModuleConstants(t *testing.T) {
	assert.Equal(t, "1.0.0", Version)
	assert.Equal(t, "muxer", Name)
	assert.NotEmpty(t, Description)
}

