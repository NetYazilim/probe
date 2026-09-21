// Command probe runs a single health check from the command line.
//
// The logic here is deliberately split so it can be tested without a process:
// parseArgs turns an argument list into a config and never exits, and every
// printing function writes to an io.Writer rather than to stdout directly.
// main is the only part that reads os.Args, exits, or touches the terminal.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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

// errHelp reports that the user asked for usage rather than made a mistake.
var errHelp = errors.New("help requested")

// config is a fully validated command line.
type config struct {
	command string
	target  string
	opts    probe.Options
	loop    time.Duration
}

func main() {
	cfg, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, errHelp) {
			usage(os.Stdout)
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		usage(os.Stderr)
		os.Exit(1)
	}

	// Ctrl+C cancels the probe in flight rather than only killing the process,
	// which matters in loop mode where interrupting is the documented way to
	// stop. A second signal takes the default action and terminates at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if cfg.loop == 0 {
		if !executeProbe(ctx, os.Stdout, cfg) {
			os.Exit(1)
		}
		return
	}

	runLoop(ctx, os.Stdout, cfg)
}

// parseArgs validates a command line and reports problems as errors instead of
// exiting, so the rules can be tested directly. logDest receives the
// per-attempt log when -debug is set.
func parseArgs(argv []string, logDest io.Writer) (config, error) {
	if len(argv) == 0 {
		return config{}, errors.New("a command is required")
	}

	command := argv[0]
	switch command {
	case "-h", "--help", "help":
		return config{}, errHelp
	}
	// Rejected here rather than at probe time so that a typo is reported once,
	// not on every iteration of a loop.
	if !isKnownCommand(command) {
		return config{}, fmt.Errorf("unknown command %q", command)
	}

	var (
		maxAttempts  int
		threshold    int
		timeout      time.Duration
		interval     time.Duration
		loopInterval time.Duration
		size         int
		debug        bool
	)

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // errors are returned, not printed here
	fs.IntVar(&maxAttempts, "attempts", probe.DefaultMaxAttempts, "Maximum number of attempts")
	fs.IntVar(&threshold, "threshold", probe.DefaultSuccessThreshold, "Successful attempts required for the probe to succeed")
	fs.DurationVar(&timeout, "timeout", 0, "Timeout per attempt (0 = use default)")
	fs.DurationVar(&interval, "interval", probe.DefaultInterval, "Pause between attempts")
	fs.DurationVar(&loopInterval, "loop", 0, "Loop interval (0 = run once, e.g., 5s, 1m)")
	fs.IntVar(&size, "size", probe.DefaultSize, "ICMP payload size in bytes (ping only)")
	fs.BoolVar(&debug, "debug", false, "Log per-attempt detail to stderr")

	if err := fs.Parse(argv[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config{}, errHelp
		}
		return config{}, err
	}

	args := fs.Args()
	if len(args) == 0 {
		return config{}, fmt.Errorf("the %s command requires a target", command)
	}
	target := args[0]

	// flag.Parse stops at the first non-flag argument, so anything after the
	// target would otherwise be discarded in silence. A user who writes
	// "probe tcp host:port -attempts 5" means those flags to count.
	if len(args) > 1 {
		extra := strings.Join(args[1:], " ")
		if strings.HasPrefix(args[1], "-") {
			return config{}, fmt.Errorf(
				"unexpected argument(s) after the target: %s\nFlags must come before the target, e.g. probe %s %s %s",
				extra, command, args[1], target)
		}
		return config{}, fmt.Errorf("unexpected argument(s) after the target: %s", extra)
	}

	// Validated as typed. The flag defaults are the real defaults, so a zero
	// here was written by the user: Options treats zero as "unset", but on the
	// command line "-attempts 0" is a mistake, not a request for the default.
	switch {
	case maxAttempts < 1:
		return config{}, fmt.Errorf("-attempts must be at least 1 (got %d)", maxAttempts)
	case threshold < 1:
		return config{}, fmt.Errorf("-threshold must be at least 1 (got %d)", threshold)
	case threshold > maxAttempts:
		return config{}, fmt.Errorf("-threshold (%d) cannot exceed -attempts (%d)", threshold, maxAttempts)
	case timeout < 0:
		return config{}, fmt.Errorf("-timeout must not be negative (got %v)", timeout)
	case interval < 0:
		return config{}, fmt.Errorf("-interval must not be negative (got %v)", interval)
	case size < 0:
		return config{}, fmt.Errorf("-size must not be negative (got %d)", size)
	case loopInterval < 0:
		return config{}, fmt.Errorf("-loop must not be negative (got %v)", loopInterval)
	}

	cfg := config{
		command: command,
		target:  target,
		loop:    loopInterval,
		opts: probe.Options{
			MaxAttempts:      maxAttempts,
			SuccessThreshold: threshold,
			Timeout:          timeout,
			Interval:         interval,
			Size:             size,
		},
	}
	if debug {
		cfg.opts.Logger = slog.New(slog.NewTextHandler(logDest, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return cfg, nil
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

// runLoop probes repeatedly until the context is cancelled.
func runLoop(ctx context.Context, w io.Writer, cfg config) {
	fmt.Fprintf(w, "Running probe every %v (press Ctrl+C to stop)\n\n", cfg.loop)

	ticker := time.NewTicker(cfg.loop)
	defer ticker.Stop()

	for {
		// The return value is intentionally ignored: in loop mode a failing
		// probe is a result to report, not a reason to terminate.
		executeProbe(ctx, w, cfg)
		fmt.Fprintln(w, "---")

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// executeProbe runs a single probe and reports whether it succeeded. It never
// terminates the process, so callers stay in control of the exit behaviour.
func executeProbe(ctx context.Context, w io.Writer, cfg config) bool {
	switch cfg.command {
	case "ping":
		return handlePing(ctx, w, cfg)
	case "tcp":
		return handleTCP(ctx, w, cfg)
	case "tls":
		return handleTLS(ctx, w, cfg, false)
	case "tls-cert":
		return handleTLS(ctx, w, cfg, true)
	case "http":
		return handleHTTP(ctx, w, cfg)
	default:
		fmt.Fprintf(w, "Error: unknown command '%s'\n", cfg.command)
		return false
	}
}

// printSummary prints the fields every probe has in common.
func printSummary(w io.Writer, s probe.Summary) {
	fmt.Fprintf(w, "Attempt: %d\n", s.Attempts)
	fmt.Fprintf(w, "Success: %v\n", s.Success)
	if s.Successes > 1 || s.Failures > 0 {
		fmt.Fprintf(w, "Successes: %d  Failures: %d\n", s.Successes, s.Failures)
	}
	if s.Duration > 0 {
		fmt.Fprintf(w, "Duration: %s\n", ms(s.Duration))
	}
	if s.Successes > 1 {
		fmt.Fprintf(w, "Min/Avg/Max: %s / %s / %s\n", ms(s.Min), ms(s.Avg), ms(s.Max))
	}
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000.0)
}

func printError(w io.Writer, err error) bool {
	if err != nil {
		fmt.Fprintf(w, "Error: %v\n", err)
		return false
	}
	return true
}

func handlePing(ctx context.Context, w io.Writer, cfg config) bool {
	r := probeping.RunContext(ctx, cfg.target, cfg.opts)

	fmt.Fprintf(w, "Target: %s\n", r.Address)
	if r.ResolvedIP != "" && r.ResolvedIP != r.Address {
		fmt.Fprintf(w, "Resolved IP: %s\n", r.ResolvedIP)
	}
	printSummary(w, r.Summary)

	return printError(w, r.Error) && r.Success
}

func handleTCP(ctx context.Context, w io.Writer, cfg config) bool {
	r := probetcp.RunContext(ctx, cfg.target, cfg.opts)

	fmt.Fprintf(w, "Host: %s\n", r.Host)
	fmt.Fprintf(w, "Port: %s\n", r.Port)
	printSummary(w, r.Summary)

	if r.Success {
		fmt.Fprintf(w, "Local Address: %s\n", r.LocalAddr)
		fmt.Fprintf(w, "Remote Address: %s\n", r.RemoteAddr)
	}

	return printError(w, r.Error) && r.Success
}

func handleTLS(ctx context.Context, w io.Writer, cfg config, certOnly bool) bool {
	var r probetls.Result
	if certOnly {
		r = probetls.RunCertOnlyContext(ctx, cfg.target, cfg.opts)
	} else {
		r = probetls.RunContext(ctx, cfg.target, cfg.opts)
	}

	fmt.Fprintf(w, "Host: %s\n", r.Host)
	printSummary(w, r.Summary)

	// Printed whenever a certificate was obtained, not only on success: an
	// expired certificate is a failure, and its details are exactly what the
	// user needs to see.
	if r.Subject != "" {
		fmt.Fprintf(w, "Subject: %s\n", r.Subject)
		fmt.Fprintf(w, "Issuer: %s\n", r.Issuer)
		fmt.Fprintf(w, "Valid From: %s\n", r.NotBefore.Format(time.DateTime))
		fmt.Fprintf(w, "Expires At: %s\n", r.ExpiresAt.Format(time.DateTime))
		fmt.Fprintf(w, "Days Until Expiry: %d\n", r.DaysUntilExpiry)

		switch {
		case r.Expired:
			fmt.Fprintln(w, "Certificate Status: EXPIRED")
		case r.NotYetValid:
			fmt.Fprintln(w, "Certificate Status: NOT YET VALID")
		}

		// Only worth a line when the chain is the weaker part; otherwise it
		// would just repeat the leaf's expiry.
		if r.ChainLength > 1 && r.ChainExpiresAt.Before(r.ExpiresAt) {
			fmt.Fprintf(w, "Chain Expires At: %s (%d days, %d certificates presented)\n",
				r.ChainExpiresAt.Format(time.DateTime), r.ChainDaysUntilExpiry, r.ChainLength)
		}

		fmt.Fprintf(w, "TLS Version: %s\n", r.Protocol)
		fmt.Fprintf(w, "Cipher Suite: %s\n", r.CipherSuite)
		if certOnly && r.HandshakeError != nil {
			fmt.Fprintf(w, "Handshake Warning: %v\n", r.HandshakeError)
		}
	}

	return printError(w, r.Error) && r.Success
}

func handleHTTP(ctx context.Context, w io.Writer, cfg config) bool {
	r := probehttp.RunContext(ctx, cfg.target, cfg.opts)

	fmt.Fprintf(w, "URL: %s\n", r.URL)
	printSummary(w, r.Summary)
	fmt.Fprintf(w, "Status Code: %d\n", r.StatusCode)

	return printError(w, r.Error) && r.Success
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "╔════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(w, "║            Probe - Multi-Protocol Health Check              ║")
	fmt.Fprintln(w, "╚════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(w, "\nUsage: probe <command> [flags] <target>")
	fmt.Fprintln(w, "\nCommands:")
	fmt.Fprintln(w, "  ping              ICMP ping probe (IP or hostname)")
	fmt.Fprintln(w, "  tcp               TCP port connectivity check")
	fmt.Fprintln(w, "  tls               Strict TLS/SSL handshake and certificate check")
	fmt.Fprintln(w, "  tls-cert          TLS/SSL certificate info without requiring full handshake")
	fmt.Fprintln(w, "  http              HTTP/HTTPS status check")
	fmt.Fprintln(w, "\nFlags:")
	fmt.Fprintln(w, "  -attempts int      Maximum number of attempts (default: 3)")
	fmt.Fprintln(w, "  -threshold int     Successful attempts required to pass (default: 1)")
	fmt.Fprintln(w, "  -timeout duration  Timeout per attempt (default: 1s for ping/tcp, 5s for http/tls/tls-cert)")
	fmt.Fprintln(w, "  -interval duration Pause between attempts (default: 500ms)")
	fmt.Fprintln(w, "  -loop duration     Loop interval (0 = run once, e.g., 5s, 1m, 10s)")
	fmt.Fprintln(w, "  -size int          ICMP payload size in bytes, ping only (default: 56)")
	fmt.Fprintln(w, "  -debug             Log per-attempt detail to stderr")
	fmt.Fprintln(w, "\nFlags must come before the target.")
	fmt.Fprintln(w, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "\n1. ICMP Ping")
	fmt.Fprintln(w, "  probe ping 8.8.8.8")
	fmt.Fprintln(w, "  probe ping google.com")
	fmt.Fprintln(w, "  probe ping -attempts 5 -threshold 3 8.8.8.8      # 3 of up to 5 must answer")
	fmt.Fprintln(w, "  probe ping -size 1472 gateway.local              # path MTU check")
	fmt.Fprintln(w, "  probe ping -loop 5s 8.8.8.8                      # Run every 5 seconds")
	fmt.Fprintln(w, "\n2. TCP Port Connectivity Check")
	fmt.Fprintln(w, "  probe tcp example.com:22")
	fmt.Fprintln(w, "  probe tcp -attempts 5 google.com:443")
	fmt.Fprintln(w, "  probe tcp -loop 10s example.com:22               # Run every 10 seconds")
	fmt.Fprintln(w, "\n3. HTTP/HTTPS Status Check")
	fmt.Fprintln(w, "  probe http https://google.com")
	fmt.Fprintln(w, "  probe http -timeout 3s https://example.com")
	fmt.Fprintln(w, "  probe http -loop 1m https://google.com           # Run every 1 minute")
	fmt.Fprintln(w, "\n4. TLS/SSL Strict Check")
	fmt.Fprintln(w, "  probe tls google.com:443")
	fmt.Fprintln(w, "  probe tls -loop 30m google.com:443               # Run every 30 minutes")
	fmt.Fprintln(w, "\n5. TLS/SSL Certificate-Only Check")
	fmt.Fprintln(w, "  probe tls-cert google.com:443")
	fmt.Fprintln(w, "  probe tls-cert -timeout 10s mtls.example.com:443")
	fmt.Fprintln(w, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "Default Timeouts:")
	fmt.Fprintln(w, "  - ICMP Ping: 1 second per attempt")
	fmt.Fprintln(w, "  - TCP: 1 second per attempt")
	fmt.Fprintln(w, "  - HTTP/HTTPS: 1 second per attempt")
	fmt.Fprintln(w, "  - TLS/SSL: 5 seconds per attempt")
}
