// Package geography provides deterministic offline country lookup for scored
// challenge locations.
package geography

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

//go:embed natural-earth-admin0-5.1.1.json.gz.b64
var boundaryData embed.FS

const maxBoundaryDataBytes = 32 << 20

type point struct{ lon, lat float64 }
type ring []point

type polygon struct {
	rings          []ring
	minLon, minLat float64
	maxLon, maxLat float64
}

type country struct {
	code     string
	polygons []polygon
}

// Index is an immutable in-memory copy of the bundled Natural Earth Admin 0
// country boundaries.
type Index struct{ countries []country }

type feature struct {
	ID       string `json:"id"`
	Geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

// LoadNaturalEarthIndex validates and loads the bundled Natural Earth 5.1.1
// Admin 0 boundary data. The asset is offline and has no runtime network path.
func LoadNaturalEarthIndex() (*Index, error) {
	encoded, err := boundaryData.ReadFile("natural-earth-admin0-5.1.1.json.gz.b64")
	if err != nil {
		return nil, fmt.Errorf("read country boundaries: %w", err)
	}
	compressed := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	decodedBytes, err := base64.StdEncoding.Decode(compressed, bytes.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode country boundary asset: %w", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed[:decodedBytes]))
	if err != nil {
		return nil, fmt.Errorf("open country boundaries: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxBoundaryDataBytes+1))
	closeErr := reader.Close()
	if err != nil {
		return nil, fmt.Errorf("read country boundary data: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close country boundary data: %w", closeErr)
	}
	if len(raw) > maxBoundaryDataBytes {
		return nil, fmt.Errorf("country boundary data exceeds %d bytes", maxBoundaryDataBytes)
	}
	var features []feature
	if err := json.Unmarshal(raw, &features); err != nil {
		return nil, fmt.Errorf("decode country boundaries: %w", err)
	}
	if len(features) < 200 {
		return nil, fmt.Errorf("country boundary data is incomplete: found %d features", len(features))
	}
	index := &Index{countries: make([]country, 0, len(features))}
	seen := make(map[string]struct{}, len(features))
	for _, item := range features {
		if item.ID == "" {
			return nil, errors.New("country boundary has no Admin 0 code")
		}
		if _, exists := seen[item.ID]; exists {
			return nil, fmt.Errorf("duplicate Admin 0 country code %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		polygons, err := decodePolygons(item.Geometry.Type, item.Geometry.Coordinates)
		if err != nil {
			return nil, fmt.Errorf("decode boundaries for %s: %w", item.ID, err)
		}
		index.countries = append(index.countries, country{code: item.ID, polygons: polygons})
	}
	sort.Slice(index.countries, func(i, j int) bool { return index.countries[i].code < index.countries[j].code })
	return index, nil
}

func decodePolygons(geometryType string, raw json.RawMessage) ([]polygon, error) {
	var coordinates [][][]float64
	switch geometryType {
	case "Polygon":
		if err := json.Unmarshal(raw, &coordinates); err != nil {
			return nil, err
		}
		return makePolygons([][][][]float64{coordinates})
	case "MultiPolygon":
		var multi [][][][]float64
		if err := json.Unmarshal(raw, &multi); err != nil {
			return nil, err
		}
		return makePolygons(multi)
	default:
		return nil, fmt.Errorf("unsupported geometry type %q", geometryType)
	}
}

func makePolygons(raw [][][][]float64) ([]polygon, error) {
	polygons := make([]polygon, 0, len(raw))
	for _, rawPolygon := range raw {
		if len(rawPolygon) == 0 {
			continue
		}
		current := polygon{minLon: math.Inf(1), minLat: math.Inf(1), maxLon: math.Inf(-1), maxLat: math.Inf(-1)}
		for _, rawRing := range rawPolygon {
			if len(rawRing) < 4 {
				return nil, errors.New("polygon ring has fewer than four points")
			}
			isOuterRing := len(current.rings) == 0
			points := make(ring, 0, len(rawRing))
			for _, coordinate := range rawRing {
				if len(coordinate) < 2 || math.IsNaN(coordinate[0]) || math.IsNaN(coordinate[1]) {
					return nil, errors.New("invalid polygon coordinate")
				}
				lon := coordinate[0]
				if len(points) > 0 {
					previousLon := points[len(points)-1].lon
					for lon-previousLon > 180 {
						lon -= 360
					}
					for lon-previousLon < -180 {
						lon += 360
					}
				}
				p := point{lon: lon, lat: coordinate[1]}
				points = append(points, p)
				if isOuterRing {
					current.minLon = math.Min(current.minLon, p.lon)
					current.maxLon = math.Max(current.maxLon, p.lon)
					current.minLat = math.Min(current.minLat, p.lat)
					current.maxLat = math.Max(current.maxLat, p.lat)
				}
			}
			current.rings = append(current.rings, points)
		}
		centerLon := (current.minLon + current.maxLon) / 2
		for ringIndex := 1; ringIndex < len(current.rings); ringIndex++ {
			hole := current.rings[ringIndex]
			var minLon, maxLon float64
			for pointIndex, p := range hole {
				if pointIndex == 0 || p.lon < minLon {
					minLon = p.lon
				}
				if pointIndex == 0 || p.lon > maxLon {
					maxLon = p.lon
				}
			}
			shift := math.Round((centerLon-(minLon+maxLon)/2)/360) * 360
			for pointIndex := range hole {
				hole[pointIndex].lon += shift
			}
		}
		polygons = append(polygons, current)
	}
	if len(polygons) == 0 {
		return nil, errors.New("country has no polygon geometry")
	}
	return polygons, nil
}

// CountryAt returns the Natural Earth Admin 0 code containing a coordinate.
// Water points have no result. If a point falls on a disputed shared border,
// sorted feature codes provide a stable, deterministic choice.
func (index *Index) CountryAt(lat, lon float64) (string, bool) {
	if index == nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 || math.IsNaN(lat) || math.IsNaN(lon) {
		return "", false
	}
	for _, candidate := range index.countries {
		for _, shape := range candidate.polygons {
			if lat < shape.minLat || lat > shape.maxLat {
				continue
			}
			shapeLon := longitudeNear(lon, (shape.minLon+shape.maxLon)/2)
			if shapeLon < shape.minLon || shapeLon > shape.maxLon {
				continue
			}
			if polygonContains(shape, point{lon: shapeLon, lat: lat}) {
				return candidate.code, true
			}
		}
	}
	return "", false
}

func polygonContains(shape polygon, p point) bool {
	centerLon := (shape.minLon + shape.maxLon) / 2
	outer := ringLocation(shape.rings[0], p, centerLon)
	if outer < 0 {
		return false
	}
	if outer == 0 {
		return true
	}
	for _, hole := range shape.rings[1:] {
		location := ringLocation(hole, p, centerLon)
		if location == 1 {
			return false
		}
		if location == 0 {
			return true
		}
	}
	return true
}

// ringLocation returns -1 outside, 0 on the boundary, and 1 inside.
func ringLocation(points ring, p point, centerLon float64) int {
	p.lon = longitudeNear(p.lon, centerLon)
	inside := false
	for i, current := range points {
		previous := points[(i+len(points)-1)%len(points)]
		if onSegment(p, previous, current) {
			return 0
		}
		if (current.lat > p.lat) != (previous.lat > p.lat) {
			crossingLon := (previous.lon-current.lon)*(p.lat-current.lat)/(previous.lat-current.lat) + current.lon
			if p.lon < crossingLon {
				inside = !inside
			}
		}
	}
	if inside {
		return 1
	}
	return -1
}

func longitudeNear(lon, reference float64) float64 {
	for lon-reference > 180 {
		lon -= 360
	}
	for lon-reference < -180 {
		lon += 360
	}
	return lon
}

func onSegment(p, a, b point) bool {
	cross := (p.lon-a.lon)*(b.lat-a.lat) - (p.lat-a.lat)*(b.lon-a.lon)
	if math.Abs(cross) > 1e-10 {
		return false
	}
	return p.lon >= math.Min(a.lon, b.lon)-1e-10 && p.lon <= math.Max(a.lon, b.lon)+1e-10 &&
		p.lat >= math.Min(a.lat, b.lat)-1e-10 && p.lat <= math.Max(a.lat, b.lat)+1e-10
}
