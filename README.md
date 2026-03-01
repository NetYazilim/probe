# Probe - Multi-Protocol Health Check

[Turkish README](README.TR.md) 



Multi-protocol health check library written in Go. Import as a package in your projects, or use the included CLI for simple checks.

> **Note:** The CLI tool in this repository is primarily designed to demonstrate the library's capabilities and perform simple checks. For a comprehensive detailed logging tool based on this library, please check out the **[NetYazilim/izle](https://github.com/NetYazilim/izle)** project.

## Features

- **ICMP Ping Probe** - Check host availability using ICMP Echo (supports both IP addresses and hostnames; IPv6 support available on Linux).
- **TCP Probe** - Verify port connectivity and connection information (works with both IPv4 and IPv6 targets).
- **HTTP/HTTPS Probe** - Verify web service health with status code checks (supports IPv4/IPv6 endpoints transparently).
- **TLS/SSL Probe** - Retrieve and validate SSL/TLS certificate information (Expiry Date, Days Remaining, Issuer, etc.) directly from the server over IPv4 or IPv6.
- **No external Go package dependencies - uses only Go standard library.**

## Platform Support

- **Linux** - Uses UDP-based ICMP (requires kernel support for unprivileged ping). Supports both IPv4 and IPv6.
- **Windows** - Uses Win32 ICMP API (no special privileges required). Currently supports IPv4 only for ICMP ping.

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

**To make the change persistent:**

```bash
echo "net.ipv4.ping_group_range = 0 2147483647" | sudo tee /etc/sysctl.d/99-ping.conf
sudo sysctl -p /etc/sysctl.d/99-ping.conf
```

## Installation

```bash
go get "github.com/netyazilim/probe"
```

## Usage

```bash
# Display help
./probe

# ICMP Ping (IP Addresses or Hostnames)
./probe ping 8.8.8.8
./probe ping google.com
./probe -attempts 5 ping 8.8.8.8
./probe -timeout 2s ping google.com
./probe -loop 5s ping 8.8.8.8                    # Run every 5 seconds

# TCP Probe (Port Connectivity)
./probe tcp example.com:22
./probe tcp google.com:443
./probe -attempts 5 tcp google.com:443
./probe -loop 10s tcp google.com:443             # Run every 10 seconds

# HTTP/HTTPS (URLs)
./probe http https://google.com
./probe -timeout 3s http https://example.com
./probe -loop 1m http https://google.com         # Run every 1 minute

# TLS/SSL Certificate Info
./probe tls google.com:443
./probe -timeout 10s tls example.com:443
./probe -loop 30m tls google.com:443             # Run every 30 minutes
```

## Flags

```
-attempts int     Maximum number of attempts (default: 3)
-timeout duration Timeout per attempt (default: 1s for ping/tcp/http, 5s for tls)
-loop duration    Loop interval (0 = run once, e.g., 5s, 1m, 10s)
```

## Commands

```
ping               ICMP ping probe (supports IP addresses and hostnames)
tcp                TCP port connectivity check (host:port format)
http               HTTP/HTTPS status check (URL format)
tls                TLS/SSL certificate information (host:port format)
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
```

### Docker Note
The container runs as an unprivileged user (`probeuser`).

## Output Examples

### ICMP Ping
```
Target: 8.8.8.8
Attempt: 3
Success: true
Duration: 45.32 ms
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

### TLS/SSL
```
Host: google.com
Attempt: 3
Success: true
Duration: 235.67 ms
Subject: CN=www.google.com
Issuer: CN=Google Internet Authority G3
Expires At: 2025-12-15 23:59:59
Days Until Expiry: 310
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
```


## Dependencies

This package uses only standard Go library packages:
No external dependencies required.

## Requirements

- Go 1.20 or later
- For ICMP Ping on Linux:
  - Check kernel support: `cat /proc/sys/net/ipv4/ping_group_range`
  - Unprivileged ICMP must be enabled in kernel
  - To enable: `sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"`
- Internet connectivity for TCP and TLS probes

## Error Handling

All probes include retry logic (3 attempts by default) with 500ms delays between attempts. Detailed error messages are provided for troubleshooting.
