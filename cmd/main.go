package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/netyazilim/probe"
	"github.com/netyazilim/probe/pkg/probehttp"
	"github.com/netyazilim/probe/pkg/probeping"
	"github.com/netyazilim/probe/pkg/probetcp"
	"github.com/netyazilim/probe/pkg/probetls"
)

var (
	maxAttempts  int
	threshold    int
	timeout      time.Duration
	interval     time.Duration
	loopInterval time.Duration
	size         int
	debug        bool
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
	fs.IntVar(&maxAttempts, "attempts", probe.DefaultMaxAttempts, "Maximum number of attempts")
	fs.IntVar(&threshold, "threshold", probe.DefaultSuccessThreshold, "Successful attempts required for the probe to succeed")
	fs.DurationVar(&timeout, "timeout", 0, "Timeout per attempt (0 = use default)")
	fs.DurationVar(&interval, "interval", probe.DefaultInterval, "Pause between attempts")
	fs.DurationVar(&loopInterval, "loop", 0, "Loop interval (0 = run once, e.g., 5s, 1m)")
	fs.IntVar(&size, "size", probe.DefaultSize, "ICMP payload size in bytes (ping only)")
	fs.BoolVar(&debug, "debug", false, "Log per-attempt detail to stderr")

	err := fs.Parse(os.Args[2:])
	if err != nil {
		printUsage()
		os.Exit(1)
	}

	// Reject unknown commands before doing anything else, so that a typo is not
	// reported once per iteration in loop mode.
	if !isKnownCommand(command) {
		fmt.Printf("Error: unknown command '%s'\n\n", command)
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

	// flag.Parse stops at the first non-flag argument, so anything after the
	// target was silently discarded before. Reject it instead: a user who
	// writes "probe tcp host:port -attempts 5" means those flags to count.
	if len(args) > 1 {
		extra := args[1:]
		fmt.Printf("Error: unexpected argument(s) after the target: %s\n", strings.Join(extra, " "))
		if strings.HasPrefix(extra[0], "-") {
			fmt.Println("Flags must come before the target, e.g. probe " + command + " -attempts 5 " + target)
		}
		os.Exit(1)
	}

	// Validate the flags as typed. The flag defaults are the real defaults, so a
	// zero here was written by the user: Options treats zero as "unset", but on
	// the command line "-attempts 0" is a mistake, not a request for 3.
	switch {
	case maxAttempts < 1:
		fmt.Printf("Error: -attempts must be at least 1 (got %d)\n", maxAttempts)
		os.Exit(1)
	case threshold < 1:
		fmt.Printf("Error: -threshold must be at least 1 (got %d)\n", threshold)
		os.Exit(1)
	case threshold > maxAttempts:
		fmt.Printf("Error: -threshold (%d) cannot exceed -attempts (%d)\n", threshold, maxAttempts)
		os.Exit(1)
	case timeout < 0:
		fmt.Printf("Error: -timeout must not be negative (got %v)\n", timeout)
		os.Exit(1)
	case interval < 0:
		fmt.Printf("Error: -interval must not be negative (got %v)\n", interval)
		os.Exit(1)
	case size < 0:
		fmt.Printf("Error: -size must not be negative (got %d)\n", size)
		os.Exit(1)
	case loopInterval < 0:
		fmt.Printf("Error: -loop must not be negative (got %v)\n", loopInterval)
		os.Exit(1)
	}

	opts := probe.Options{
		MaxAttempts:      maxAttempts,
		SuccessThreshold: threshold,
		Timeout:          timeout,
		Interval:         interval,
		Size:             size,
	}
	if debug {
		opts.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	// Ctrl+C cancels the probe in flight rather than only killing the process,
	// which matters in loop mode where interrupting is the documented way to
	// stop. A second signal takes the default action and terminates at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Handle loop
	if loopInterval == 0 {
		if !executeProbe(ctx, command, target, opts) {
			os.Exit(1)
		}
		return
	}

	// Run in loop
	fmt.Printf("Running probe every %v (press Ctrl+C to stop)\n\n", loopInterval)
	loopTicker := time.NewTicker(loopInterval)
	defer loopTicker.Stop()

	for {
		// The return value is intentionally ignored: in loop mode a failing
		// probe is a result to report, not a reason to terminate.
		executeProbe(ctx, command, target, opts)
		fmt.Println("---")

		select {
		case <-ctx.Done():
			return
		case <-loopTicker.C:
		}
	}
}

// isKnownCommand reports whether command is a supported probe command.
func isKnownCommand(command string) bool {
	switch command {
	case "ping", "tcp", "tls", "tls-cert", "http":
		return true
	default:
		return false
	}
}

// executeProbe runs a single probe and reports whether it succeeded.
// It never terminates the process, so that callers (in particular loop mode)
// stay in control of the exit behaviour.
func executeProbe(ctx context.Context, command, target string, opts probe.Options) bool {
	switch command {
	case "ping":
		return handlePing(ctx, target, opts)
	case "tcp":
		return handleTCP(ctx, target, opts)
	case "tls":
		return handleTLS(ctx, target, opts, false)
	case "tls-cert":
		return handleTLS(ctx, target, opts, true)
	case "http":
		return handleHTTP(ctx, target, opts)
	default:
		fmt.Printf("Error: unknown command '%s'\n\n", command)
		printUsage()
		return false
	}
}

func printUsage() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║            Probe - Multi-Protocol Health Check              ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println("\nUsage: probe <command> [flags] <target>")
	fmt.Println("\nCommands:")
	fmt.Println("  ping              ICMP ping probe (IP or hostname)")
	fmt.Println("  tcp               TCP port connectivity check")
	fmt.Println("  tls               Strict TLS/SSL handshake and certificate check")
	fmt.Println("  tls-cert          TLS/SSL certificate info without requiring full handshake")
	fmt.Println("  http              HTTP/HTTPS status check")
	fmt.Println("\nFlags:")
	fmt.Println("  -attempts int     Maximum number of attempts (default: 3)")
	fmt.Println("  -threshold int    Successful attempts required to pass (default: 1)")
	fmt.Println("  -timeout duration Timeout per attempt (default: 1s for ping/tcp/http, 5s for tls/tls-cert)")
	fmt.Println("  -interval duration Pause between attempts (default: 500ms)")
	fmt.Println("  -loop duration    Loop interval (0 = run once, e.g., 5s, 1m, 10s)")
	fmt.Println("  -size int         ICMP payload size in bytes, ping only (default: 56)")
	fmt.Println("  -debug            Log per-attempt detail to stderr")
	fmt.Println("\nFlags must come before the target.")
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Examples:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\n1. ICMP Ping")
	fmt.Println("  probe ping 8.8.8.8")
	fmt.Println("  probe ping google.com")
	fmt.Println("  probe ping -attempts 5 8.8.8.8")
	fmt.Println("  probe ping -attempts 5 -threshold 3 8.8.8.8      # 3 of up to 5 must answer")
	fmt.Println("  probe ping -size 1472 gateway.local              # path MTU check")
	fmt.Println("  probe ping -loop 5s 8.8.8.8                      # Run every 5 seconds")

	fmt.Println("\n2. TCP Port Connectivity Check")
	fmt.Println("  probe tcp example.com:22")
	fmt.Println("  probe tcp -attempts 5 google.com:443")
	fmt.Println("  probe tcp -loop 10s example.com:22              # Run every 10 seconds")

	fmt.Println("\n3. HTTP/HTTPS Status Check")
	fmt.Println("  probe http https://google.com")
	fmt.Println("  probe http -timeout 3s https://example.com")
	fmt.Println("  probe http -loop 1m https://google.com          # Run every 1 minute")

	fmt.Println("\n4. TLS/SSL Strict Check")
	fmt.Println("  probe tls google.com:443")
	fmt.Println("  probe tls -timeout 10s example.com:443")
	fmt.Println("  probe tls -loop 30m google.com:443              # Run every 30 minutes")

	fmt.Println("\n5. TLS/SSL Certificate-Only Check")
	fmt.Println("  probe tls-cert google.com:443")
	fmt.Println("  probe tls-cert https://example.com")
	fmt.Println("  probe tls-cert -timeout 10s mtls.example.com:443")

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Default Timeouts:")
	fmt.Println("  - ICMP Ping: 1 second per attempt")
	fmt.Println("  - TCP: 1 second per attempt")
	fmt.Println("  - HTTP/HTTPS: 1 second per attempt")
	fmt.Println("  - TLS/SSL: 5 seconds per attempt")
}

// printSummary prints the fields every probe has in common.
func printSummary(s probe.Summary) {
	fmt.Printf("Attempt: %d\n", s.Attempts)
	fmt.Printf("Success: %v\n", s.Success)
	if s.Successes > 1 || s.Failures > 0 {
		fmt.Printf("Successes: %d  Failures: %d\n", s.Successes, s.Failures)
	}
	if s.Duration > 0 {
		fmt.Printf("Duration: %s\n", ms(s.Duration))
	}
	if s.Successes > 1 {
		fmt.Printf("Min/Avg/Max: %s / %s / %s\n", ms(s.Min), ms(s.Avg), ms(s.Max))
	}
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000.0)
}

