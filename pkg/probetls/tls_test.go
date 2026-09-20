package probetls

import "testing"

func TestNormalizeTLSTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantAddr   string
		wantServer string
		wantErr    bool
	}{
		{"bare host gets the default port", "example.com", "example.com:443", "example.com", false},
		{"host with port", "example.com:8443", "example.com:8443", "example.com", false},
		{"https URL", "https://example.com", "example.com:443", "example.com", false},
		{"https URL with path", "https://example.com/health", "example.com:443", "example.com", false},
		{"https URL with port", "https://example.com:8443/x", "example.com:8443", "example.com", false},
		{"IPv4", "10.0.0.1", "10.0.0.1:443", "10.0.0.1", false},
		{"IPv4 with port", "10.0.0.1:8443", "10.0.0.1:8443", "10.0.0.1", false},
		{"bare IPv6", "::1", "[::1]:443", "::1", false},
		{"bracketed IPv6", "[::1]", "[::1]:443", "::1", false},
		{"bracketed IPv6 with port", "[::1]:8443", "[::1]:8443", "::1", false},
		{"surrounding whitespace", "  example.com  ", "example.com:443", "example.com", false},
		{"empty", "", "", "", true},
		{"only whitespace", "   ", "", "", true},
		{"scheme without host", "https://", "", "", true},
		{"malformed host:port:port", "example.com:80:90", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, server, err := normalizeTLSTarget(tt.target)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if addr != tt.wantAddr {
				t.Errorf("address = %q, want %q", addr, tt.wantAddr)
			}
			if server != tt.wantServer {
				t.Errorf("serverName = %q, want %q", server, tt.wantServer)
			}
		})
	}
}

func TestTLSVersionToString(t *testing.T) {
	tests := map[uint16]string{
		0x0301: "TLS 1.0",
		0x0302: "TLS 1.1",
		0x0303: "TLS 1.2",
		0x0304: "TLS 1.3",
		0x0300: "Unknown (0x0300)",
	}

	for version, want := range tests {
		if got := tlsVersionToString(version); got != want {
			t.Errorf("tlsVersionToString(%#04x) = %q, want %q", version, got, want)
		}
	}
}
