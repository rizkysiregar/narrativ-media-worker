package config

import "testing"

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		secure bool
	}{
		{"http://localhost:9000", "localhost:9000", false},
		{"https://minio.example.com", "minio.example.com", true},
		{"minio:9000", "minio:9000", false},
	}
	for _, c := range cases {
		host, secure := parseEndpoint(c.in)
		if host != c.host || secure != c.secure {
			t.Errorf("parseEndpoint(%q) = %q,%v want %q,%v", c.in, host, secure, c.host, c.secure)
		}
	}
}
