package aio

import (
	"runtime"
	"sync"
)

type Settings struct {
	Flags      uint32
	Threads    uint32
	ThreadIdle uint32
}

type Options struct {
	// EngineCylinders
	// 引擎规模
	EngineCylinders int
	// Settings
	// AIO配置设置
	Settings Settings
}

type Engine struct {
	fd        int
	cylinders int
	settings  Settings
}

var (
	_createEngineOnce         = sync.Once{}
	_engine           *Engine = nil
	_defaultOptions           = Options{
		EngineCylinders: runtime.NumCPU() * 2,
		Settings:        Settings{},
	}
)

func Startup(options Options) {
	cylinders := options.EngineCylinders
	if cylinders < 1 {
		cylinders = runtime.NumCPU() * 2
	}
	_engine = &Engine{
		fd:        0,
		cylinders: cylinders,
		settings:  options.Settings,
	}
	_engine.Start()
}

func Shutdown() {
	engine().Stop()
}

func engine() *Engine {
	_createEngineOnce.Do(func() {
		if _engine == nil {
			_engine = &Engine{
				fd:        0,
				cylinders: _defaultOptions.EngineCylinders,
				settings:  _defaultOptions.Settings,
			}
			_engine.Start()
			runtime.SetFinalizer(_engine, (*Engine).Stop)
		}
	})
	return _engine
}
