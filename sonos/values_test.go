package sonos

import (
	"testing"
)

func TestValuesKeepsFirstElementText(t *testing.T) {
	for _, tt := range []struct {
		name, xml, key, want string
	}{
		{"repeated element", `<DIDL-Lite><item><upnp:albumArtURI>/a.jpg</upnp:albumArtURI><upnp:albumArtURI>/b.jpg</upnp:albumArtURI></item></DIDL-Lite>`, "albumArtURI", "/a.jpg"},
		{"empty first element", `<item><albumArtURI/><albumArtURI>/b.jpg</albumArtURI></item>`, "albumArtURI", "/b.jpg"},
		{"text split across tokens", `<item><title>Rock &amp; <![CDATA[Roll]]> Radio</title></item>`, "title", "Rock & Roll Radio"},
		{"nested elements keep their own text", `<item><title>Song</title><creator>Artist</creator></item>`, "creator", "Artist"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Values([]byte(tt.xml))
			if err != nil || got[tt.key] != tt.want {
				t.Fatalf("Values()[%q] = %q, %v; want %q", tt.key, got[tt.key], err, tt.want)
			}
		})
	}
}
