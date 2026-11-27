//go:build !js

package kernel

import (
	"context"
	"os"
	"os/signal"
)

// RunInteractive runs a guest program like RunProcess, and forwards the host
// process's SIGINT to the kernel's foreground processes instead of letting it
// end the host. A host terminal in cooked mode turns Ctrl-C into SIGINT. In
// raw mode the console input reads Ctrl-C as a byte and signals the same way.
func (k *Kernel) RunInteractive(ctx context.Context, wasm []byte, args ...string) error {
	// Forward the host's SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			k.Signal(SIGINT)
		}
	}()

	// Run the program.
	return k.RunProcess(ctx, wasm, args...)
}
