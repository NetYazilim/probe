package probetls

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/netyazilim/probe"
)

// DefaultTimeout bounds a single handshake. A TLS handshake needs more
// headroom than the other probes, hence the larger value.
const DefaultTimeout = 5 * time.Second

// Result holds TLS/SSL probe result
type Result struct {
	probe.Summary

	Host string

	// HandshakeError is set in certificate-only mode when the server presented
	// a certificate but the handshake could not be completed, typically
	// because the server requires a client certificate.
	HandshakeError error

	Subject         string
	Issuer          string
	ExpiresAt       time.Time
	DaysUntilExpiry int
	Protocol        string
	CipherSuite     string
}

// Run performs a strict TLS/SSL probe: the handshake must complete and
// certificate verification must pass.
//
// It is a thin wrapper over RunContext kept for callers written against the
// original API.
func Run(target string, maxAttempts int, timeout time.Duration) Result {
	return RunContext(context.Background(), target, probe.Options{
		MaxAttempts: maxAttempts,
		Timeout:     timeout,
	})
}

// RunCertOnly performs a certificate-only TLS/SSL probe. It reports the
// presented server certificate even when the handshake cannot be completed
// because the server requires a client certificate for mTLS.
//
// It is a thin wrapper over RunCertOnlyContext kept for callers written
// against the original API.
func RunCertOnly(target string, maxAttempts int, timeout time.Duration) Result {
	return RunCertOnlyContext(context.Background(), target, probe.Options{
		MaxAttempts: maxAttempts,
		Timeout:     timeout,
	})
}

// RunContext performs a strict TLS/SSL probe, honouring ctx for cancellation.
func RunContext(ctx context.Context, target string, opts probe.Options) Result {
	return run(ctx, target, opts, false)
}

// RunCertOnlyContext performs a certificate-only TLS/SSL probe, honouring ctx
// for cancellation.
func RunCertOnlyContext(ctx context.Context, target string, opts probe.Options) Result {
	return run(ctx, target, opts, true)
}

func run(ctx context.Context, target string, opts probe.Options, certOnly bool) Result {
	address, serverName, err := normalizeTLSTarget(target)
	if err != nil {
		return Result{Summary: probe.Summary{Error: err}, Host: target}
	}

	opts = opts.WithDefaults(DefaultTimeout)
	if err := opts.Validate(); err != nil {
		return Result{Summary: probe.Summary{Error: err}, Host: serverName}
	}

	result := Result{Host: serverName}

	stats := probe.Repeat(ctx, opts, func(ctx context.Context) (time.Duration, error) {
		start := time.Now()

		dialer := &net.Dialer{Timeout: opts.Timeout}
		rawConn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return time.Since(start), fmt.Errorf("TCP connection failed: %v", err)
		}

		// In certificate-only mode the presented certificate is collected even
		// when the server expects a client certificate (mTLS) and the
		// handshake therefore fails.
		tlsConn := tls.Client(rawConn, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: certOnly,
		})

		handshakeErr := tlsConn.HandshakeContext(ctx)
		state := tlsConn.ConnectionState()
		elapsed := time.Since(start)
		_ = tlsConn.Close()

		certs := state.PeerCertificates
		if len(certs) == 0 {
			if handshakeErr != nil {
				return elapsed, fmt.Errorf("TLS handshake failed: %v", handshakeErr)
			}
			return elapsed, fmt.Errorf("TLS handshake did not return a certificate")
		}

		if handshakeErr != nil && !certOnly {
			result.HandshakeError = handshakeErr
			return elapsed, fmt.Errorf("TLS handshake failed: %v", handshakeErr)
		}

		cert := certs[0]
		result.Subject = cert.Subject.String()
		result.Issuer = cert.Issuer.String()
		result.ExpiresAt = cert.NotAfter
		result.DaysUntilExpiry = int(time.Until(cert.NotAfter).Hours() / 24)
		result.Protocol = tlsVersionToString(state.Version)
		result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
		if certOnly {
			result.HandshakeError = handshakeErr
		} else {
			result.HandshakeError = nil
		}

		return elapsed, nil
	})

	result.Summary = stats.Summarize(opts.SuccessThreshold)
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
