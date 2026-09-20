//go:build !linux && !windows

package probeping

import (
	"context"
	"fmt"
	"runtime"

	"github.com/netyazilim/probe"
)

// run is not implemented on this platform.
//
// ICMP ping is currently supported on Linux (unprivileged datagram ICMP
// socket) and Windows (Win32 ICMP API) only. Everywhere else it reports an
// unsuccessful Result with an explanatory error, so that packages importing
// this module still build and the probetcp, probehttp and probetls probes
// remain usable.
func run(ctx context.Context, address string, opts probe.Options) Result {
	return Result{
		Summary: probe.Summary{
			Error: fmt.Errorf("ICMP ping is not supported on %s (supported platforms: linux, windows)", runtime.GOOS),
		},
		Address: address,
	}
}
