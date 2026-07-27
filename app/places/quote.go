package places

import (
	"math"
	"sort"
)

// QuoteBreakdown describes one priced segment in a quote.
type QuoteBreakdown struct {
	FromHour     float64 `json:"fromHour"`
	ToHour       float64 `json:"toHour"`
	Unit         string  `json:"unit"`
	PriceSatang  int64   `json:"priceSatang"`
	AmountSatang int64   `json:"amountSatang"`
}

// Quote is the calculated parking price for a stay duration.
type Quote struct {
	PlaceID            string           `json:"placeId"`
	Hours              float64          `json:"hours"`
	Currency           string           `json:"currency"`
	FreeMinutesApplied int              `json:"freeMinutesApplied"`
	ChargeableHours    float64          `json:"chargeableHours"`
	SubtotalSatang     int64            `json:"subtotalSatang"`
	DailyMaxApplied    bool             `json:"dailyMaxApplied"`
	TotalSatang        int64            `json:"totalSatang"`
	Breakdown          []QuoteBreakdown `json:"breakdown"`
}

// CalculateQuote prices a stay of `hours` using a place rate sheet.
//
// Rules:
//  1. Subtract free_minutes from the stay, then round remaining time up to whole hours.
//  2. Walk each charged hour through sorted tiers (from_hour ≤ h < to_hour, or open-ended).
//  3. Hourly tiers add price once per charged hour in range.
//  4. Flat tiers add price once when the stay first enters that tier range.
//  5. Cap at daily_max when set.
func CalculateQuote(placeID string, hours float64, rate *PlaceRateDetail) Quote {
	currency := "THB"
	if rate != nil && rate.Currency != nil && *rate.Currency != "" {
		currency = *rate.Currency
	}

	quote := Quote{
		PlaceID:   placeID,
		Hours:     hours,
		Currency:  currency,
		Breakdown: []QuoteBreakdown{},
	}

	if rate == nil || hours <= 0 || math.IsNaN(hours) || math.IsInf(hours, 0) {
		return quote
	}

	freeMinutes := 0
	if rate.FreeMinutes != nil {
		if *rate.FreeMinutes == -1 {
			// Fully free parking.
			quote.FreeMinutesApplied = -1
			return quote
		}
		if *rate.FreeMinutes > 0 {
			freeMinutes = *rate.FreeMinutes
		}
	}
	quote.FreeMinutesApplied = freeMinutes

	chargeableMinutes := hours*60 - float64(freeMinutes)
	if chargeableMinutes <= 0 {
		return quote
	}
	chargeableHours := math.Ceil(chargeableMinutes / 60)
	quote.ChargeableHours = chargeableHours

	tiers := append([]PlaceRateTier(nil), rate.RateTier...)
	sort.SliceStable(tiers, func(i, j int) bool {
		if tiers[i].FromHour == tiers[j].FromHour {
			return tiers[i].TierOrder < tiers[j].TierOrder
		}
		return tiers[i].FromHour < tiers[j].FromHour
	})

	flatApplied := map[int]bool{}
	var subtotalSatang int64

	for h := 0; h < int(chargeableHours); h++ {
		hourStart := float64(h)
		hourEnd := float64(h + 1)
		tier := findTierForHour(tiers, hourStart)
		if tier == nil {
			continue
		}

		switch tier.Unit {
		case "flat":
			if flatApplied[tier.TierOrder] {
				continue
			}
			flatApplied[tier.TierOrder] = true
			priceSatang := satangFromFloat(tier.Price)
			subtotalSatang += priceSatang
			quote.Breakdown = append(quote.Breakdown, QuoteBreakdown{
				FromHour:     tier.FromHour,
				ToHour:       tierEndOr(tier, hourEnd),
				Unit:         "flat",
				PriceSatang:  priceSatang,
				AmountSatang: priceSatang,
			})
		default: // hourly
			priceSatang := satangFromFloat(tier.Price)
			subtotalSatang += priceSatang
			quote.Breakdown = append(quote.Breakdown, QuoteBreakdown{
				FromHour:     hourStart,
				ToHour:       hourEnd,
				Unit:         "hourly",
				PriceSatang:  priceSatang,
				AmountSatang: priceSatang,
			})
		}
	}

	quote.SubtotalSatang = subtotalSatang
	quote.TotalSatang = subtotalSatang
	if rate.DailyMax != nil && *rate.DailyMax > 0 && subtotalSatang > satangFromFloat(*rate.DailyMax) {
		quote.TotalSatang = satangFromFloat(*rate.DailyMax)
		quote.DailyMaxApplied = true
	}

	return quote
}

func satangFromFloat(v float64) int64 {
	return int64(math.Round(v * 100))
}

func findTierForHour(tiers []PlaceRateTier, hour float64) *PlaceRateTier {
	var matched *PlaceRateTier
	for i := range tiers {
		tier := &tiers[i]
		if hour < tier.FromHour {
			continue
		}
		if tier.ToHour != nil && hour >= *tier.ToHour {
			continue
		}
		matched = tier
	}
	return matched
}

func tierEndOr(tier *PlaceRateTier, fallback float64) float64 {
	if tier.ToHour != nil {
		return *tier.ToHour
	}
	return fallback
}
