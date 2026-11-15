package sonos

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestCallErrorKeepsCauseWithoutURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	speaker := Speaker{URL: "http://" + ln.Addr().String(), Services: map[string]Service{"RenderingControl": {Type: "urn:RenderingControl:1", Control: "/control"}}}
	_ = ln.Close() // Nothing listens on the port any more.
	_, err = New().Call(t.Context(), speaker, "RenderingControl", "GetVolume", nil)
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("cause not wrapped: %v", err)
	}
	if msg := err.Error(); !strings.HasPrefix(msg, "Sonos GetVolume request failed: ") || strings.Contains(msg, "http://") || strings.Contains(msg, "/control") {
		t.Fatalf("error should name the action and cause but not the URL: %q", msg)
	}
}
