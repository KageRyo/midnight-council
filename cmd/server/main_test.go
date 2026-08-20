package main

import "testing"

func TestListenAddress(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		port    string
		want    string
		wantErr bool
	}{
		{name: "default", want: ":8080"},
		{name: "port", port: "10000", want: ":10000"},
		{name: "addr takes precedence", addr: ":9090", port: "10000", want: ":9090"},
		{name: "invalid port", port: "not-a-port", wantErr: true},
		{name: "zero port", port: "0", wantErr: true},
		{name: "port too high", port: "65536", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ADDR", test.addr)
			t.Setenv("PORT", test.port)

			got, err := listenAddress()
			if (err != nil) != test.wantErr {
				t.Fatalf("listenAddress() error = %v, want error = %t", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("listenAddress() = %q, want %q", got, test.want)
			}
		})
	}
}
