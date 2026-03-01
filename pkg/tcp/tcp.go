package tcp

import (
	"fmt"
	"net"
	"time"
)

// Result holds TCP probe result
type Result struct {
	Host       string
	Port       string
	Success    bool
	Attempts   int
	Error      error
	Duration   time.Duration
	RemoteAddr string
	LocalAddr  string
}

// Run performs TCP probe to the specified host.
// target specifies the target (hostname:port or IP:port, e.g., example.com:80).
// maxAttempts specifies the maximum number of attempts.
// timeout specifies the maximum duration to wait for a connection.
// Returns a Result struct containing the TCP connection information.
func Run(target string, maxAttempts int, timeout time.Duration) Result {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return Result{
			Host:     target,
			Success:  false,
			Attempts: 0,
			Error:    fmt.Errorf("invalid target format: %v", err),
		}
	}

	result := Result{
		Host:     host,
		Port:     port,
		Attempts: maxAttempts,
	}

	for i := 0; i < maxAttempts; i++ {
		result.Attempts = i + 1

		startTime := time.Now()

		// Establish TCP connection
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := dialer.Dial("tcp", target)
		result.Duration = time.Since(startTime)

		if err != nil {
			result.Error = fmt.Errorf("TCP connection failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Get connection details
		result.RemoteAddr = conn.RemoteAddr().String()
		result.LocalAddr = conn.LocalAddr().String()
		result.Success = true
		result.Error = nil
		_ = conn.Close()
		break
	}

	return result
}
