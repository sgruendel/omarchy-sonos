package sonos

import (
	"net"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchRepeatsMSearchAndCollectsUniqueLocations(t *testing.T) {
	loopback := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	speaker, err := net.ListenUDP("udp4", loopback)
	if err != nil {
		t.Fatal(err)
	}
	defer speaker.Close()
	client, err := net.ListenUDP("udp4", loopback)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	const location = "http://127.0.0.1:1400/xml/device_description.xml"
	var searches atomic.Int32
	go func() {
		buf := make([]byte, 2048)
		for {
			n, peer, err := speaker.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if !strings.HasPrefix(string(buf[:n]), "M-SEARCH * HTTP/1.1\r\n") {
				continue
			}
			// Drop the first search, as a lossy network might, then answer
			// every later one so the reply arrives more than once.
			if searches.Add(1) == 1 {
				continue
			}
			_, _ = speaker.WriteToUDP([]byte("HTTP/1.1 200 OK\r\nLOCATION: "+location+"\r\n\r\n"), peer)
		}
	}()
	window := ssdpSearches*ssdpResendInterval + 250*time.Millisecond
	got := search(t.Context(), window, []*net.UDPConn{client}, speaker.LocalAddr().(*net.UDPAddr))
	if !slices.Equal(got, []string{location}) {
		t.Fatalf("locations = %v, want one %s", got, location)
	}
	if n := searches.Load(); n != ssdpSearches {
		t.Fatalf("M-SEARCH sent %d times, want %d", n, ssdpSearches)
	}
}
