//go:build windows

package probeping

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"syscall"
	"time"
	"unsafe"

	"github.com/netyazilim/probe"
)

// Windows ICMP structures.
//
// These mirror the Win32 definitions exactly; field widths matter because the
// reply buffer is reinterpreted in place.
//
//	typedef struct ip_option_information {
//	  UCHAR  Ttl; UCHAR Tos; UCHAR Flags; UCHAR OptionsSize; PUCHAR OptionsData;
//	} IP_OPTION_INFORMATION;
type IPOptionInformation struct {
	TTL         uint8
	TOS         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData uintptr
}

// IcmpEchoReply mirrors ICMP_ECHO_REPLY:
//
//	IPAddr Address; ULONG Status; ULONG RoundTripTime;
//	USHORT DataSize; USHORT Reserved; PVOID Data;
//	IP_OPTION_INFORMATION Options;
//
// DataSize and Reserved are USHORT (not ULONG); declaring them wider shifts
// Data and Options to the wrong offsets.
type IcmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32 // milliseconds, measured by the OS
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       IPOptionInformation
}

// Win32 API calls
var (
	iphlpapi            = syscall.NewLazyDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
)

func run(ctx context.Context, address string, opts probe.Options) Result {
	opts = opts.WithDefaults(DefaultTimeout)
	if err := opts.Validate(); err != nil {
		return Result{Summary: probe.Summary{Error: err}, Address: address}
	}

	result := Result{Address: address}

	// Resolve hostname to IP if necessary
	resolvedTarget := address
	if net.ParseIP(address) == nil {
		addrs, err := net.LookupHost(address)
		if err != nil || len(addrs) == 0 {
			result.Error = fmt.Errorf("DNS resolution failed: cannot resolve hostname: %v", err)
			return result
		}

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

	ip := net.ParseIP(resolvedTarget)
	if ip == nil {
		result.Error = fmt.Errorf("invalid IP address: %s", resolvedTarget)
		return result
	}
	ipv4 := ip.To4()
	if ipv4 == nil {
		result.Error = fmt.Errorf("only IPv4 is supported: %s", resolvedTarget)
		return result
	}
	result.ResolvedIP = resolvedTarget

	// Windows IcmpSendEcho expects the address in host byte order.
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

	timeoutMs := uint32(opts.Timeout.Milliseconds())
	if timeoutMs == 0 {
		timeoutMs = 1000 // the API works in whole milliseconds
	}

	// The Win32 API matches the reply for us, so the payload only has to give
	// the packet the requested size; it is built the same way as on Linux so
	// that both platforms put the same bytes on the wire.
	runID := uint16(rand.Uint32())
	var seq uint16

	opts.Logger.Debug("starting ICMP ping",
		"target", address, "resolved", resolvedTarget, "runID", runID, "size", opts.Size)

	stats := probe.Repeat(ctx, opts, func(ctx context.Context) (time.Duration, error) {
		seq++
		data := newPayload(runID, seq, opts.Size)
		dataSize := uint32(len(data))

		// Microsoft's documentation asks for one ICMP_ECHO_REPLY plus
		// RequestSize bytes of data, plus 8 more bytes to hold a possible ICMP
		// error message.
		replySize := uint32(unsafe.Sizeof(IcmpEchoReply{}) + uintptr(dataSize) + 8)
		replyBuf := make([]byte, replySize)

		start := time.Now()
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
		// Wall-clock duration around the syscall; used only as a fallback
		// below, because it also contains the syscall overhead.
		measured := time.Since(start)

		if ret == 0 {
			if err.Error() == "Error due to lack of resources." {
				return measured, fmt.Errorf("Windows ICMP API error: %v (Note: This may occur with certain network configurations or firewall settings)", err)
			}
			return measured, fmt.Errorf("IcmpSendEcho failed: %v", err)
		}
		if ret != 1 {
			return measured, fmt.Errorf("unexpected reply count: %d", ret)
		}

		reply := (*IcmpEchoReply)(unsafe.Pointer(&replyBuf[0]))
		if reply.Status != 0 {
			return measured, fmt.Errorf("ping failed with status code: %d", reply.Status)
		}

		result.BytesRecv = int(reply.DataSize)

		// Prefer the round-trip time reported by the OS over the locally
		// measured value, which additionally contains the syscall overhead.
		// Its resolution is one millisecond, so a sub-millisecond reply
		// reports 0; in that case keep the measured value, which is a usable
		// upper bound.
		if reply.RoundTripTime > 0 {
			return time.Duration(reply.RoundTripTime) * time.Millisecond, nil
		}
		return measured, nil
	})

	result.Summary = stats.Summarize(opts.SuccessThreshold)
	return result
}
