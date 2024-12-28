package security

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"strconv"
)

type AlertError uint8

func (e AlertError) Error() string {
	return Alert(e).String()
}

type Alert uint8

func (e Alert) String() string {
	s, ok := alertText[e]
	if ok {
		return "tls: " + s
	}
	return "tls: Alert(" + strconv.Itoa(int(e)) + ")"
}

func (e Alert) Error() string {
	return e.String()
}

func SendAlert(ctx context.Context, writer transport.Writer, alert AlertError) (future async.Future[async.Void]) {

	return
}

const (
	// Alert level
	alertLevelWarning = 1
	alertLevelError   = 2
)

const (
	alertCloseNotify                  Alert = 0
	alertUnexpectedMessage            Alert = 10
	alertBadRecordMAC                 Alert = 20
	alertDecryptionFailed             Alert = 21
	alertRecordOverflow               Alert = 22
	alertDecompressionFailure         Alert = 30
	alertHandshakeFailure             Alert = 40
	alertBadCertificate               Alert = 42
	alertUnsupportedCertificate       Alert = 43
	alertCertificateRevoked           Alert = 44
	alertCertificateExpired           Alert = 45
	alertCertificateUnknown           Alert = 46
	alertIllegalParameter             Alert = 47
	alertUnknownCA                    Alert = 48
	alertAccessDenied                 Alert = 49
	alertDecodeError                  Alert = 50
	alertDecryptError                 Alert = 51
	alertExportRestriction            Alert = 60
	alertProtocolVersion              Alert = 70
	alertInsufficientSecurity         Alert = 71
	alertInternalError                Alert = 80
	alertInappropriateFallback        Alert = 86
	alertUserCanceled                 Alert = 90
	alertNoRenegotiation              Alert = 100
	alertMissingExtension             Alert = 109
	alertUnsupportedExtension         Alert = 110
	alertCertificateUnobtainable      Alert = 111
	alertUnrecognizedName             Alert = 112
	alertBadCertificateStatusResponse Alert = 113
	alertBadCertificateHashValue      Alert = 114
	alertUnknownPSKIdentity           Alert = 115
	alertCertificateRequired          Alert = 116
	alertNoApplicationProtocol        Alert = 120
	alertECHRequired                  Alert = 121
)

var alertText = map[Alert]string{
	alertCloseNotify:                  "close notify",
	alertUnexpectedMessage:            "unexpected message",
	alertBadRecordMAC:                 "bad record MAC",
	alertDecryptionFailed:             "decryption failed",
	alertRecordOverflow:               "record overflow",
	alertDecompressionFailure:         "decompression failure",
	alertHandshakeFailure:             "handshake failure",
	alertBadCertificate:               "bad certificate",
	alertUnsupportedCertificate:       "unsupported certificate",
	alertCertificateRevoked:           "revoked certificate",
	alertCertificateExpired:           "expired certificate",
	alertCertificateUnknown:           "unknown certificate",
	alertIllegalParameter:             "illegal parameter",
	alertUnknownCA:                    "unknown certificate authority",
	alertAccessDenied:                 "access denied",
	alertDecodeError:                  "error decoding message",
	alertDecryptError:                 "error decrypting message",
	alertExportRestriction:            "export restriction",
	alertProtocolVersion:              "protocol version not supported",
	alertInsufficientSecurity:         "insufficient security level",
	alertInternalError:                "internal error",
	alertInappropriateFallback:        "inappropriate fallback",
	alertUserCanceled:                 "user canceled",
	alertNoRenegotiation:              "no renegotiation",
	alertMissingExtension:             "missing extension",
	alertUnsupportedExtension:         "unsupported extension",
	alertCertificateUnobtainable:      "certificate unobtainable",
	alertUnrecognizedName:             "unrecognized name",
	alertBadCertificateStatusResponse: "bad certificate status response",
	alertBadCertificateHashValue:      "bad certificate hash value",
	alertUnknownPSKIdentity:           "unknown PSK identity",
	alertCertificateRequired:          "certificate required",
	alertNoApplicationProtocol:        "no application protocol",
	alertECHRequired:                  "encrypted client hello required",
}
