//go:build integration

package lvs

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Flush all IPVS rules before running tests to ensure a clean state.
	handle, err := NewIPVSHandle("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create IPVS handle for pre-test flush: %v\n", err)
		os.Exit(1)
	}
	if err := handle.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush IPVS rules before tests: %v\n", err)
		handle.Close()
		os.Exit(1)
	}
	handle.Close()

	code := m.Run()

	// Flush all IPVS rules after running tests to leave a clean state.
	handle, err = NewIPVSHandle("")
	if err == nil {
		handle.Flush()
		handle.Close()
	}

	os.Exit(code)
}