func handlePing(ctx context.Context, target string, opts probe.Options) bool {
	result := probeping.RunContext(ctx, target, opts)

	fmt.Printf("Target: %s\n", result.Address)
	if result.ResolvedIP != "" && result.ResolvedIP != result.Address {
		fmt.Printf("Resolved IP: %s\n", result.ResolvedIP)
	}
	printSummary(result.Summary)

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		return false
	}
	return result.Success
}

func handleTCP(ctx context.Context, target string, opts probe.Options) bool {
	result := probetcp.RunContext(ctx, target, opts)

	fmt.Printf("Host: %s\n", result.Host)
	fmt.Printf("Port: %s\n", result.Port)
	printSummary(result.Summary)

	if result.Success {
		fmt.Printf("Local Address: %s\n", result.LocalAddr)
		fmt.Printf("Remote Address: %s\n", result.RemoteAddr)
	}

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		return false
	}
	return result.Success
}

func handleTLS(ctx context.Context, target string, opts probe.Options, certOnly bool) bool {
	var result probetls.Result
	if certOnly {
		result = probetls.RunCertOnlyContext(ctx, target, opts)
	} else {
		result = probetls.RunContext(ctx, target, opts)
	}

	fmt.Printf("Host: %s\n", result.Host)
	printSummary(result.Summary)

	if result.Success {
		fmt.Printf("Subject: %s\n", result.Subject)
		fmt.Printf("Issuer: %s\n", result.Issuer)
		fmt.Printf("Expires At: %s\n", result.ExpiresAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Days Until Expiry: %d\n", result.DaysUntilExpiry)
		fmt.Printf("TLS Version: %s\n", result.Protocol)
		fmt.Printf("Cipher Suite: %s\n", result.CipherSuite)
		if certOnly && result.HandshakeError != nil {
			fmt.Printf("Handshake Warning: %v\n", result.HandshakeError)
		}
	}

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		return false
	}
	return result.Success
}

func handleHTTP(ctx context.Context, target string, opts probe.Options) bool {
	result := probehttp.RunContext(ctx, target, opts)

	fmt.Printf("URL: %s\n", result.URL)
	printSummary(result.Summary)
	fmt.Printf("Status Code: %d\n", result.StatusCode)

	if result.Error != nil {
		fmt.Printf("Error: %v\n", result.Error)
		return false
	}
	return result.Success
}
