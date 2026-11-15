package sonos

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Discover sends SSDP from each active IPv4 interface so Wi-Fi and Ethernet work.
func Discover(ctx context.Context, window time.Duration) []string {
	interfaces, _ := net.Interfaces()
	found := make(chan string, 128)
	var connections []*net.UDPConn
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
			_ = conn.SetDeadline(time.Now().Add(window))
			_, _ = conn.WriteToUDP([]byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n\r\n"), &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900})
			go func(c *net.UDPConn) {
				buf := make([]byte, 8192)
				for {
					n, peer, err := c.ReadFromUDP(buf)
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
			}(conn)
		}
	}
	defer func() {
		for _, c := range connections {
			_ = c.Close()
		}
	}()
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

// ErrNotLocal reports a speaker connection to an address outside the LAN.
var ErrNotLocal = errors.New("speaker address must be a local IP address")

// localIP reports whether host is a private, loopback or link-local IP address.
func localIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

// localDialer refuses connections to non-local addresses. Control sees the
// resolved address, so hostnames are covered, before any packet is sent.
func localDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			if host, _, err := net.SplitHostPort(address); err != nil || !localIP(host) {
				return ErrNotLocal
			}
			return nil
		},
	}
}
func HostLocation(host string) (string, error) {
	if !localIP(host) {
		return "", fmt.Errorf("speaker host must be a local IP address")
	}
	return "http://" + net.JoinHostPort(net.ParseIP(host).String(), "1400") + "/xml/device_description.xml", nil
}
