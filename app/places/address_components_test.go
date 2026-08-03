package places

import "testing"

func TestParseAddressComponentsBangkokStyle(t *testing.T) {
	got := parseAddressComponents([]googleAddressComponent{
		{LongText: "999", Types: []string{"street_number"}},
		{LongText: "Rama I Road", Types: []string{"route"}},
		{LongText: "Pathum Wan", Types: []string{"sublocality_level_1", "sublocality", "political"}},
		{LongText: "Bangkok", Types: []string{"administrative_area_level_1", "political"}},
		{LongText: "10330", Types: []string{"postal_code"}},
		{LongText: "Thailand", Types: []string{"country", "political"}},
	})

	if got.AddressLine != "999 Rama I Road" {
		t.Fatalf("AddressLine = %q", got.AddressLine)
	}
	if got.District != "Pathum Wan" {
		t.Fatalf("District = %q", got.District)
	}
	if got.Province != "Bangkok" {
		t.Fatalf("Province = %q", got.Province)
	}
	if got.PostalCode != "10330" {
		t.Fatalf("PostalCode = %q", got.PostalCode)
	}
}

func TestParseAddressComponentsWithAdmin2(t *testing.T) {
	got := parseAddressComponents([]googleAddressComponent{
		{LongText: "หมู่บ้านตัวอย่าง", Types: []string{"premise"}},
		{LongText: "บางรัก", Types: []string{"sublocality_level_1", "sublocality", "political"}},
		{LongText: "เมืองเชียงใหม่", Types: []string{"administrative_area_level_2", "political"}},
		{LongText: "เชียงใหม่", Types: []string{"administrative_area_level_1", "political"}},
		{LongText: "50000", Types: []string{"postal_code"}},
	})

	if got.AddressLine != "หมู่บ้านตัวอย่าง" {
		t.Fatalf("AddressLine = %q", got.AddressLine)
	}
	if got.District != "เมืองเชียงใหม่" {
		t.Fatalf("District = %q", got.District)
	}
	if got.Subdistrict != "บางรัก" {
		t.Fatalf("Subdistrict = %q", got.Subdistrict)
	}
	if got.Province != "เชียงใหม่" {
		t.Fatalf("Province = %q", got.Province)
	}
}
