package cmd

import "testing"

func TestGuardBindAddr(t *testing.T) {
	cases := []struct {
		bind          string
		allowInsecure bool
		wantErr       bool
	}{
		{"127.0.0.1:7777", false, false},
		{"localhost:7777", false, false},
		{"[::1]:7777", false, false},
		{"0.0.0.0:7777", false, true},
		{":7777", false, true}, // empty host = all interfaces
		{"192.168.1.10:7777", false, true},
		{"0.0.0.0:7777", true, false}, // explicit override
		{"not-a-host-port", false, true},
	}
	for _, c := range cases {
		err := guardBindAddr(c.bind, c.allowInsecure)
		if (err != nil) != c.wantErr {
			t.Errorf("guardBindAddr(%q, %v) err=%v, wantErr=%v", c.bind, c.allowInsecure, err, c.wantErr)
		}
	}
}
