package tls

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Result holds TLS/SSL probe result
type Result struct {
	Host            string
	Success         bool
	Attempts        int
	Error           error
	HandshakeError  error
	Duration        time.Duration
	Subject         string
	Issuer          string
	ExpiresAt       time.Time
	DaysUntilExpiry int
	Protocol        string
	CipherSuite     string
}

// Run performs a strict TLS/SSL probe.
// The handshake must complete successfully and certificate verification must pass.
func Run(target string, maxAttempts int, timeout time.Duration) Result {
	return run(target, maxAttempts, timeout, false)
}

// RunCertOnly performs a certificate-only TLS/SSL probe.
// It returns the presented server certificate even if the handshake cannot be
// fully completed because the server requires a client certificate for mTLS.
func RunCertOnly(target string, maxAttempts int, timeout time.Duration) Result {
	return run(target, maxAttempts, timeout, true)
}

func run(target string, maxAttempts int, timeout time.Duration, certOnly bool) Result {
	address, serverName, err := normalizeTLSTarget(target)
	if err != nil {
		return Result{
			Host:     target,
			Attempts: 0,
			Error:    err,
		}
	}

	result := Result{
		Host:     serverName,
		Attempts: maxAttempts,
	}

	for i := 0; i < maxAttempts; i++ {
		result.Attempts = i + 1

		startTime := time.Now()

		// Establish TCP connection first
		dialer := &net.Dialer{Timeout: timeout}
		rawConn, err := dialer.Dial("tcp", address)
		if err != nil {
			result.Duration = time.Since(startTime)
			result.Error = fmt.Errorf("TCP connection failed: %v", err)
			result.HandshakeError = nil
			if i < maxAttempts-1 {
				time.Sleep(500 * time.Millisecond)
			}
			continue
		}

		// In cert-only mode, still collect the presented certificate even if the
		// server expects a client certificate (mTLS) and the handshake fails.
		tlsConn := tls.Client(rawConn, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: certOnly,
		})

		handshakeErr := tlsConn.Handshake()
		state := tlsConn.ConnectionState()
		result.Duration = time.Since(startTime)
		_ = tlsConn.Close()

		certs := state.PeerCertificates
		if len(certs) == 0 {
			if handshakeErr != nil {
				result.Error = fmt.Errorf("TLS handshake failed: %v", handshakeErr)
			} else {
				result.Error = fmt.Errorf("TLS handshake did not return a certificate")
			}
			result.HandshakeError = nil
			if i < maxAttempts-1 {
				time.Sleep(500 * time.Millisecond)
			}
			continue
		}

		if handshakeErr != nil && !certOnly {
			result.Error = fmt.Errorf("TLS handshake failed: %v", handshakeErr)
			result.HandshakeError = handshakeErr
			if i < maxAttempts-1 {
				time.Sleep(500 * time.Millisecond)
			}
			continue
		}

		cert := certs[0]
		result.Subject = cert.Subject.String()
		result.Issuer = cert.Issuer.String()
		result.ExpiresAt = cert.NotAfter
		result.DaysUntilExpiry = int(time.Until(cert.NotAfter).Hours() / 24)
		result.Protocol = tlsVersionToString(state.Version)
		result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
		result.Success = true
		result.Error = nil
		if certOnly {
			result.HandshakeError = handshakeErr
		} else {
			result.HandshakeError = nil
		}
		break
	}

	return result
}

func normalizeTLSTarget(target string) (string, string, error) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return "", "", fmt.Errorf("target is empty")
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("invalid TLS target %q: %w", target, err)
		}
		raw = u.Host
		if raw == "" {
			return "", "", fmt.Errorf("invalid TLS target %q: missing host", target)
		}
	}

	const defaultPort = "443"
	host := raw
	port := defaultPort

	if parsedHost, parsedPort, err := net.SplitHostPort(raw); err == nil {
		host = parsedHost
		port = parsedPort
	} else {
		trimmed := strings.TrimSpace(strings.Trim(raw, "[]"))
		if ip := net.ParseIP(trimmed); ip != nil {
			host = trimmed
		} else if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			host = trimmed
		} else if strings.Count(raw, ":") > 0 {
			return "", "", fmt.Errorf("invalid TLS target %q: %w", target, err)
		}
	}

	serverName := strings.TrimSpace(strings.Trim(host, "[]"))
	if serverName == "" {
		return "", "", fmt.Errorf("invalid TLS target %q: missing hostname", target)
	}

	address := net.JoinHostPort(serverName, port)
	return address, serverName, nil
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
