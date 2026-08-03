package places

import "strings"

// StructuredAddress is a places-table-aligned address breakdown.
type StructuredAddress struct {
	AddressLine string `json:"addressLine,omitempty"`
	Subdistrict string `json:"subdistrict,omitempty"`
	District    string `json:"district,omitempty"`
	Province    string `json:"province,omitempty"`
	PostalCode  string `json:"postalCode,omitempty"`
}

type googleAddressComponent struct {
	LongText  string   `json:"longText"`
	ShortText string   `json:"shortText"`
	Types     []string `json:"types"`
}

func parseAddressComponents(components []googleAddressComponent) StructuredAddress {
	get := func(want ...string) string {
		for _, typ := range want {
			for _, c := range components {
				for _, t := range c.Types {
					if t == typ {
						if v := strings.TrimSpace(c.LongText); v != "" {
							return v
						}
					}
				}
			}
		}
		return ""
	}

	streetNumber := get("street_number")
	route := get("route")
	premise := get("premise")

	parts := make([]string, 0, 2)
	if streetNumber != "" {
		parts = append(parts, streetNumber)
	}
	if route != "" {
		parts = append(parts, route)
	}
	addressLine := strings.Join(parts, " ")
	if addressLine == "" {
		addressLine = premise
	}

	admin2 := get("administrative_area_level_2")
	admin3 := get("administrative_area_level_3")
	sub1 := get("sublocality_level_1")
	sub2 := get("sublocality_level_2")
	neighborhood := get("neighborhood")
	locality := get("locality")

	var subdistrict, district string
	if admin2 != "" {
		district = admin2
		subdistrict = firstNonEmpty(admin3, sub2, sub1, neighborhood)
	} else {
		// Bangkok-style responses often omit admin_area_level_2 and put
		// the district (เขต) in sublocality_level_1.
		district = firstNonEmpty(sub1, locality)
		subdistrict = firstNonEmpty(sub2, admin3, neighborhood)
	}

	return StructuredAddress{
		AddressLine: addressLine,
		Subdistrict: subdistrict,
		District:    district,
		Province:    get("administrative_area_level_1"),
		PostalCode:  get("postal_code"),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
