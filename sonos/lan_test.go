package sonos

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSpeakerRequestsStayOnTheLAN(t *testing.T) {
	// 203.0.113.0/24 is reserved for documentation; the dialer must refuse it
	// before connecting, whether it names a speaker or a topology member.
	if _, err := New().Describe(t.Context(), "http://203.0.113.7:1400/xml/device_description.xml"); !errors.Is(err, ErrNotLocal) {
		t.Fatalf("public speaker address: %v", err)
	}
	// A real loopback server, since the in-memory test network has no IPs.
	followed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed = true
		}
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := New().Describe(t.Context(), srv.URL+"/xml/device_description.xml"); err == nil || !strings.Contains(err.Error(), "HTTP 302") || followed {
		t.Fatalf("speaker redirect: err=%v, followed=%v", err, followed)
	}
}

func TestLocalityCheckAcceptsScopedIPv6(t *testing.T) {
	for _, tt := range []struct {
		host  string
		local bool
	}{
		{"192.168.1.3", true},
		{"10.0.0.2", true},
		{"127.0.0.1", true},
		{"169.254.10.1", true},
		{"fe80::1%eth0", true}, // Scoped link-local, as Dialer.Control sees it.
		{"fe80::1", true},
		{"fd00::1", true},
		{"::1", true},
		{"::ffff:192.168.1.3", true},
		{"::ffff:8.8.8.8", false},
		{"203.0.113.7", false},
		{"2001:db8::1%eth0", false},
		{"speaker.local", false},
		{"", false},
	} {
		if _, local := localAddr(tt.host); local != tt.local {
			t.Errorf("localAddr(%q) = %v, want %v", tt.host, local, tt.local)
		}
	}
	// Control receives the resolved address with its zone and must not alter it.
	control := localDialer(time.Second).Control
	if err := control("tcp6", "[fe80::1%eth0]:1400", nil); err != nil {
		t.Fatalf("scoped link-local speaker refused: %v", err)
	}
	if err := control("tcp6", "[2001:db8::1]:1400", nil); !errors.Is(err, ErrNotLocal) {
		t.Fatalf("public IPv6 accepted: %v", err)
	}
}

func TestHostLocationRoundTripsCachedHosts(t *testing.T) {
	for _, tt := range []struct{ host, want string }{
		{"192.168.1.3", "http://192.168.1.3:1400/xml/device_description.xml"},
		{"::ffff:192.168.1.3", "http://192.168.1.3:1400/xml/device_description.xml"},
		{"fd00::1", "http://[fd00::1]:1400/xml/device_description.xml"},
		{"fe80::1%eth0", "http://[fe80::1%25eth0]:1400/xml/device_description.xml"},
	} {
		got, err := HostLocation(tt.host)
		if err != nil || got != tt.want {
			t.Fatalf("HostLocation(%q) = %q, %v; want %q", tt.host, got, err, tt.want)
		}
		// Discovery caches u.Hostname() of a speaker URL and feeds it back here.
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("HostLocation(%q) is not a valid URL: %v", tt.host, err)
		}
		if again, err := HostLocation(u.Hostname()); err != nil || again != got {
			t.Fatalf("cached host %q does not round-trip: %q, %v", u.Hostname(), again, err)
		}
	}
}
