package sonos

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSOAPTopologyPlaybackAndControls(t *testing.T) {
	var serverURL string
	volume := "17"
	mute := "0"
	state := "PLAYING"
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprintf(w, `<root><device><friendlyName>Living Room</friendlyName><UDN>uuid:RINCON_A</UDN><serviceList><service><serviceType>urn:schemas-upnp-org:service:ZoneGroupTopology:1</serviceType><controlURL>/topology</controlURL></service></serviceList><deviceList><device><serviceList><service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>/transport</controlURL></service><service><serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType><controlURL>/rendering</controlURL></service></serviceList></device></deviceList></device></root>`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		args, err := Values(body)
		if err != nil {
			t.Error(err)
		}
		action := strings.TrimSuffix(strings.Split(r.Header.Get("SOAPACTION"), "#")[1], `"`)
		payload := ""
		switch action {
		case "GetZoneGroupState":
			payload = "<ZoneGroupState>" + escape(`<ZoneGroupState><ZoneGroups><ZoneGroup Coordinator="RINCON_A"><ZoneGroupMember UUID="RINCON_A" ZoneName="Living Room" Location="`+serverURL+`/description"/><ZoneGroupMember UUID="RINCON_S" ZoneName="Satellite" Invisible="1" Location="`+serverURL+`/satellite"/></ZoneGroup></ZoneGroups></ZoneGroupState>`) + "</ZoneGroupState>"
		case "GetTransportInfo":
			payload = "<CurrentTransportState>" + state + "</CurrentTransportState>"
		case "GetPositionInfo":
			payload = "<TrackURI>http://stream.radioparadise.com/aac-320</TrackURI><RelTime>00:01:02</RelTime><TrackDuration>00:03:40</TrackDuration><TrackMetaData>" + escape(`<DIDL-Lite><item><title>Station</title><creator>Artist</creator><streamContent>Artist - Track</streamContent><album>Album</album><albumArtURI>/art.jpg</albumArtURI></item></DIDL-Lite>`) + "</TrackMetaData>"
		case "GetMediaInfo":
			payload = "<CurrentURI>x-sonosapi-radio:channel%3a0%3a4%3aresume?sid=308</CurrentURI><CurrentURIMetaData>" + escape(`<DIDL-Lite><item><title>Radio Paradise</title></item></DIDL-Lite>`) + "</CurrentURIMetaData>"
		case "GetCurrentTransportActions":
			payload = "<Actions>Pause,Stop,Next</Actions>"
		case "GetVolume":
			payload = "<CurrentVolume>" + volume + "</CurrentVolume>"
		case "GetMute":
			payload = "<CurrentMute>" + mute + "</CurrentMute>"
		case "SetVolume":
			if strings.Index(string(body), "<InstanceID>") > strings.Index(string(body), "<Channel>") || strings.Index(string(body), "<Channel>") > strings.Index(string(body), "<DesiredVolume>") {
				t.Error("UPnP arguments are out of declaration order")
			}
			volume = args["DesiredVolume"]
		case "SetMute":
			mute = args["DesiredMute"]
		case "Pause":
			state = "PAUSED_PLAYBACK"
		default:
			t.Errorf("unexpected action %s", action)
		}
		fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><Response>%s</Response></s:Body></s:Envelope>`, payload)
	}))
	c := New()
	c.HTTP.Transport = srv.Client().Transport
	serverURL = srv.URL
	ctx := context.Background()
	speaker, err := c.Describe(ctx, srv.URL+"/description")
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := c.Topology(ctx, speaker)
	if err != nil || len(rooms) != 1 || rooms[0].Coordinator != "RINCON_A" {
		t.Fatalf("topology: %+v %v", rooms, err)
	}
	p, err := c.Playback(ctx, rooms[0])
	if err != nil || p.Title != "Track" || p.Artist != "Artist" || p.Station != "Radio Paradise" || p.Position != 62 || p.Duration != 220 || p.Artwork != srv.URL+"/art.jpg" {
		t.Fatalf("playback: %+v %v", p, err)
	}
	if p.TrackURI != "http://stream.radioparadise.com/aac-320" || p.URI != "x-sonosapi-radio:channel%3a0%3a4%3aresume?sid=308" {
		t.Fatal("track audio URL was lost when reading overall source URI")
	}
	if err = c.SetVolume(ctx, speaker, 33); err != nil {
		t.Fatal(err)
	}
	if err = c.SetMute(ctx, speaker, true); err != nil {
		t.Fatal(err)
	}
	v, m, err := c.Volume(ctx, speaker)
	if err != nil || v != 33 || !m {
		t.Fatal("volume/mute failed")
	}
	if err = c.SetVolume(ctx, speaker, 101); err == nil {
		t.Fatal("invalid volume accepted")
	}
	if err = c.Transport(ctx, speaker, "Pause"); err != nil {
		t.Fatal(err)
	}
}
func TestSOAPFaultAndSSDPValidation(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`<Envelope><Body><Fault><errorCode>701</errorCode><errorDescription>Transition not available</errorDescription></Fault></Body></Envelope>`))
	}))
	c := New()
	c.HTTP.Transport = srv.Client().Transport
	_, err := c.Call(context.Background(), Speaker{URL: srv.URL, Services: map[string]Service{"AVTransport": {Type: "urn:AVTransport:1", Control: "/"}}}, "AVTransport", "Pause", nil)
	if err == nil || !strings.Contains(err.Error(), "701") {
		t.Fatalf("fault: %v", err)
	}
	message := "HTTP/1.1 200 OK\r\nLOCATION: http://192.168.1.3:1400/xml/device_description.xml\r\n\r\n"
	if ssdpLocation(message, net.ParseIP("192.168.1.3")) == "" {
		t.Fatal("valid SSDP reply rejected")
	}
	if ssdpLocation(message, net.ParseIP("192.168.1.4")) != "" {
		t.Fatal("SSDP reply redirected to another host")
	}
	if _, err = HostLocation("https://example.com"); err == nil {
		t.Fatal("URL accepted as host")
	}
	if _, err = HostLocation("8.8.8.8"); err == nil {
		t.Fatal("public IP accepted")
	}
}
