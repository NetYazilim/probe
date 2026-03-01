package ping

// This package contains platform-specific implementations:
//
// ping_linux.go  - ICMP ping for Linux (UDP-based)
//                  Requires CAP_NET_RAW capability or root privileges
//
// ping_windows.go - ICMP ping for Windows (Win32 ICMP API)
//                   Does NOT require Administrator privileges
//
// Run() function provides the same interface on all platforms
