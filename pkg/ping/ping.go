package ping

// This package contains platform-specific implementations:
//
// ping_linux.go   - ICMP ping for Linux, over an UNPRIVILEGED datagram ICMP
//                   socket (SOCK_DGRAM + IPPROTO_ICMP, opened with network
//                   "udp4"/"udp6"). This is NOT a raw socket: it needs neither
//                   root nor the CAP_NET_RAW capability. The kernel gates it
//                   with net.ipv4.ping_group_range, which must cover the gid
//                   the process runs as.
//
// ping_windows.go - ICMP ping for Windows (Win32 ICMP API, iphlpapi.dll)
//                   Does NOT require Administrator privileges
//
// ping_other.go   - Every other platform: Run reports that ICMP ping is not
//                   supported there, so this package still builds and the tcp,
//                   http and tls probes remain usable.
//
// Run() function provides the same interface on all platforms
