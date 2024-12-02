package aio

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
)

type Settings struct {
	QueueSize  uint32
	Flags      uint32
	Features   uint32
	Threads    uint32
	ThreadIdle uint32
}

type Options struct {
	// EngineCylinders
	// 引擎规模
	EngineCylinders int
	// LoadBalance
	// 负载均衡器。 RoundRobin 和 Least
	LoadBalance LoadBalanceKind
	// Settings
	// AIO配置设置
	Settings any
}

func ResolveSettings[T any](settings any) T {
	if settings == nil {
		return *new(T)
	}
	s, ok := settings.(T)
	if !ok {
		panic(fmt.Errorf("aio: cannot resolve %s settings from %s", reflect.TypeOf(*new(T)), reflect.TypeOf(settings)))
	}
	return s
}

type Cylinder interface {
	Fd() int
	Loop(beg func(), end func())
	Stop()
	Up()
	Down()
	Actives() int64
}

func NextCylinder() Cylinder {
	return engine().next()
}

type LoadBalanceKind int

const (
	RoundRobin LoadBalanceKind = iota
	Least
)

type CylindersLoadBalancer interface {
	Next() (cylinder Cylinder)
}

func newRoundRobinCylindersLoadBalancer(cylinders []Cylinder) CylindersLoadBalancer {
	return &RoundRobinCylindersLoadBalancer{
		idx:       -1,
		num:       int64(len(cylinders)),
		cylinders: cylinders,
	}
}

type RoundRobinCylindersLoadBalancer struct {
	idx       int64
	num       int64
	cylinders []Cylinder
}

func (lb *RoundRobinCylindersLoadBalancer) Next() (cylinder Cylinder) {
	idx := atomic.AddInt64(&lb.idx, 1) % lb.num
	cylinder = lb.cylinders[idx]
	return
}

func newLeastCylindersLoadBalancer(cylinders []Cylinder) CylindersLoadBalancer {
	return &leastCylindersLoadBalancer{
		num:       len(cylinders),
		cylinders: cylinders,
	}
}

type leastCylindersLoadBalancer struct {
	num       int
	cylinders []Cylinder
}

func (lb *leastCylindersLoadBalancer) Next() (cylinder Cylinder) {
	idx := 0
	actives := int64(0)
	for i, c := range lb.cylinders {
		cActives := c.Actives()
		if cActives < 1 {
			idx = i
			break
		}
		if actives < cActives {
			idx = i
		}
	}
	cylinder = lb.cylinders[idx]
	return
}

func newEngine(options Options) *Engine {
	cylinders := options.EngineCylinders
	if cylinders < 1 {
		cylinders = runtime.NumCPU() * 2
	}
	_engine = &Engine{
		fd:        0,
		settings:  options.Settings,
		cylinders: make([]Cylinder, cylinders),
		wg:        new(sync.WaitGroup),
	}
	switch options.LoadBalance {
	case Least:
		_engine.loadBalancer = newLeastCylindersLoadBalancer(_engine.cylinders)
		break
	default:
		_engine.loadBalancer = newRoundRobinCylindersLoadBalancer(_engine.cylinders)
	}
	return _engine
}

type Engine struct {
	fd           int
	settings     any
	loadBalancer CylindersLoadBalancer
	cylinders    []Cylinder
	wg           *sync.WaitGroup
}

func (engine *Engine) next() Cylinder {
	return engine.loadBalancer.Next()
}

func (engine *Engine) markCylinderLoop() {
	engine.wg.Add(1)
}

func (engine *Engine) markCylinderStop() {
	engine.wg.Done()
}

var (
	_createEngineOnce         = sync.Once{}
	_engine           *Engine = nil
	_defaultOptions           = Options{
		EngineCylinders: 0,
		LoadBalance:     RoundRobin,
		Settings:        nil,
	}
)

func Startup(options Options) {
	_engine = newEngine(options)
	_engine.Start()
}

func Shutdown() {
	engine().Stop()
}

func engine() *Engine {
	_createEngineOnce.Do(func() {
		if _engine == nil {
			_engine = newEngine(_defaultOptions)
			_engine.Start()
			runtime.SetFinalizer(_engine, (*Engine).Stop)
		}
	})
	return _engine
}
