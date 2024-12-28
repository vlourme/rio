package security

import (
	"crypto/ecdh"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/security/mlkem768"
	"io"

	"golang.org/x/crypto/sha3"
)

// This file contains the functions necessary to compute the TLS 1.3 key
// schedule. See RFC 8446, Section 7.

type keySharePrivateKeys struct {
	curveID tls.CurveID
	ecdhe   *ecdh.PrivateKey
	kyber   *mlkem768.DecapsulationKey
}

// kyberDecapsulate implements decapsulation according to Kyber Round 3.
func kyberDecapsulate(dk *mlkem768.DecapsulationKey, c []byte) ([]byte, error) {
	K, err := mlkem768.Decapsulate(dk, c)
	if err != nil {
		return nil, err
	}
	return kyberSharedSecret(K, c), nil
}

// kyberEncapsulate implements encapsulation according to Kyber Round 3.
func kyberEncapsulate(ek []byte) (c, ss []byte, err error) {
	c, ss, err = mlkem768.Encapsulate(ek)
	if err != nil {
		return nil, nil, err
	}
	return c, kyberSharedSecret(ss, c), nil
}

func kyberSharedSecret(K, c []byte) []byte {
	// Package mlkem768 implements ML-KEM, which compared to Kyber removed a
	// final hashing step. Compute SHAKE-256(K || SHA3-256(c), 32) to match Kyber.
	// See https://words.filippo.io/mlkem768/#bonus-track-using-a-ml-kem-implementation-as-kyber-v3.
	h := sha3.NewShake256()
	h.Write(K)
	ch := sha3.Sum256(c)
	h.Write(ch[:])
	out := make([]byte, 32)
	h.Read(out)
	return out
}

const x25519PublicKeySize = 32

// generateECDHEKey returns a PrivateKey that implements Diffie-Hellman
// according to RFC 8446, Section 4.2.8.2.
func generateECDHEKey(rand io.Reader, curveID tls.CurveID) (*ecdh.PrivateKey, error) {
	curve, ok := curveForCurveID(curveID)
	if !ok {
		return nil, errors.New("tls: internal error: unsupported curve")
	}

	return curve.GenerateKey(rand)
}

func curveForCurveID(id tls.CurveID) (ecdh.Curve, bool) {
	switch id {
	case tls.X25519:
		return ecdh.X25519(), true
	case tls.CurveP256:
		return ecdh.P256(), true
	case tls.CurveP384:
		return ecdh.P384(), true
	case tls.CurveP521:
		return ecdh.P521(), true
	default:
		return nil, false
	}
}

func curveIDForCurve(curve ecdh.Curve) (tls.CurveID, bool) {
	switch curve {
	case ecdh.X25519():
		return tls.X25519, true
	case ecdh.P256():
		return tls.CurveP256, true
	case ecdh.P384():
		return tls.CurveP384, true
	case ecdh.P521():
		return tls.CurveP521, true
	default:
		return 0, false
	}
}
