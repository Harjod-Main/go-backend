package submissions

import "testing"

func TestMapPlaceType(t *testing.T) {
	parking := "parking"
	mall := "mall"
	if got := mapPlaceType(&parking); got != "standalone_parking" {
		t.Fatalf("parking => %s", got)
	}
	if got := mapPlaceType(&mall); got != "shopping_mall" {
		t.Fatalf("mall => %s", got)
	}
	if got := mapPlaceType(nil); got != "standalone_parking" {
		t.Fatalf("nil => %s", got)
	}
}

func TestParseOpeningHoursOpen24(t *testing.T) {
	hours := parseOpeningHours([]byte(`{"mon":"open_24_hours","tue":{"open":"08:00","close":"22:00"}}`))
	if len(hours) != 7 {
		t.Fatalf("expected 7 days, got %d", len(hours))
	}
	var mon, tue, wed *publishedHour
	for i := range hours {
		switch hours[i].DayOfWeek {
		case "MON":
			mon = &hours[i]
		case "TUE":
			tue = &hours[i]
		case "WED":
			wed = &hours[i]
		}
	}
	if mon == nil || mon.IsClosed || mon.OpenTime == nil || *mon.OpenTime != "00:00:00" {
		t.Fatalf("unexpected mon: %+v", mon)
	}
	if tue == nil || tue.IsClosed || tue.OpenTime == nil || *tue.OpenTime != "08:00:00" {
		t.Fatalf("unexpected tue: %+v", tue)
	}
	if wed == nil || !wed.IsClosed {
		t.Fatalf("unexpected wed: %+v", wed)
	}
}

func TestParseRateTiers(t *testing.T) {
	tiers := parseRateTiers([]byte(`[{"unit":"flat","toHour":24,"fromHour":0,"pricePerHour":40}]`))
	if len(tiers) != 1 {
		t.Fatalf("expected 1 tier, got %d", len(tiers))
	}
	if tiers[0].Unit != "flat" || tiers[0].Price != 40 || tiers[0].FromHour != 0 {
		t.Fatalf("unexpected tier: %+v", tiers[0])
	}
	if tiers[0].ToHour == nil || *tiers[0].ToHour != 24 {
		t.Fatalf("unexpected toHour: %+v", tiers[0].ToHour)
	}
}

func TestParseMoney(t *testing.T) {
	raw := "฿1,200"
	got := parseMoney(&raw)
	if got == nil || *got != 1200 {
		t.Fatalf("got %+v", got)
	}
}
