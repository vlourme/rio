package security

import "fmt"

// QUICEncryptionLevel represents a QUIC encryption level used to transmit
// handshake messages.
type QUICEncryptionLevel int

const (
	QUICEncryptionLevelInitial = QUICEncryptionLevel(iota)
	QUICEncryptionLevelEarly
	QUICEncryptionLevelHandshake
	QUICEncryptionLevelApplication
)

func (l QUICEncryptionLevel) String() string {
	switch l {
	case QUICEncryptionLevelInitial:
		return "Initial"
	case QUICEncryptionLevelEarly:
		return "Early"
	case QUICEncryptionLevelHandshake:
		return "Handshake"
	case QUICEncryptionLevelApplication:
		return "Application"
	default:
		return fmt.Sprintf("QUICEncryptionLevel(%v)", int(l))
	}
}
