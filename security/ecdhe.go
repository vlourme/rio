package security

import (
	"crypto/tls"
)

// TLS Elliptic Curve Point Formats
// https://www.iana.org/assignments/tls-parameters/tls-parameters.xml#tls-parameters-9
const (
	pointFormatUncompressed uint8 = 0
)

// supportsECDHE returns whether ECDHE key exchanges can be used with this
// pre-TLS 1.3 client.
func supportsECDHE(c *tls.Config, version uint16, supportedCurves []tls.CurveID, supportedPoints []uint8) bool {
	supportsCurve := false
	for _, curve := range supportedCurves {
		if configSupportsCurve(c, version, curve) {
			supportsCurve = true
			break
		}
	}

	supportsPointFormat := false
	for _, pointFormat := range supportedPoints {
		if pointFormat == pointFormatUncompressed {
			supportsPointFormat = true
			break
		}
	}
	// Per RFC 8422, Section 5.1.2, if the Supported Point Formats extension is
	// missing, uncompressed points are supported. If supportedPoints is empty,
	// the extension must be missing, as an empty extension body is rejected by
	// the parser. See https://go.dev/issue/49126.
	if len(supportedPoints) == 0 {
		supportsPointFormat = true
	}

	return supportsCurve && supportsPointFormat
}
