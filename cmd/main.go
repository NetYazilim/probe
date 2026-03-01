package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/netyazilim/probe/pkg/http"
	"github.com/netyazilim/probe/pkg/ping"
	"github.com/netyazilim/probe/pkg/tcp"
	"github.com/netyazilim/probe/pkg/tls"
)

var (
	maxAttempts  int
	timeout      time.Duration
	loopInterval time.Duration
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Check for help
	if command == "-h" || command == "--help" || command == "help" {
		printUsage()
		os.Exit(0)
	}

	// Parse remaining arguments
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.IntVar(&maxAttempts, "attempts", 3, "Maximum number of attempts")
	fs.DurationVar(&timeout, "timeout", 0, "Timeout per attempt (0 = use default)")
	fs.DurationVar(&loopInterval, "loop", 0, "Loop interval (0 = run once, e.g., 5s, 1m)")

	// Parse flags from position 2 onwards
	err := fs.Parse(os.Args[2:])
	if err != nil {
		printUsage()
		os.Exit(1)
	}

	// Get remaining args (should be target)
	args := fs.Args()
	if len(args) < 1 {
		fmt.Printf("Error: %s command requires a target\n\n", command)
		printUsage()
		os.Exit(1)
	}

	target := args[0]

	// Set default timeout based on command
	defaultTimeout := 1 * time.Second
	if command == "tls" {
		defaultTimeout = 5 * time.Second
	}

	actualTimeout := timeout
	if actualTimeout == 0 {
		actualTimeout = defaultTimeout
	}

	// Handle loop
	if loopInterval == 0 {
		executeProbe(command, target, actualTimeout)
		return
	}

	// Run in loop
	fmt.Printf("Running probe every %v (press Ctrl+C to stop)\n\n", loopInterval)
	loopTicker := time.NewTicker(loopInterval)
	defer loopTicker.Stop()

	for {
		executeProbe(command, target, actualTimeout)
		fmt.Println("---")
		<-loopTicker.C
	}
}

func executeProbe(command, target string, timeout time.Duration) {
	switch command {
	case "ping":
		handlePing(target, timeout)
	case "tcp":
		handleTCP(target, timeout)
	case "tls":
		handleTLS(target, timeout)
	case "http":
		handleHTTP(target, timeout)
	default:
		fmt.Printf("Error: unknown command '%s'\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║            Probe - Multi-Protocol Status Check              ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println("\nUsage: probe <command> [flags] <target>")
	fmt.Println("\nCommands:")
	fmt.Println("  ping              ICMP ping probe (IP or hostname)")
	fmt.Println("  tcp               TCP port connectivity check")
	fmt.Println("  tls               TLS/SSL certificate information")
	fmt.Println("  http              HTTP/HTTPS status check")
	fmt.Println("\nFlags:")
	fmt.Println("  -attempts int     Maximum number of attempts (default: 3)")
	fmt.Println("  -timeout duration Timeout per attempt (default: 1s for ping/tcp/http, 5s for tls)")
	fmt.Println("  -loop duration    Loop interval (0 = run once, e.g., 5s, 1m, 10s)")
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Examples:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\n1. ICMP Ping")
	fmt.Println("  probe ping 8.8.8.8")
	fmt.Println("  probe ping google.com")
	fmt.Println("  probe ping -attempts 5 8.8.8.8")
	fmt.Println("  probe ping -timeout 2s google.com")
	fmt.Println("  probe ping -loop 5s 8.8.8.8                     # Run every 5 seconds")

	fmt.Println("\n2. TCP Port Connectivity Check")
	fmt.Println("  probe tcp example.com:22")
	fmt.Println("  probe tcp google.com:443")
	fmt.Println("  probe tcp -attempts 5 google.com:443")
	fmt.Println("  probe tcp -loop 10s example.com:22              # Run every 10 seconds")

	fmt.Println("\n3. HTTP/HTTPS Status Check")
	fmt.Println("  probe http https://google.com")
	fmt.Println("  probe http -timeout 3s https://example.com")
	fmt.Println("  probe http -loop 1m https://google.com          # Run every 1 minute")

	fmt.Println("\n4. TLS/SSL Certificate Information")
	fmt.Println("  probe tls google.com:443")
	fmt.Println("  probe tls -timeout 10s example.com:443")
	fmt.Println("  probe tls -loop 30m google.com:443              # Run every 30 minutes")

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Default Timeouts:")
	fmt.Println("  - ICMP Ping: 1 second per attempt")
	fmt.Println("  - TCP: 1 second per attempt")
	fmt.Println("  - HTTP/HTTPS: 1 second per attempt")
	fmt.Println("  - TLS/SSL: 5 seconds per attempt")
}

func handlePing(target string, timeout time.Duration) {
	result := ping.Run(target, maxAttempts, timeout)

	fmt.Printf("Target: %s\n", result.Address)
	if result.ResolvedIP != "" && result.ResolvedIP != result.Address {
		fmt.Printf("Resolved IP: %s\n", result.ResolvedIP)
	}
	fmt.Printf("Attempt: %d\n", result.Attempts)
	fmt.Printf("Success: %v\n", result.Success)

	if result.Duration > 0 {
		fmt.Printf("Duration: %.2f ms\n", float64(result.Duration.Microseconds())/1000.0)
	}

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		os.Exit(1)
	}
}

func handleTCP(target string, timeout time.Duration) {
	result := tcp.Run(target, maxAttempts, timeout)

	fmt.Printf("Host: %s\n", result.Host)
	fmt.Printf("Port: %s\n", result.Port)
	fmt.Printf("Attempt: %d\n", result.Attempts)
	fmt.Printf("Success: %v\n", result.Success)

	if result.Duration > 0 {
		fmt.Printf("Duration: %.2f ms\n", float64(result.Duration.Microseconds())/1000.0)
	}

	if result.Success {
		fmt.Printf("Local Address: %s\n", result.LocalAddr)
		fmt.Printf("Remote Address: %s\n", result.RemoteAddr)
	}

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		os.Exit(1)
	}
}

func handleTLS(target string, timeout time.Duration) {
	result := tls.Run(target, maxAttempts, timeout)

	fmt.Printf("Host: %s\n", result.Host)
	fmt.Printf("Attempt: %d\n", result.Attempts)
	fmt.Printf("Success: %v\n", result.Success)

	if result.Duration > 0 {
		fmt.Printf("Duration: %.2f ms\n", float64(result.Duration.Microseconds())/1000.0)
	}

	if result.Success {
		fmt.Printf("Subject: %s\n", result.Subject)
		fmt.Printf("Issuer: %s\n", result.Issuer)
		fmt.Printf("Expires At: %s\n", result.ExpiresAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Days Until Expiry: %d\n", result.DaysUntilExpiry)
		fmt.Printf("TLS Version: %s\n", result.Protocol)
		fmt.Printf("Cipher Suite: %s\n", result.CipherSuite)
	}

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		os.Exit(1)
	}
}

func handleHTTP(target string, timeout time.Duration) {
	result := http.Run(target, maxAttempts, timeout)

	fmt.Printf("URL: %s\n", result.URL)
	fmt.Printf("Attempt: %d\n", result.Attempts)
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Status Code: %d\n", result.StatusCode)

	if result.Duration > 0 {
		fmt.Printf("Duration: %.2f ms\n", float64(result.Duration.Microseconds())/1000.0)
	}

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		os.Exit(1)
	}
}
