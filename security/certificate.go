package security

import (
	"crypto/tls"
	"crypto/x509"
)

func leafOfCertificate(c *tls.Certificate) (*x509.Certificate, error) {
	if c.Leaf != nil {
		return c.Leaf, nil
	}
	return x509.ParseCertificate(c.Certificate[0])
}
