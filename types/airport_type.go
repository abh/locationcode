package types

import (
	"sort"
	"strings"

	alphafoxtrot "github.com/grumpypixel/go-airport-finder"
)

type Airport struct {
	Name     string
	Code     string
	Distance float64
	Type     string
}

func NewAirport(airport *alphafoxtrot.Airport) *Airport {
	code := strings.ToLower(airport.Country.ISOCode + airport.IATACode)

	a := &Airport{
		Name: airport.Name,
		Code: code,
		Type: airport.Type,
	}

	return a
}

func (a *Airport) String() string {
	return a.Name
}

func UniqAirports(r []*Airport) []*Airport {
	seen := make(map[string]struct{})
	j := 0
	for _, v := range r {
		if _, ok := seen[v.Code]; ok {
			continue
		}
		seen[v.Code] = struct{}{}
		r[j] = v
		j++
	}
	return r[:j]
}

func SortAirports(r []*Airport) {
	sort.Slice(r, func(i, j int) bool {
		if r[i].Type == r[j].Type {
			return r[i].Distance < r[j].Distance
		}
		return r[i].Type < r[j].Type
	})
}
