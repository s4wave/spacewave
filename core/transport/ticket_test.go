package transport

import "testing"

func TestSignalURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "relative root",
			base: "/",
			want: "/api/signal/ticket",
		},
		{
			name: "absolute without trailing slash",
			base: "https://cloud.example",
			want: "https://cloud.example/api/signal/ticket",
		},
		{
			name: "absolute with trailing slash",
			base: "https://cloud.example/",
			want: "https://cloud.example/api/signal/ticket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := signalURL(tt.base, "/api/signal/ticket")
			if err != nil {
				t.Fatal(err)
			}
			if got := u.String(); got != tt.want {
				t.Fatalf("signalURL(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestSignalWebSocketURL(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		ticket string
		want   string
	}{
		{
			name:   "relative root",
			base:   "/",
			ticket: "ticket /?&",
			want:   "/api/signal/ws?tk=ticket+%2F%3F%26",
		},
		{
			name:   "absolute without trailing slash",
			base:   "https://cloud.example",
			ticket: "ticket",
			want:   "wss://cloud.example/api/signal/ws?tk=ticket",
		},
		{
			name:   "absolute with trailing slash",
			base:   "https://cloud.example/",
			ticket: "ticket",
			want:   "wss://cloud.example/api/signal/ws?tk=ticket",
		},
		{
			name:   "http uses ws",
			base:   "http://cloud.example",
			ticket: "ticket",
			want:   "ws://cloud.example/api/signal/ws?tk=ticket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := signalWebSocketURL(tt.base, tt.ticket)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("signalWebSocketURL(%q, %q) = %q, want %q", tt.base, tt.ticket, got, tt.want)
			}
		})
	}
}
