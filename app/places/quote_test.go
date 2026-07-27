package places_test

import (
	"testing"

	"github.com/RinTanth/go-backend/app/places"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int       { return &v }
func f64Ptr(v float64) *float64 { return &v }
func sPtr(v string) *string   { return &v }

func TestCalculateQuote_FreeWithinAllowance(t *testing.T) {
	r := require.New(t)
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(30),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{{
			TierOrder: 1, FromHour: 0, ToHour: f64Ptr(24), Price: 40, Unit: "hourly",
		}},
	}

	quote := places.CalculateQuote("p1", 0.5, rate)
	r.Equal(int64(0), quote.TotalSatang)
	r.Equal(0.0, quote.ChargeableHours)
	r.Equal(30, quote.FreeMinutesApplied)
}

func TestCalculateQuote_FullyFree(t *testing.T) {
	r := require.New(t)
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(-1),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{{
			TierOrder: 1, FromHour: 0, ToHour: f64Ptr(24), Price: 40, Unit: "hourly",
		}},
	}

	quote := places.CalculateQuote("p1", 5, rate)
	r.Equal(int64(0), quote.TotalSatang)
	r.Equal(-1, quote.FreeMinutesApplied)
}

func TestCalculateQuote_HourlyTiers(t *testing.T) {
	r := require.New(t)
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(0),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{
			{TierOrder: 1, FromHour: 0, ToHour: f64Ptr(2), Price: 20, Unit: "hourly"},
			{TierOrder: 2, FromHour: 2, ToHour: f64Ptr(11), Price: 20, Unit: "hourly"},
			{TierOrder: 3, FromHour: 12, ToHour: f64Ptr(24), Price: 200, Unit: "flat"},
		},
	}

	// 3 hours → 2h@20 + 1h@20 = 60
	quote := places.CalculateQuote("p1", 3, rate)
	r.Equal(3.0, quote.ChargeableHours)
	r.Equal(int64(6000), quote.TotalSatang)
	r.False(quote.DailyMaxApplied)
}

func TestCalculateQuote_RoundsUpPartialHour(t *testing.T) {
	r := require.New(t)
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(15),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{
			{TierOrder: 1, FromHour: 0, ToHour: f64Ptr(24), Price: 40, Unit: "hourly"},
		},
	}

	// 1 hour stay, 15 min free → 45 min billable → ceil to 1 hour → 40
	quote := places.CalculateQuote("p1", 1, rate)
	r.Equal(1.0, quote.ChargeableHours)
	r.Equal(int64(4000), quote.TotalSatang)
}

func TestCalculateQuote_DailyMaxCap(t *testing.T) {
	r := require.New(t)
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(0),
		DailyMax:    f64Ptr(100),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{
			{TierOrder: 1, FromHour: 0, ToHour: nil, Price: 50, Unit: "hourly"},
		},
	}

	quote := places.CalculateQuote("p1", 5, rate)
	r.Equal(int64(25000), quote.SubtotalSatang)
	r.Equal(int64(10000), quote.TotalSatang)
	r.True(quote.DailyMaxApplied)
}

func TestCalculateQuote_FlatTierOnce(t *testing.T) {
	r := require.New(t)
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(0),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{
			{TierOrder: 1, FromHour: 0, ToHour: f64Ptr(2), Price: 10, Unit: "flat"},
			{TierOrder: 2, FromHour: 2, ToHour: f64Ptr(24), Price: 20, Unit: "hourly"},
		},
	}

	// 4 hours → flat 10 once for hours 0-1, then hourly 20 for hours 2 and 3 → 10+20+20=50
	quote := places.CalculateQuote("p1", 4, rate)
	r.Equal(int64(5000), quote.TotalSatang)
}
