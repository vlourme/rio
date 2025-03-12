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
	/* p2
	{8, 500 * time.Nanosecond},
	{16, 100 * time.Microsecond},
	{32, 200 * time.Microsecond},
	{64, 500 * time.Microsecond},

	Destination: [192.168.100.120]:9000
	Interface eth0 address [192.168.100.1]:0
	Using interface eth0 to connect to [192.168.100.120]:9000
	Ramped up to 50 connections.
	Total data sent:     212.4 MiB (222731136 bytes)
	Total data received: 211.6 MiB (221837280 bytes)
	Bandwidth per channel: 7.112⇅ Mbps (889.0 kBps)
	Aggregate bandwidth: 177.435↓, 178.150↑ Mbps
	Packet rate estimate: 20466.5↓, 15786.7↑ (3↓, 26↑ TCP MSS/op)
	Test duration: 10.002 s.
	*/

	/* p3
	{8, 500 * time.Nanosecond},
	{16, 100 * time.Microsecond},
	{32, 500 * time.Microsecond},
	{64, 1000 * time.Microsecond},

	Destination: [192.168.100.120]:9000
	Interface eth0 address [192.168.100.1]:0
	Using interface eth0 to connect to [192.168.100.120]:9000
	Ramped up to 50 connections.
	Total data sent:     389.5 MiB (408449520 bytes)
	Total data received: 387.5 MiB (406335365 bytes)
	Bandwidth per channel: 13.032⇅ Mbps (1629.0 kBps)
	Aggregate bandwidth: 324.949↓, 326.639↑ Mbps
	Packet rate estimate: 36002.3↓, 28189.6↑ (3↓, 25↑ TCP MSS/op)
	Test duration: 10.0037 s.
	*/
	defaultCurve = Curve{
		{1, 15 * time.Second},
		{4, 500 * time.Nanosecond},
		{8, 1 * time.Microsecond},
		{16, 2 * time.Microsecond},
		{32, 4 * time.Microsecond},
		{64, 10 * time.Microsecond},

		//{4, 500 * time.Nanosecond},
		//{8, 1 * time.Microsecond},
		//{16, 50 * time.Microsecond},
		//{32, 500 * time.Microsecond},
		//{64, 1 * time.Millisecond},
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
