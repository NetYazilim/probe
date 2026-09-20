package probetls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/netyazilim/probe"
)

// issue creates a certificate valid over the given window. With parent nil it
// is a self-signed CA; otherwise it is signed by parent.
func issue(t *testing.T, cn string, notBefore, notAfter time.Time, isCA bool,
	parent *x509.Certificate, parentKey *ecdsa.PrivateKey,
) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generating serial: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}
	return cert, key, der
}

// serveTLS starts a TLS listener presenting the given chain and returns its
// address. The server never validates its own certificate, so an expired one
// can be served deliberately.
func serveTLS(t *testing.T, chain [][]byte, key *ecdsa.PrivateKey) string {
	t.Helper()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: chain, PrivateKey: key}},
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				if c, ok := conn.(*tls.Conn); ok {
					_ = c.HandshakeContext(context.Background())
				}
				_ = conn.Close()
			}()
		}
	}()

	return ln.Addr().String()
}

func certOpts() probe.Options {
	return probe.Options{MaxAttempts: 1, Timeout: 5 * time.Second, Interval: time.Millisecond}
}

func TestCertOnlyAcceptsValidCertificate(t *testing.T) {
	now := time.Now()
	_, key, der := issue(t, "valid.example", now.Add(-24*time.Hour), now.Add(60*24*time.Hour), false, nil, nil)
	addr := serveTLS(t, [][]byte{der}, key)

	r := RunCertOnlyContext(context.Background(), addr, certOpts())

	if !r.Success {
		t.Fatalf("Success = false for a valid certificate: %v", r.Error)
	}
	if r.Expired || r.NotYetValid {
		t.Errorf("Expired = %v, NotYetValid = %v, want both false", r.Expired, r.NotYetValid)
	}
	if r.DaysUntilExpiry < 58 || r.DaysUntilExpiry > 60 {
		t.Errorf("DaysUntilExpiry = %d, want about 60", r.DaysUntilExpiry)
	}
	if r.ChainLength != 1 {
		t.Errorf("ChainLength = %d, want 1", r.ChainLength)
	}
	if r.NotBefore.IsZero() {
		t.Error("NotBefore not reported")
	}
	if !strings.Contains(r.Subject, "valid.example") {
		t.Errorf("Subject = %q, want it to name the certificate", r.Subject)
	}
}

func TestCertOnlyRejectsExpiredCertificate(t *testing.T) {
	now := time.Now()
	_, key, der := issue(t, "expired.example", now.Add(-90*24*time.Hour), now.Add(-10*24*time.Hour), false, nil, nil)
	addr := serveTLS(t, [][]byte{der}, key)

	r := RunCertOnlyContext(context.Background(), addr, certOpts())

	if r.Success {
		t.Fatal("Success = true for an expired certificate; a health check must not call that healthy")
	}
	if !r.Expired {
		t.Error("Expired = false although the certificate is expired")
	}
	if r.Error == nil || !strings.Contains(r.Error.Error(), "certificate expired on") {
		t.Errorf("Error = %v, want it to say the certificate expired", r.Error)
	}
	// An expired leaf also makes the chain's earliest expiry lie in the past,
	// so both checks would fire. The leaf check has to be the one that reports,
	// otherwise the message blames the chain for the leaf's problem.
	if r.Error != nil && strings.Contains(r.Error.Error(), "chain") {
		t.Errorf("Error = %v, want the leaf to be blamed rather than the chain", r.Error)
	}
	if r.DaysUntilExpiry >= 0 {
		t.Errorf("DaysUntilExpiry = %d, want a negative count for an expired certificate", r.DaysUntilExpiry)
	}
	// The details still have to be reported: they are what the user needs.
	if !strings.Contains(r.Subject, "expired.example") || r.ExpiresAt.IsZero() {
		t.Errorf("certificate details missing on failure: subject %q expires %v", r.Subject, r.ExpiresAt)
	}
}

