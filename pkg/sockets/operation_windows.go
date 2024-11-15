//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"io"
	"unsafe"
)

type operation struct {
	// Used by IOCP interface, it must be first field
	// of the struct, as our code rely on it.
	overlapped windows.Overlapped
	mode       OperationMode
	// fields used only by net package
	conn   *connection
	buf    windows.WSABuf
	msg    windows.WSAMsg
	sa     windows.Sockaddr
	rsa    *windows.RawSockaddrAny
	rsan   int32
	iocp   windows.Handle
	handle windows.Handle
	flags  uint32
	bufs   []windows.WSABuf
	qty    uint32
	// fields used only by net callback
	acceptHandler   AcceptHandler
	dialHandler     DialHandler
	readHandler     ReadHandler
	writeHandler    WriteHandler
	readFromHandler ReadFromHandler
	readMsgHandler  ReadMsgHandler
	writeMsgHandler WriteMsgHandler
}

func (op *operation) complete(qty int, err error) {
	switch op.mode {
	case accept:
		op.completeAccept(qty, err)
		break
	case dial:
		op.completeDial(qty, err)
		break
	case read:
		op.completeRead(qty, err)
		break
	case write:
		op.completeWrite(qty, err)
		break
	case readFrom:
		op.completeReadFrom(qty, err)
		break
	case writeTo:
		op.completeWriteTo(qty, err)
		break
	case readMsg:
		op.completeReadMsg(qty, err)
		break
	case writeMsg:
		op.completeWriteMsg(qty, err)
		break
	default:
		break
	}
	op.reset()
}

func (op *operation) reset() {
	op.overlapped.Offset = 0
	op.overlapped.OffsetHigh = 0
	op.overlapped.Internal = 0
	op.overlapped.InternalHigh = 0
	op.overlapped.HEvent = 0
	op.mode = 0
}

func (op *operation) eofError(qty int, err error) error {
	if qty == 0 && err == nil && op.conn.zeroReadIsEOF {
		return io.EOF
	}
	return err
}

func (op *operation) InitMsg(p []byte, oob []byte) {
	op.InitBuf(p)
	op.msg.Buffers = &op.buf
	op.msg.BufferCount = 1

	op.msg.Name = nil
	op.msg.Namelen = 0

	op.msg.Flags = 0
	op.msg.Control.Len = uint32(len(oob))
	op.msg.Control.Buf = nil
	if len(oob) != 0 {
		op.msg.Control.Buf = &oob[0]
	}
}

func (op *operation) OOB() (oob []byte) {
	if op.mode != readMsg {
		oob = make([]byte, 0, 1)
		return
	}
	oobLen := int(op.msg.Control.Len)
	oob = unsafe.Slice(op.msg.Control.Buf, oobLen)
	return
}

func (op *operation) InitBuf(buf []byte) {
	op.buf.Len = uint32(len(buf))
	op.buf.Buf = nil
	if len(buf) != 0 {
		op.buf.Buf = &buf[0]
	}
}
