//go:build windows
// +build windows

package ping

import (
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"
)

// Result holds the ping result
type Result struct {
	Address    string
	ResolvedIP string // The resolved IPv4 address (may be same as Address if IP was provided)
	Success    bool
	Attempts   int
	BytesRecv  int
	Error      error
	Duration   time.Duration
}

// Windows ICMP structures
type IcmpEchoReply struct {
	Address       uint32
	Status        int32
	RoundTripTime uint32
	DataSize      uint32
	Reserved      uint32
	Data          uintptr
	Options       uintptr
}

// Win32 API calls
var (
	iphlpapi            = syscall.NewLazyDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
)

// Run performs an ICMP ping to the specified address using Windows ICMP API.
// address can be an IP address or a hostname.
// This method does NOT require Administrator privileges.
// maxAttempts specifies the maximum number of ping attempts.
// timeout specifies the maximum duration to wait for a response.
// Returns a Result struct containing the results of the ping operation.
func Run(address string, maxAttempts int, timeout time.Duration) Result {
	result := Result{
		Address:  address,
		Attempts: maxAttempts,
	}

	// Resolve hostname to IP if necessary
	resolvedTarget := address
	if net.ParseIP(address) == nil {
		// Target is not an IP address, try to resolve it as a hostname
		// Use LookupHost which returns string IPs, then filter for IPv4
		addrs, err := net.LookupHost(address)
		if err != nil || len(addrs) == 0 {
			result.Error = fmt.Errorf("DNS resolution failed: cannot resolve hostname: %v", err)
			return result
		}

		// Find first IPv4 address
		var ipv4Addr string
		for _, addr := range addrs {
			if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil {
				ipv4Addr = addr
				break
			}
		}

		if ipv4Addr == "" {
			result.Error = fmt.Errorf("no IPv4 address found for hostname: %s (IPv6 ping is not yet supported on Windows)", address)
			return result
		}

		resolvedTarget = ipv4Addr
	}

	// Parse resolved IP address
	ip := net.ParseIP(resolvedTarget)
	if ip == nil {
		result.Error = fmt.Errorf("invalid IP address: %s", resolvedTarget)
		return result
	}

	// Convert to IPv4
	ipv4 := ip.To4()
	if ipv4 == nil {
		result.Error = fmt.Errorf("only IPv4 is supported: %s", resolvedTarget)
		return result
	}

	// Store resolved IP in result
	result.ResolvedIP = resolvedTarget

	// Convert IP address to uint32 (host byte order for Windows API)
	// Windows IcmpSendEcho expects IP in host byte order (little-endian on x86/x64)
	ipAddr := uint32(ipv4[3])<<24 | uint32(ipv4[2])<<16 | uint32(ipv4[1])<<8 | uint32(ipv4[0])

	// Create ICMP handle once for all attempts
	hIcmpFile, _, createErr := procIcmpCreateFile.Call()
	if hIcmpFile == 0 {
		result.Error = fmt.Errorf("IcmpCreateFile failed: %v (Windows ICMP API error)", createErr)
		return result
	}
	defer func() {
		_, _, _ = procIcmpCloseHandle.Call(hIcmpFile)
	}()

	// Prepare ping data
	data := []byte("PROBE-DATA")
	dataSize := uint32(len(data))

	// Convert timeout to milliseconds
	timeoutMs := uint32(timeout.Milliseconds())
	if timeoutMs == 0 {
		timeoutMs = 1000 // Minimum 1 second
	}

	for i := 0; i < maxAttempts; i++ {
		result.Attempts = i + 1

		startTime := time.Now()

		// Call IcmpSendEcho
		replySize := uint32(unsafe.Sizeof(IcmpEchoReply{}) + uintptr(dataSize))
		replyBuf := make([]byte, replySize)

		ret, _, err := procIcmpSendEcho.Call(
			hIcmpFile,
			uintptr(ipAddr),
			uintptr(unsafe.Pointer(&data[0])),
			uintptr(dataSize),
			0, // IpOptionInformation (NULL)
			uintptr(unsafe.Pointer(&replyBuf[0])),
			uintptr(replySize),
			uintptr(timeoutMs),
		)

		duration := time.Since(startTime)
		result.Duration = duration

		if ret == 0 {
			// Provide more informative error message for common Windows ICMP errors
			errMsg := err.Error()
			if errMsg == "Error due to lack of resources." {
				result.Error = fmt.Errorf("Windows ICMP API error: %v (Note: This may occur with certain network configurations or firewall settings)", err)
			} else {
				result.Error = fmt.Errorf("IcmpSendEcho failed: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if ret != 1 {
			result.Error = fmt.Errorf("unexpected reply count: %d", ret)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Parse reply
		reply := (*IcmpEchoReply)(unsafe.Pointer(&replyBuf[0]))

		if reply.Status != 0 {
			result.Error = fmt.Errorf("ping failed with status code: %d", reply.Status)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		result.Success = true
		result.BytesRecv = int(reply.DataSize)
		return result
	}

	return result
}
