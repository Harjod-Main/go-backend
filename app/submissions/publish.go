package submissions

import (
	"encoding/json"
	"strconv"
	"strings"
)

var dayKeyToEnum = map[string]string{
	"sun": "SUN",
	"mon": "MON",
	"tue": "TUE",
	"wed": "WED",
	"thu": "THU",
	"fri": "FRI",
	"sat": "SAT",
}

var dayEnumOrder = []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}

type publishedHour struct {
	DayOfWeek string
	OpenTime  *string
	CloseTime *string
	IsClosed  bool
}

type publishedRateTier struct {
	TierOrder int
	Price     float64
	Unit      string
	FromHour  float64
	ToHour    *float64
}

type rateTierDraft struct {
	Unit         string   `json:"unit"`
	FromHour     float64  `json:"fromHour"`
	ToHour       *float64 `json:"toHour"`
	PricePerHour float64  `json:"pricePerHour"`
}

func mapPlaceType(placeType *string) string {
	if placeType == nil {
		return "standalone_parking"
	}
	switch strings.ToLower(strings.TrimSpace(*placeType)) {
	case "parking":
		return "standalone_parking"
	case "mall":
		return "shopping_mall"
	case "government":
		return "government"
	default:
		return "other"
	}
}

func derefOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func parseMoney(value *string) *float64 {
	if value == nil {
		return nil
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return nil
	}
	cleaned := strings.Builder{}
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '.' {
			cleaned.WriteRune(r)
		}
	}
	parsed, err := strconv.ParseFloat(cleaned.String(), 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseOpeningHours(raw json.RawMessage) []publishedHour {
	out := make([]publishedHour, 0, len(dayEnumOrder))
	byDay := map[string]publishedHour{}

	if len(raw) > 0 && string(raw) != "null" {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(raw, &payload); err == nil {
			for key, dayRaw := range payload {
				dayEnum, ok := dayKeyToEnum[strings.ToLower(strings.TrimSpace(key))]
				if !ok {
					continue
				}
				trimmed := strings.TrimSpace(string(dayRaw))
				if trimmed == `"open_24_hours"` {
					open := "00:00:00"
					close := "23:59:59"
					byDay[dayEnum] = publishedHour{
						DayOfWeek: dayEnum,
						OpenTime:  &open,
						CloseTime: &close,
						IsClosed:  false,
					}
					continue
				}
				var window struct {
					Open  string `json:"open"`
					Close string `json:"close"`
				}
				if err := json.Unmarshal(dayRaw, &window); err != nil {
					continue
				}
				open := normalizeClock(window.Open)
				close := normalizeClock(window.Close)
				if open == "" || close == "" {
					continue
				}
				byDay[dayEnum] = publishedHour{
					DayOfWeek: dayEnum,
					OpenTime:  &open,
					CloseTime: &close,
					IsClosed:  false,
				}
			}
		}
	}

	for _, day := range dayEnumOrder {
		if hour, ok := byDay[day]; ok {
			out = append(out, hour)
			continue
		}
		out = append(out, publishedHour{
			DayOfWeek: day,
			IsClosed:  true,
		})
	}
	return out
}

func normalizeClock(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) < 2 {
		return ""
	}
	hour := parts[0]
	minute := parts[1]
	if len(hour) == 1 {
		hour = "0" + hour
	}
	if len(minute) == 1 {
		minute = "0" + minute
	}
	if len(minute) > 2 {
		minute = minute[:2]
	}
	return hour + ":" + minute + ":00"
}

func parseRateTiers(raw json.RawMessage) []publishedRateTier {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var drafts []rateTierDraft
	if err := json.Unmarshal(raw, &drafts); err != nil {
		return nil
	}
	out := make([]publishedRateTier, 0, len(drafts))
	for i, draft := range drafts {
		unit := strings.ToLower(strings.TrimSpace(draft.Unit))
		if unit != "flat" && unit != "hourly" {
			unit = "hourly"
		}
		if draft.PricePerHour < 0 {
			continue
		}
		out = append(out, publishedRateTier{
			TierOrder: i + 1,
			Price:     draft.PricePerHour,
			Unit:      unit,
			FromHour:  draft.FromHour,
			ToHour:    draft.ToHour,
		})
	}
	return out
}

func amenitiesFlags(amenities []string) (hasEV, hasTransit, hasCover bool, transitType *string) {
	for _, item := range amenities {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "ev_charging":
			hasEV = true
		case "bts", "mrt", "transit":
			hasTransit = true
			if transitType == nil {
				label := strings.ToUpper(strings.TrimSpace(item))
				if label == "TRANSIT" {
					label = "BTS"
				}
				transitType = &label
			}
		case "covered", "cover":
			hasCover = true
		}
	}
	return hasEV, hasTransit, hasCover, transitType
}
