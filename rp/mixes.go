package rp

import (
	"maps"
	"slices"
)

type Mix struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Mixes lists the automatic choice first, followed by supported channel IDs.
func Mixes() []Mix {
	mixes := []Mix{{ID: -1, Name: "Automatic"}}
	for _, id := range slices.Sorted(maps.Keys(Channels)) {
		mixes = append(mixes, Mix{ID: id, Name: Channels[id]})
	}
	return mixes
}
