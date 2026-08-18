package places

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
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
	NightRateApplied   bool             `json:"nightRateApplied"`
	TotalSatang        int64            `json:"totalSatang"`
	Breakdown          []QuoteBreakdown `json:"breakdown"`
}

// QuoteOptions are extras the map quote can apply on top of the rate sheet.
type QuoteOptions struct {
	// StampFreeMinutes is walk-in (no-spend) stamp free time. -1 = fully free.
	StampFreeMinutes int
	// Now is the assumed stay start (Asia/Bangkok). Zero skips overnight fees.
	Now time.Time
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
	return CalculateQuoteOpts(placeID, hours, rate, QuoteOptions{})
}

func CalculateQuoteOpts(placeID string, hours float64, rate *PlaceRateDetail, opts QuoteOptions) Quote {
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

	rateFree := 0
	if rate.FreeMinutes != nil {
		if *rate.FreeMinutes == -1 {
			quote.FreeMinutesApplied = -1
			return quote
		}
		if *rate.FreeMinutes > 0 {
			rateFree = *rate.FreeMinutes
		}
	}
	if opts.StampFreeMinutes == -1 {
		quote.FreeMinutesApplied = -1
		return quote
	}
	stampFree := 0
	if opts.StampFreeMinutes > 0 {
		stampFree = opts.StampFreeMinutes
	}
	freeMinutes := rateFree + stampFree
	quote.FreeMinutesApplied = freeMinutes

	chargeableMinutes := hours*60 - float64(freeMinutes)
	if chargeableMinutes > 0 {
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
	}

	if rate.NightRate != nil && *rate.NightRate > 0 && !opts.Now.IsZero() &&
		rate.NightStartTime != nil && rate.NightEndTime != nil {
		nights := overlappingNights(opts.Now, hours, *rate.NightStartTime, *rate.NightEndTime)
		if nights > 0 {
			nightSatang := satangFromFloat(*rate.NightRate) * int64(nights)
			quote.TotalSatang += nightSatang
			quote.NightRateApplied = true
			quote.Breakdown = append(quote.Breakdown, QuoteBreakdown{
				FromHour:     0,
				ToHour:       hours,
				Unit:         "night",
				PriceSatang:  satangFromFloat(*rate.NightRate),
				AmountSatang: nightSatang,
			})
		}
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

func overlappingNights(now time.Time, hours float64, startClock, endClock string) int {
	startH, startM, startS, okStart := parseClock(startClock)
	endH, endM, endS, okEnd := parseClock(endClock)
	if !okStart || !okEnd {
		return 0
	}
	loc := now.Location()
	stayStart := now
	stayEnd := now.Add(time.Duration(hours * float64(time.Hour)))
	if !stayEnd.After(stayStart) {
		return 0
	}

	day := time.Date(stayStart.Year(), stayStart.Month(), stayStart.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	last := time.Date(stayEnd.Year(), stayEnd.Month(), stayEnd.Day(), 0, 0, 0, 0, loc)
	count := 0
	for !day.After(last) {
		windowStart := time.Date(day.Year(), day.Month(), day.Day(), startH, startM, startS, 0, loc)
		windowEnd := time.Date(day.Year(), day.Month(), day.Day(), endH, endM, endS, 0, loc)
		if !windowEnd.After(windowStart) {
			windowEnd = windowEnd.Add(24 * time.Hour)
		}
		if stayStart.Before(windowEnd) && stayEnd.After(windowStart) {
			count++
		}
		day = day.AddDate(0, 0, 1)
	}
	return count
}

func parseClock(value string) (hour, minute, sec int, ok bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	hour, errH := strconv.Atoi(parts[0])
	minute, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, 0, false
	}
	if len(parts) == 3 {
		sec, errS := strconv.Atoi(parts[2])
		if errS != nil || sec < 0 || sec > 59 {
			return 0, 0, 0, false
		}
	}
	return hour, minute, sec, true
}
