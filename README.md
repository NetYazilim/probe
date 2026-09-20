# Probe - Multi-Protocol Health Check

[Turkish README](README.TR.MD)



Multi-protocol health check library written in Go. Import as a package in your projects, or use the included CLI for simple checks.

> **Note:** The CLI tool in this repository is primarily designed to demonstrate the library's capabilities and perform simple checks. For a comprehensive detailed logging tool based on this library, please check out the **[NetYazilim/izle](https://github.com/NetYazilim/izle)** project.

## Features

- **ICMP Ping Probe** - Check host availability using ICMP Echo (supports both IP addresses and hostnames; IPv6 support available on Linux).
- **TCP Probe** - Verify port connectivity and connection information (works with both IPv4 and IPv6 targets).
- **HTTP/HTTPS Probe** - Verify web service health with status code checks (supports IPv4/IPv6 endpoints transparently).
- **TLS/SSL Probe** - Supports two modes over IPv4 or IPv6: strict TLS validation (`tls`) and certificate-only inspection (`tls-cert`). The certificate-only mode can still read the presented server certificate even when full TLS negotiation is blocked by mTLS/client-certificate requirements.
- **Success threshold** - decide how many of the attempts have to answer. A probe stops as soon as the threshold is met, and equally as soon as the remaining attempts can no longer reach it.
- **Cancellable** - every probe takes a `context.Context` and honours it while dialling, handshaking and waiting for a reply, not just between attempts.
- **Configurable ICMP payload size** - `-size` makes ping usable for path MTU checks, and keeps packets to different targets the same size so their round-trip times are comparable.
- **Minimal dependencies** - `golang.org/x/net` is required for ICMP ping on Linux only; the TCP, HTTP and TLS probes, and the Windows and macOS builds, use the Go standard library alone.

## Platform Support

- **Linux** - Uses UDP-based ICMP (requires kernel support for unprivileged ping). Supports both IPv4 and IPv6.
- **Windows** - Uses Win32 ICMP API (no special privileges required). Currently supports IPv4 only for ICMP ping.
- **macOS and other platforms** - ICMP ping is not implemented and returns an explicit error; the TCP, HTTP and TLS probes work normally.

### Linux ICMP Ping Configuration

On Linux, the kernel must allow unprivileged ICMP ping. Check if it's enabled:

```bash
cat /proc/sys/net/ipv4/ping_group_range
```

**Output meanings:**
- `1 0` - ICMP ping disabled for unprivileged users
- `0 2147483647` - All users can perform ICMP ping without root

**To enable unprivileged ICMP ping:**

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
```

> On a systemd-based distribution this is usually already the default: systemd
> ships `net.ipv4.ping_group_range = 0 2147483647` in its
> `sysctl.d/50-default.conf`, under the comment *"ping(8) without CAP_NET_ADMIN
> and CAP_NET_RAW"*. Where you are likely to need the command is a container or
> a minimal image that never runs `systemd-sysctl`, which therefore keeps the
> kernel default `1 0` (disabled). Check first rather than assuming either way.

**To make the change persistent:**

```bash
echo "net.ipv4.ping_group_range = 0 2147483647" | sudo tee /etc/sysctl.d/99-ping.conf
sudo sysctl -p /etc/sysctl.d/99-ping.conf
```

## Installation

```bash
go get "github.com/netyazilim/probe"
```

## Library usage

Each probe lives in its own package and exposes the same pair of entry points:
`RunContext` for new code, and `Run` for callers written against the original
API.

```go
import (
    "context"
    "time"

    "github.com/netyazilim/probe"
    "github.com/netyazilim/probe/pkg/probeping"
    "github.com/netyazilim/probe/pkg/probetcp"
    "github.com/netyazilim/probe/pkg/probehttp"
    "github.com/netyazilim/probe/pkg/probetls"
)

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

r := probeping.RunContext(ctx, "8.8.8.8", probe.Options{
    MaxAttempts:      5,
    SuccessThreshold: 3,               // 3 of up to 5 must answer
    Timeout:          time.Second,     // per attempt
    Interval:         200 * time.Millisecond,
    Size:             1472,            // ICMP payload bytes
    Logger:           logger,          // *slog.Logger, may be nil
})

