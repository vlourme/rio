package aio_test

import (
	"bytes"
	"github.com/brickingsoft/rio/pkg/aio"
	"golang.org/x/sys/windows"
	"testing"
	"unsafe"
)

func TestBuf_Bytes(t *testing.T) {
	b := []byte("hello world")
	buf := aio.Buf{
		Buf: &b[0],
		Len: uint32(len(b)),
	}
	bufptr := unsafe.Pointer(&buf)
	t.Log(bufptr)
	wb := (*windows.WSABuf)(bufptr)
	t.Log(unsafe.Pointer(wb))

	p := unsafe.Slice(wb.Buf, wb.Len)
	t.Log(bytes.Equal(b, p), buf.Buf, wb.Buf)
	t.Log(string(p))
}

func TestMsg(t *testing.T) {
	b := []byte("hello world")
	oob := []byte("oob")
	addr, _, _, err := aio.ResolveAddr("udp", ":8080")
	if err != nil {
		t.Fatal(err)
	}
	msg := aio.Msg{}
	msg.AppendBuffer(b)
	msg.SetControl(oob)
	msg.SetAddr(addr)
	msg.Flags = 1

	msgptr := unsafe.Pointer(&msg)
	t.Log(msgptr)
	wm := (*windows.WSAMsg)(msgptr)
	t.Log(unsafe.Pointer(wm))
	buffers := unsafe.Slice(wm.Buffers, wm.BufferCount)
	buffer := buffers[0]
	p := unsafe.Slice(buffer.Buf, buffer.Len)
	t.Log(bytes.Equal(b, p))
	t.Log(string(p))
}
