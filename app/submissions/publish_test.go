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

func TestParseStampEntries(t *testing.T) {
	raw := []byte(`[
		{"id":"1","value":{"category":"spending","spendingUpTo":"500","freeHour":"2","stampLocation":"G Floor","signagePhotos":["https://x/a.jpg"]}},
		{"id":"2","value":{"category":"bank_card","bankName":"scb","cardName":"visa","stampLocation":"B1"}},
		{"id":"3","value":{"category":"unknown"}}
	]`)
	got := parseStampEntries(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 stamps, got %d", len(got))
	}
	if got[0].ValidationType != "spending" || got[0].ConditionDescription != "500" {
		t.Fatalf("unexpected spending stamp: %+v", got[0])
	}
	if got[0].FreeMinutes == nil || *got[0].FreeMinutes != 120 {
		t.Fatalf("unexpected free minutes: %+v", got[0].FreeMinutes)
	}
	if got[1].ValidationType != "credential" || got[1].ProgramName == nil || *got[1].ProgramName != "Visa" {
		t.Fatalf("unexpected bank stamp: %+v", got[1])
	}
}

func TestParseReservedEntries(t *testing.T) {
	raw := []byte(`[
		{"id":"1","value":{"category":"credit_card","creditCardName":"SCB M Visa","rule":"2 hrs","location":"1F"}},
		{"id":"2","value":{"category":"company","companyName":"Acme"}},
		{"id":"3","value":{"category":"credit_card","creditCardName":""}}
	]`)
	got := parseReservedEntries(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 reserved, got %d", len(got))
	}
	if got[0].ReservationType != "cardholder" || got[0].ProgramOther == nil || *got[0].ProgramOther != "SCB M Visa" {
		t.Fatalf("unexpected reserved: %+v", got[0])
	}
	if got[1].ReservationType != "tenant" {
		t.Fatalf("unexpected company reserved: %+v", got[1])
	}
}

func TestParseEVEntries(t *testing.T) {
	raw := []byte(`[
		{"id":"1","value":{
			"providerName":"ea_anywhere",
			"location":"B2",
			"connectors":[{"id":"c1","connectorType":"TYPE_2","total":"2"},{"id":"c2","connectorType":"CCS2","total":"1"}]
		}},
		{"id":"2","value":{"providerName":"tesla","connectors":[]}}
	]`)
	got := parseEVEntries(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 EV, got %d", len(got))
	}
	if got[0].ProviderName != "EA Anywhere" {
		t.Fatalf("unexpected provider: %s", got[0].ProviderName)
	}
	if len(got[0].Connectors) != 3 {
		t.Fatalf("expected 3 connectors, got %d", len(got[0].Connectors))
	}
	if got[0].Connectors[0].ConnectorType != "AC_Type_2" || got[0].Connectors[2].ConnectorType != "CCS2" {
		t.Fatalf("unexpected connectors: %+v", got[0].Connectors)
	}
}

func TestWrapPrivilegeEntry(t *testing.T) {
	raw := []byte(`{"category":"spending","spendingUpTo":"300"}`)
	wrapped, err := wrapPrivilegeEntry(raw)
	if err != nil {
		t.Fatal(err)
	}
	stamps := parseStampEntries(wrapped)
	if len(stamps) != 1 || stamps[0].ValidationType != "spending" {
		t.Fatalf("unexpected: %+v", stamps)
	}
}