func TestCertOnlyRejectsNotYetValidCertificate(t *testing.T) {
	now := time.Now()
	_, key, der := issue(t, "future.example", now.Add(48*time.Hour), now.Add(400*24*time.Hour), false, nil, nil)
	addr := serveTLS(t, [][]byte{der}, key)

	r := RunCertOnlyContext(context.Background(), addr, certOpts())

	if r.Success {
		t.Fatal("Success = true for a certificate that is not valid yet")
	}
	if !r.NotYetValid {
		t.Error("NotYetValid = false although the validity window has not started")
	}
	if r.Error == nil || !strings.Contains(r.Error.Error(), "not valid until") {
		t.Errorf("Error = %v, want it to say when the certificate becomes valid", r.Error)
	}
}

func TestCertOnlyReportsTheWeakestLinkInTheChain(t *testing.T) {
	now := time.Now()

	// The leaf is healthy for a year, but the intermediate that signed it
	// expires in ten days. Only looking at the leaf hides the problem.
	ca, caKey, caDER := issue(t, "intermediate.example", now.Add(-24*time.Hour), now.Add(10*24*time.Hour), true, nil, nil)
	_, leafKey, leafDER := issue(t, "leaf.example", now.Add(-24*time.Hour), now.Add(365*24*time.Hour), false, ca, caKey)

	addr := serveTLS(t, [][]byte{leafDER, caDER}, leafKey)

	r := RunCertOnlyContext(context.Background(), addr, certOpts())

	if !r.Success {
		t.Fatalf("Success = false although nothing has expired yet: %v", r.Error)
	}
	if r.ChainLength != 2 {
		t.Fatalf("ChainLength = %d, want 2", r.ChainLength)
	}
	if r.DaysUntilExpiry < 360 {
		t.Errorf("DaysUntilExpiry = %d, want about 365 for the leaf", r.DaysUntilExpiry)
	}
	if r.ChainDaysUntilExpiry > 11 {
		t.Errorf("ChainDaysUntilExpiry = %d, want about 10: the intermediate is the weakest link",
			r.ChainDaysUntilExpiry)
	}
	if !r.ChainExpiresAt.Before(r.ExpiresAt) {
		t.Errorf("chain expiry %v is not earlier than the leaf's %v", r.ChainExpiresAt, r.ExpiresAt)
	}
}

func TestCertOnlyRejectsExpiredIntermediate(t *testing.T) {
	now := time.Now()

	ca, caKey, caDER := issue(t, "old-ca.example", now.Add(-400*24*time.Hour), now.Add(-5*24*time.Hour), true, nil, nil)
	_, leafKey, leafDER := issue(t, "leaf.example", now.Add(-24*time.Hour), now.Add(365*24*time.Hour), false, ca, caKey)

	addr := serveTLS(t, [][]byte{leafDER, caDER}, leafKey)

	r := RunCertOnlyContext(context.Background(), addr, certOpts())

	if r.Success {
		t.Fatal("Success = true although an intermediate in the chain has expired")
	}
	if r.Expired {
		t.Error("Expired = true, but it is the intermediate that expired, not the leaf")
	}
	if r.Error == nil || !strings.Contains(r.Error.Error(), "chain") {
		t.Errorf("Error = %v, want it to point at the chain", r.Error)
	}
}

func TestStrictModeRejectsUntrustedCertificate(t *testing.T) {
	now := time.Now()
	_, key, der := issue(t, "self-signed.example", now.Add(-24*time.Hour), now.Add(60*24*time.Hour), false, nil, nil)
	addr := serveTLS(t, [][]byte{der}, key)

	r := RunContext(context.Background(), addr, certOpts())

	if r.Success {
		t.Fatal("Success = true in strict mode for a self-signed certificate")
	}
	if r.Error == nil {
		t.Error("Error = nil although the handshake must fail")
	}
}
