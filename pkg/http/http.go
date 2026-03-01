package http

import (
	"fmt"
	"net/http"
	"time"
)

// Result holds HTTP probe result
type Result struct {
	URL        string
	Success    bool
	StatusCode int
	Attempts   int
	Error      error
	Duration   time.Duration
}

// Run performs HTTP/HTTPS probe to the specified URL.
// url specifies the target URL (http:// or https://).
// maxAttempts specifies the maximum number of attempts.
// timeout specifies the maximum duration to wait for a response.
// Returns a Result struct containing the results of the HTTP probe operation.
func Run(url string, maxAttempts int, timeout time.Duration) Result {
	result := Result{
		URL:      url,
		Attempts: maxAttempts,
	}

	client := &http.Client{
		Timeout: timeout,
	}

	for i := 0; i < maxAttempts; i++ {
		result.Attempts = i + 1

		startTime := time.Now()
		resp, err := client.Get(url)
		result.Duration = time.Since(startTime)

		if err != nil {
			result.Error = fmt.Errorf("request failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		result.StatusCode = resp.StatusCode
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Success = true
			result.Error = nil
			break
		}

		result.Error = fmt.Errorf("HTTP %d", resp.StatusCode)
		time.Sleep(500 * time.Millisecond)
	}

	return result
}
