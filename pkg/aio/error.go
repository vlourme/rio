package aio

import (
	"github.com/brickingsoft/errors"
)

var (
	ErrUnexpectedCompletion = errors.Define("unexpected completion error", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrBusy                 = errors.Define("busy", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
)

func IsUnexpectedCompletionError(err error) bool {
	return errors.Is(err, ErrUnexpectedCompletion)
}

func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}

const (
	errMetaPkgKey = "pkg"
	errMetaPkgVal = "aio"
)

const (
	errMetaOpKey      = "op"
	errMetaOpListen   = "listen"
	errMetaOpConnect  = "connect"
	errMetaOpAccept   = "accept"
	errMetaOpClose    = "close"
	errMetaOpRecv     = "receive"
	errMetaOpRecvFrom = "receive_from"
	errMetaOpRecvMsg  = "receive_message"
	errMetaOpSend     = "send"
	errMetaOpSendTo   = "send_to"
	errMetaOpSendMsg  = "send_message"
	errMetaOpSendfile = "sendfile"
)
