package services

import (
	"math/rand/v2"
	"sync"
)

type roomGenerator struct {
	mu    sync.Mutex
	inUse map[string]struct{}
}

var colors = []string{
	"amber", "aqua", "azure", "beige", "black", "blue",
	"bronze", "cherry", "coral", "crimson", "cyan",
	"emerald", "fuchsia", "golden", "gray", "green",
	"indigo", "ivory", "jade", "lavender", "lime",
	"magenta", "maroon", "navy", "olive", "orange",
	"pearl", "pink", "plum", "purple", "red",
	"rose", "ruby", "saffron", "salmon", "scarlet",
	"silver", "tan", "teal", "turquoise", "violet",
	"white", "yellow",
}

var cities = []string{
	// Europe
	"amsterdam", "athens", "barcelona", "berlin", "brussels", "budapest",
	"copenhagen", "dublin", "edinburgh", "florence", "geneva", "helsinki",
	"istanbul", "lisbon", "london", "madrid", "milan", "munich", "kyiv",
	"oslo", "paris", "porto", "prague", "rome", "stockholm",
	"vienna", "venice", "warsaw", "zagreb", "zurich",

	// Worldwide
	"abu_dhabi", "bangkok", "beijing", "buenos_aires", "cape_town",
	"chicago", "dubai", "hanoi", "hong_kong", "jakarta",
	"jerusalem", "kathmandu", "kuala_lumpur", "lagos", "los_angeles",
	"melbourne", "mexico_city", "miami", "montreal", "mumbai",
	"nairobi", "new_york", "rio", "santiago", "seoul",
	"shanghai", "singapore", "sydney", "tokyo", "toronto", "vancouver",
}

// NewRoomGenerator creates a new unique room name generator.
func NewRoomGenerator() *roomGenerator {
	return &roomGenerator{
		inUse: make(map[string]struct{}),
	}
}

// Generate returns a unique room name like "paris-crimson"
func (g *roomGenerator) Generate() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	for {
		city := cities[rand.IntN(len(cities))]
		color := colors[rand.IntN(len(colors))]
		room := city + "-" + color

		if _, ok := g.inUse[room]; !ok {
			g.inUse[room] = struct{}{}
			return room
		}
	}
}

// Release frees a room name for reuse.
func (g *roomGenerator) Release(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inUse, name)
}
