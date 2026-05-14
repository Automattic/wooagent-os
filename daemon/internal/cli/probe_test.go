package cli

import (
	"net"
	"strings"
	"testing"
)

// probeBindAddr should return nil when the port is free and a helpful
// error when another listener is holding the port. The error must
// surface both the address (for context) and a recovery hint.
func TestProbeBindAddr_FreePortReturnsNil(t *testing.T) {
	// Bind ":0" lets the OS pick a free port; capture it, release, then
	// probe — the probe should find the port free and return nil.
	picker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	addr := picker.Addr().String()
	_ = picker.Close()

	if err := probeBindAddr(addr); err != nil {
		t.Errorf("probeBindAddr on free port: got %v, want nil", err)
	}
}

func TestProbeBindAddr_BusyPortReturnsError(t *testing.T) {
	// Hold a listener open while the probe runs. Probe should fail.
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("seed listener: %v", err)
	}
	defer holder.Close()
	addr := holder.Addr().String()

	err = probeBindAddr(addr)
	if err == nil {
		t.Fatalf("probeBindAddr on busy port: got nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, addr) {
		t.Errorf("error %q should mention addr %q", msg, addr)
	}
	if !strings.Contains(msg, "lsof") {
		t.Errorf("error %q should hint at lsof recovery command", msg)
	}
}

func TestPortFromAddr(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"localhost:7777", "7777"},
		{"127.0.0.1:7777", "7777"},
		{":7777", "7777"},
		{"not-a-host-port", "not-a-host-port"}, // graceful fallback
	}
	for _, tc := range cases {
		got := portFromAddr(tc.in)
		if got != tc.want {
			t.Errorf("portFromAddr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
