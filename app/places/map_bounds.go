package places

import (
	"fmt"
	"strconv"
)

const maxMapBoundsSpanDeg = 2.0

// MapBounds is a WGS84 viewport. West/south/east/north must all be set together.
type MapBounds struct {
	West  float64
	South float64
	East  float64
	North float64
}

func parseMapBounds(west, south, east, north string) (*MapBounds, error) {
	if west == "" && south == "" && east == "" && north == "" {
		return nil, nil
	}
	if west == "" || south == "" || east == "" || north == "" {
		return nil, fmt.Errorf("west, south, east, and north are required together")
	}

	w, errW := strconv.ParseFloat(west, 64)
	s, errS := strconv.ParseFloat(south, 64)
	e, errE := strconv.ParseFloat(east, 64)
	n, errN := strconv.ParseFloat(north, 64)
	if errW != nil || errS != nil || errE != nil || errN != nil {
		return nil, fmt.Errorf("invalid map bounds")
	}
	if s < -90 || n > 90 || w < -180 || e > 180 || s >= n || w >= e {
		return nil, fmt.Errorf("invalid map bounds")
	}
	if n-s > maxMapBoundsSpanDeg || e-w > maxMapBoundsSpanDeg {
		return nil, fmt.Errorf("map bounds too large")
	}
	return &MapBounds{West: w, South: s, East: e, North: n}, nil
}
