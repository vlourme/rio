package aio

import (
	"syscall"
	"time"
)

type Transmission interface {
	Up() (uint32, *syscall.Timespec)
	Down() (uint32, *syscall.Timespec)
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

var (
	/* p5 ok -T 10s and -r 5000
	{1, 15 * time.Second},
	{8, 500 * time.Nanosecond},
	{16, 1 * time.Microsecond},
	{32, 50 * time.Microsecond},
	{64, 100 * time.Microsecond},
	{96, 500 * time.Microsecond},

	{1, 15 * time.Second},
	{8, 500 * time.Nanosecond},
	{16, 1 * time.Microsecond},
	{32, 10 * time.Microsecond},
	{64, 50 * time.Microsecond},
	{128, 100 * time.Microsecond},
	{256, 150 * time.Microsecond},
	{512, 200 * time.Microsecond},
	{1024, 500 * time.Microsecond},
	*/

	/* p6 ok -T 10s and -r 5000
	{1, 15 * time.Second},
	{8, 500 * time.Nanosecond},
	{16, 1 * time.Microsecond},
	{32, 10 * time.Microsecond},
	{64, 50 * time.Microsecond},
	{128, 100 * time.Microsecond},
	{256, 200 * time.Microsecond},
	{512, 500 * time.Microsecond},

	wrk -t 10 -c 1000 -d 10s http://192.168.100.120:9000/
	Running 10s test @ http://192.168.100.120:9000/
	  10 threads and 1000 connections
	  Thread Stats   Avg      Stdev     Max   +/- Stdev
	    Latency    19.74ms   69.11ms 863.76ms   94.80%
	    Req/Sec    19.59k     4.73k   54.37k    65.79%
	  1947667 requests in 10.10s, 187.60MB read
	Requests/sec: 192891.79
	Transfer/sec:     18.58MB
	*/
	defaultCurve = Curve{
		{1, 15 * time.Second},
		{8, 1 * time.Microsecond},
		{16, 10 * time.Microsecond},
		{32, 100 * time.Microsecond},
		{64, 200 * time.Microsecond},
		{96, 500 * time.Microsecond},
	}
)

func NewCurveTransmission(curve Curve) Transmission {
	if len(curve) == 0 {
		curve = defaultCurve
	}
	times := make([]WaitNTime, len(curve))
	for i, t := range curve {
		n := t.N
		if n == 0 {
			n = 1
		}
		timeout := t.Timeout
		if timeout < 1 {
			timeout = 1 * time.Millisecond
		}
		times[i] = WaitNTime{
			n:    n,
			time: syscall.NsecToTimespec(timeout.Nanoseconds()),
		}
	}
	return &CurveTransmission{
		curve: times,
		size:  len(curve),
		idx:   -1,
	}
}

type WaitNTime struct {
	n    uint32
	time syscall.Timespec
}

type CurveTransmission struct {
	curve []WaitNTime
	size  int
	idx   int
}

func (tran *CurveTransmission) Up() (uint32, *syscall.Timespec) {
	if tran.idx == tran.size-1 {
		return tran.curve[tran.idx].n, &tran.curve[tran.idx].time
	}
	tran.idx++
	return tran.curve[tran.idx].n, &tran.curve[tran.idx].time
}

func (tran *CurveTransmission) Down() (uint32, *syscall.Timespec) {
	if tran.idx == 0 {
		return tran.curve[0].n, &tran.curve[0].time
	}
	tran.idx--
	return tran.curve[tran.idx].n, &tran.curve[tran.idx].time
}