if r.Success {
    fmt.Println(r.Duration, r.Min, r.Avg, r.Max)
}
```

### Options

The zero value is usable: every field left at zero takes its default.

| Field | Default | Meaning |
|---|---|---|
| `MaxAttempts` | 3 | Upper bound on attempts, not a target |
| `SuccessThreshold` | 1 | Successful attempts needed to pass; must not exceed `MaxAttempts` |
| `Timeout` | 1s (5s for TLS) | Bounds a single attempt, not the run |
| `Interval` | 500ms | Pause between attempts |
| `Size` | 56 | ICMP payload bytes; ignored by the other probes |
| `Logger` | discard | `*slog.Logger` receiving per-attempt detail |

### Result

Every `Result` embeds `probe.Summary`, so these fields mean the same thing and
are reached the same way whichever probe produced them:

| Field | Meaning |
|---|---|
| `Success` | Whether `SuccessThreshold` was reached |
| `Attempts`, `Successes`, `Failures` | How many attempts were made and how they went |
| `Duration` | The most recent successful attempt |
| `Min`, `Avg`, `Max` | Across successful attempts; all equal `Duration` when only one succeeded |
| `Error` | The last error seen; `nil` when `Success` is true |

Each package adds its own fields on top: `Address`/`ResolvedIP`/`BytesRecv` for
ping, `Host`/`Port`/`LocalAddr`/`RemoteAddr` for tcp, `URL`/`StatusCode` for
http, and the certificate details for tls.

### Backwards compatibility

`Run(target, maxAttempts, timeout)` still exists in every package and behaves
as before, so existing call sites compile unchanged. Only the import paths
moved: `pkg/http` and `pkg/tls` shadowed the standard library and forced an
alias on every caller, so the packages are now `pkg/probeping`, `pkg/probetcp`,
`pkg/probehttp` and `pkg/probetls`.

## Usage

```bash
# Display help
./probe

# ICMP Ping (IP Addresses or Hostnames)
./probe ping 8.8.8.8
./probe ping google.com
./probe ping -attempts 5 8.8.8.8
./probe ping -attempts 5 -threshold 3 8.8.8.8     # 3 of up to 5 must answer
./probe ping -size 1472 gateway.local             # path MTU check
./probe ping -timeout 2s google.com
./probe ping -debug 8.8.8.8                       # per-attempt detail on stderr
./probe ping -loop 5s 8.8.8.8                     # Run every 5 seconds

# TCP Probe (Port Connectivity)
./probe tcp example.com:22
./probe tcp google.com:443
./probe tcp -attempts 5 google.com:443
./probe tcp -loop 10s google.com:443             # Run every 10 seconds

# HTTP/HTTPS (URLs)
./probe http https://google.com
./probe http -timeout 3s https://example.com
./probe http -loop 1m https://google.com         # Run every 1 minute

# TLS/SSL Strict Check
./probe tls google.com:443
./probe tls -timeout 10s example.com:443
./probe tls -loop 30m google.com:443             # Run every 30 minutes

# TLS/SSL Certificate-Only Check
./probe tls-cert google.com:443
./probe tls-cert https://google.com
./probe tls-cert -timeout 10s mtls.example.com:443
```

## Flags

```
-attempts int      Maximum number of attempts (default: 3)
-threshold int     Successful attempts required to pass (default: 1)
-timeout duration  Timeout per attempt (default: 1s for ping/tcp/http, 5s for tls/tls-cert)
-interval duration Pause between attempts (default: 500ms)
-loop duration     Loop interval (0 = run once, e.g., 5s, 1m, 10s)
-size int          ICMP payload size in bytes, ping only (default: 56)
-debug             Log per-attempt detail to stderr
```

**Flags must come before the target.** Go's flag parser stops at the first
non-flag argument, so anything written after the target would otherwise be
ignored silently; `probe` rejects it instead:

```
$ probe tcp example.com:22 -attempts 5
Error: unexpected argument(s) after the target: -attempts 5
Flags must come before the target, e.g. probe tcp -attempts 5 example.com:22
```

## Commands

```
ping               ICMP ping probe (supports IP addresses and hostnames)
tcp                TCP port connectivity check (host:port format)
http               HTTP/HTTPS status check (URL format)
tls                Strict TLS/SSL handshake and certificate validation (host:port or URL format)
tls-cert           TLS/SSL certificate inspection without requiring a full handshake (host:port or URL format)
```

## Timeout Configuration

- **ICMP Ping**: 1 second per attempt
- **TCP**: 1 second per attempt
- **HTTP/HTTPS**: 1 second per attempt
- **TLS/SSL**: 5 seconds per attempt

Each probe supports 3 retry attempts by default.

## Docker Usage

```bash
# Build Docker image
docker build -t probe .

