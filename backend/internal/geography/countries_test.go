package geography

import "testing"

func TestNaturalEarthCountryLookup(t *testing.T) {
	index, err := LoadNaturalEarthIndex()
	if err != nil {
		t.Fatalf("LoadNaturalEarthIndex = %v", err)
	}

	if code, found := index.CountryAt(48.8566, 2.3522); !found || code != "FRA" {
		t.Fatalf("Paris country = %q, %t; want FRA, true", code, found)
	}
	if code, found := index.CountryAt(0, -140); found {
		t.Fatalf("open-water country = %q, want no country", code)
	}
}
