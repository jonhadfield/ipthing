package main

import "testing"

func TestFamilyHostHint(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"ipv4.ipthing.net", "ipv4"},
		{"IPV4.ipthing.net", "ipv4"},
		{"ipv4.ipthing.net:443", "ipv4"},
		{"ipv6.ipthing.net", "ipv6"},
		{"ipv6.example.com", "ipv6"},
		{"ipthing.net", ""},
		{"www.ipthing.net", ""},
		{"notipv4.ipthing.net", ""},
	}
	for _, tt := range tests {
		if got := familyHostHint(tt.host); got != tt.want {
			t.Fatalf("familyHostHint(%q)=%q want %q", tt.host, got, tt.want)
		}
	}
}

func TestLocMapURL(t *testing.T) {
	got := locMapURL("32.7831,-96.8067")
	want := "https://www.openstreetmap.org/?mlat=32.7831&mlon=-96.8067#map=10/32.7831/-96.8067"
	if got != want {
		t.Fatalf("locMapURL got %q want %q", got, want)
	}
	if locMapURL("not-a-coord") != "" {
		t.Fatal("expected empty for invalid loc")
	}
	if locMapURL("") != "" {
		t.Fatal("expected empty for blank loc")
	}
}
