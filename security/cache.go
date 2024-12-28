package security

import (
	"crypto/x509"
	"runtime"
	"sync"
	"sync/atomic"
)

type cacheEntry struct {
	refs atomic.Int64
	cert *x509.Certificate
}

type ActiveCert struct {
	cert *x509.Certificate
}

func (e *ActiveCert) Cert() *x509.Certificate {
	return e.cert
}

var globalCertCache = new(CertCache)

type CertCache struct {
	sync.Map
}

func (cc *CertCache) active(e *cacheEntry) *ActiveCert {
	e.refs.Add(1)
	a := &ActiveCert{e.cert}
	runtime.SetFinalizer(a, func(_ *ActiveCert) {
		if e.refs.Add(-1) == 0 {
			cc.evict(e)
		}
	})
	return a
}

func (cc *CertCache) evict(e *cacheEntry) {
	cc.Delete(string(e.cert.Raw))
}

func (cc *CertCache) newCert(der []byte) (*ActiveCert, error) {
	if entry, ok := cc.Load(string(der)); ok {
		return cc.active(entry.(*cacheEntry)), nil
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	entry := &cacheEntry{cert: cert}
	if entry, loaded := cc.LoadOrStore(string(der), entry); loaded {
		return cc.active(entry.(*cacheEntry)), nil
	}
	return cc.active(entry), nil
}
