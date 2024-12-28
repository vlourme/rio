package security

import (
	"crypto/rand"
	"crypto/tls"
	"io"
	"slices"
)

const (
	x25519Kyber768Draft00 tls.CurveID = 0x6399
)

func configSupportsCurve(c *tls.Config, version uint16, curve tls.CurveID) bool {
	for _, cc := range configCurvePreferences(c, version) {
		if cc == curve {
			return true
		}
	}
	return false
}

func configCurvePreferences(c *tls.Config, version uint16) []tls.CurveID {
	var curvePreferences []tls.CurveID
	if c != nil && len(c.CurvePreferences) != 0 {
		curvePreferences = slices.Clone(c.CurvePreferences)
		if needFIPS() {
			return slices.DeleteFunc(curvePreferences, func(c tls.CurveID) bool {
				return !slices.Contains(defaultCurvePreferencesFIPS, c)
			})
		}
	} else if needFIPS() {
		curvePreferences = slices.Clone(defaultCurvePreferencesFIPS)
	} else {
		curvePreferences = defaultCurvePreferences()
	}
	if version < VersionTLS13 {
		return slices.DeleteFunc(curvePreferences, func(c tls.CurveID) bool {
			return c == x25519Kyber768Draft00
		})
	}
	return curvePreferences
}

func configRand(c *tls.Config) io.Reader {
	r := c.Rand
	if r == nil {
		return rand.Reader
	}
	return r
}
