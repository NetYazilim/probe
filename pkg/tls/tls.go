package tls

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// Result holds TLS/SSL probe result
type Result struct {
	Host            string
	Success         bool
	Attempts        int
	Error           error
	Duration        time.Duration
	Subject         string
	Issuer          string
	ExpiresAt       time.Time
	DaysUntilExpiry int
	Protocol        string
	CipherSuite     string
}

// Run performs TLS/SSL probe to the specified host.
// host specifies the target host (hostname:port, e.g., example.com:443).
// maxAttempts specifies the maximum number of attempts.
// timeout specifies the maximum duration to wait for a response.
// Returns a Result struct containing the TLS/SSL information.
func Run(host string, maxAttempts int, timeout time.Duration) Result {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	result := Result{
		Host:     hostname,
		Attempts: maxAttempts,
	}

	for i := 0; i < maxAttempts; i++ {
		result.Attempts = i + 1

		startTime := time.Now()

		// Establish TCP connection first
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := dialer.Dial("tcp", host)
		if err != nil {
			result.Duration = time.Since(startTime)
			result.Error = fmt.Errorf("TCP connection failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Perform TLS handshake
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         hostname,
			InsecureSkipVerify: false,
		})

		err = tlsConn.Handshake()
		result.Duration = time.Since(startTime)

		if err != nil {
			_ = tlsConn.Close()
			result.Error = fmt.Errorf("TLS handshake failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Retrieve certificates
		certs := tlsConn.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			_ = tlsConn.Close()
			result.Error = fmt.Errorf("no certificates found")
			time.Sleep(500 * time.Millisecond)
			continue
		}

		cert := certs[0]
		result.Subject = cert.Subject.String()
		result.Issuer = cert.Issuer.String()
		result.ExpiresAt = cert.NotAfter
		result.DaysUntilExpiry = int(time.Until(cert.NotAfter).Hours() / 24)
		result.Protocol = tlsVersionToString(tlsConn.ConnectionState().Version)
		result.CipherSuite = tls.CipherSuiteName(tlsConn.ConnectionState().CipherSuite)
		result.Success = true
		result.Error = nil
		_ = tlsConn.Close()
		break
	}

	return result
}

// tlsVersionToString converts TLS version to string format
func tlsVersionToString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04x)", version)
	}
}
