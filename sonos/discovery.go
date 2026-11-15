package sonos

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SSDP runs over UDP, so a single M-SEARCH (or its replies) can be lost,
// especially on Wi-Fi. Send it a few times, as UPnP recommends.
const (
	ssdpSearches       = 3
	ssdpResendInterval = 250 * time.Millisecond
)

var (
	ssdpGroup   = &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	ssdpMSearch = []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n\r\n")
)

// Discover sends SSDP from each active IPv4 interface so Wi-Fi and Ethernet work.
func Discover(ctx context.Context, window time.Duration) []string {
	interfaces, _ := net.Interfaces()
	var connections []*net.UDPConn
	defer func() {
		for _, c := range connections {
			_ = c.Close()
		}
	}()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, _ := net.ParseCIDR(addr.String())
			if ip.To4() == nil {
				continue
			}
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
			if err != nil {
				continue
			}
			connections = append(connections, conn)
		}
	}
	return search(ctx, window, connections, ssdpGroup)
}

// search sends M-SEARCH from each connection to group, repeating it while the
// window is open, and collects unique speaker locations. Readers stop at the
// window's deadline or when the caller closes the connections.
func search(ctx context.Context, window time.Duration, connections []*net.UDPConn, group *net.UDPAddr) []string {
	found := make(chan string, 128)
	for _, conn := range connections {
		_ = conn.SetDeadline(time.Now().Add(window))
		go func() {
			buf := make([]byte, 8192)
			for {
				n, peer, err := conn.ReadFromUDP(buf)
				if err != nil {
					return
				}
				if loc := ssdpLocation(string(buf[:n]), peer.IP); loc != "" {
					select {
					case found <- loc:
					default:
					}
				}
			}
		}()
	}
	send := func() {
		for _, conn := range connections {
			_, _ = conn.WriteToUDP(ssdpMSearch, group)
		}
	}
	send()
	sent := 1
	resend := time.NewTicker(ssdpResendInterval)
	defer resend.Stop()
	timer := time.NewTimer(window)
	defer timer.Stop()
	seen := map[string]bool{}
	out := []string{}
	for {
		select {
		case loc := <-found:
			if !seen[loc] {
				seen[loc] = true
				out = append(out, loc)
			}
		case <-resend.C:
			if sent < ssdpSearches {
				send()
				sent++
			}
		case <-timer.C:
			return out
		case <-ctx.Done():
			return out
		}
	}
}
func ssdpLocation(message string, peer net.IP) string {
	resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(message)), nil)
	if err != nil {
		return ""
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	u, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || u.Scheme != "http" || !net.ParseIP(u.Hostname()).Equal(peer) {
		return ""
	}
	return u.String()
}
func HostLocation(host string) (string, error) {
	ip := net.ParseIP(host)
	if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()) {
		return "", fmt.Errorf("speaker host must be a local IP address")
	}
	return "http://" + net.JoinHostPort(ip.String(), "1400") + "/xml/device_description.xml", nil
}
