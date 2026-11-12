package main

import "testing"

func TestResolveListenAddr(t *testing.T) {
	cases := []struct {
		name       string
		addr       string
		hasToken   bool
		wantListen string
		wantErr    bool
	}{
		{"empty host defaults loopback", ":8080", false, "127.0.0.1:8080", false},
		{"empty addr rejected", "", false, "", true},
		{"missing port rejected", "foo", false, "", true},
		{"empty port rejected", "127.0.0.1:", false, "", true},
		{"explicit loopback ok", "127.0.0.1:9000", false, "127.0.0.1:9000", false},
		{"localhost ok", "localhost:9000", false, "localhost:9000", false},
		{"ipv6 loopback ok", "[::1]:9000", false, "[::1]:9000", false},
		{"wildcard no token rejected", "0.0.0.0:8080", false, "", true},
		{"wildcard with token ok", "0.0.0.0:8080", true, "0.0.0.0:8080", false},
		{"public ip no token rejected", "203.0.113.7:8080", false, "", true},
		{"public ip with token ok", "203.0.113.7:8080", true, "203.0.113.7:8080", false},
		{"hostname no token rejected", "gitea.internal:8080", false, "", true},
		{"hostname with token ok", "gitea.internal:8080", true, "gitea.internal:8080", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listen, _, err := resolveListenAddr(c.addr, c.hasToken)
			if c.wantErr {
				if err == nil {
					t.Fatalf("resolveListenAddr(%q, %v) = %q, want error", c.addr, c.hasToken, listen)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveListenAddr(%q, %v): %v", c.addr, c.hasToken, err)
			}
			if listen != c.wantListen {
				t.Fatalf("resolveListenAddr(%q, %v) = %q, want %q", c.addr, c.hasToken, listen, c.wantListen)
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "localhost": true,
		"LOCALHOST.": true, "foo.localhost": true,
		"0.0.0.0": false, "192.168.1.1": false, "example.com": false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Fatalf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}