# Run container
docker run --rm probe ping 8.8.8.8
docker run --rm probe tcp google.com:443
docker run --rm probe http https://google.com
docker run --rm probe tls google.com:443
docker run --rm probe tls-cert mtls.example.com:443
```

### Docker Note
The container runs as an unprivileged user (`probeuser`).

## Output Examples

### ICMP Ping
```
Target: 8.8.8.8
Attempt: 1
Success: true
Duration: 45.32 ms
```

`Attempt` is how many attempts were actually made. With a threshold above one,
or after a failed attempt, the counts and the spread are reported too:

```
$ probe ping -attempts 5 -threshold 3 8.8.8.8
Target: 8.8.8.8
Attempt: 4
Success: true
Successes: 3  Failures: 1
Duration: 45.32 ms
Min/Avg/Max: 44.90 ms / 45.60 ms / 46.51 ms
```

### TCP Probe
```
Host: google.com
Port: 443
Attempt: 3
Success: true
Duration: 125.45 ms
Local Address: 192.168.1.100:52341
Remote Address: 142.251.32.14:443
```

### HTTP/HTTPS
```
URL: https://google.com
Attempt: 3
Success: true
Status Code: 200
Duration: 125.45 ms
```

### TLS/SSL (strict)
```
Host: google.com
Attempt: 3
Success: true
Duration: 235.67 ms
Subject: CN=www.google.com
Issuer: CN=Google Internet Authority G3
Expires At: 2027-07-28 23:59:59
Days Until Expiry: 310
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
```

### TLS/SSL (certificate-only)
```
Host: mtls.example.com
Attempt: 1
Success: true
Duration: 210.12 ms
Subject: CN=mtls.example.com
Issuer: CN=Example Issuing CA
Expires At: 2027-04-22 23:59:59
Days Until Expiry: 213
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
Handshake Warning: remote error: tls: certificate required
```


## Dependencies

| Package | Required for | Platforms |
|---|---|---|
| `golang.org/x/net` (`icmp`, `ipv4`, `ipv6`) | ICMP ping | Linux only |

Everything else — the TCP, HTTP and TLS probes — uses the Go standard library
alone. On Windows the ICMP ping is implemented against the Win32 `iphlpapi.dll`
API and needs no external package either, and on other platforms ICMP ping is
not implemented, so `golang.org/x/net` is pulled in by the Linux build only.

`golang.org/x/sys` appears in `go.mod` as an indirect dependency of
`golang.org/x/net`.

## Requirements

- Go 1.26 or later (the version declared in `go.mod`)
- For ICMP Ping on Linux:
  - Check kernel support: `cat /proc/sys/net/ipv4/ping_group_range`
  - Unprivileged ICMP must be enabled in kernel
  - To enable: `sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"` (already the default on most systemd-based distributions; typically needed in containers and minimal images)
- Internet connectivity for TCP and TLS probes

## Error Handling

All probes retry up to `-attempts` times (3 by default) with a `-interval`
pause between attempts (500 ms by default). A probe stops as soon as
`-threshold` attempts have succeeded — with the default threshold of 1, that is
the first success — and equally as soon as the attempts still remaining cannot
reach the threshold, since there is nothing further to learn by continuing.
Detailed error messages are provided for troubleshooting.

### Exit codes

| Mode | Behaviour |
|---|---|
| Single run | `0` when the probe succeeds, `1` when it fails |
| Loop (`-loop`) | A failing round is reported and the loop keeps going; the process ends only on `Ctrl+C` |

`Ctrl+C` cancels the probe in flight rather than only killing the process, and
in loop mode it returns immediately instead of waiting for the next tick. A
second `Ctrl+C` terminates at once.

Invalid arguments are rejected before the first probe runs, with exit code `1`:
`-attempts` and `-threshold` must be at least 1, `-threshold` must not exceed
`-attempts`, `-timeout`, `-interval`, `-loop` and `-size` must not be negative,
an unknown command is reported once instead of on every iteration, and any
argument after the target is refused.

### How the round-trip time is measured

| Platform | Source |
|---|---|
| Linux | `time.Since`, started immediately before the send and read as soon as the reply arrives. It uses Go's monotonic clock, so wall-clock changes — NTP steps, manual changes, VM restore — cannot distort or negate the result. Being measured inside the process, it also contains the send and receive system-call overhead. |
| Windows | `ICMP_ECHO_REPLY.RoundTripTime`, the value the Win32 ICMP API itself reports. Its resolution is 1 ms; a reply faster than that reports 0, and the locally measured duration is used instead. |

The two are close but not identical, because they are taken at different
points: the Linux figure is measured around the system calls, the Windows one
is produced by the operating system. Compare values across platforms by
magnitude, not to the microsecond.
