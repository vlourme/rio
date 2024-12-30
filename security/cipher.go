package security

import "io"

type Cipher interface {
	Id() uint16
	Name() string
	Encrypt(recordType RecordType, version int, plainText []byte, rand io.Reader) (record []byte, err error)
	Decrypt(record []byte) (recordType RecordType, version int, plainText []byte, err error)
}
