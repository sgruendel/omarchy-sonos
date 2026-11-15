package sonos

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
