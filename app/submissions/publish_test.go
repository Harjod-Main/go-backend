package submissions

import (
	"testing"

	"github.com/RinTanth/go-backend/app/mediaurl"
)

const testMediaPrefix = "https://sycwdwymeirxowbrqdgd.supabase.co/storage/v1/object/public/media/"

func init() {
	mediaurl.Configure("https://sycwdwymeirxowbrqdgd.supabase.co")
}

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
	goodPhoto := testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/a.jpg"
	raw := []byte(`[
		{"id":"1","value":{"category":"spending","spendingUpTo":"500","freeHour":"2","stampLocation":"G Floor","signagePhotos":["` + goodPhoto + `"]}},
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
	if len(got[0].SignagePhotos) != 1 || got[0].SignagePhotos[0] != goodPhoto {
		t.Fatalf("unexpected signage: %+v", got[0].SignagePhotos)
	}
	if got[1].ValidationType != "credential" || got[1].ProgramName == nil || *got[1].ProgramName != "Visa" {
		t.Fatalf("unexpected bank stamp: %+v", got[1])
	}
}

func TestParseStampEntries_RejectsForeignSignageURL(t *testing.T) {
	raw := []byte(`[
		{"id":"1","value":{"category":"spending","spendingUpTo":"500","freeHour":"2","signagePhotos":["https://evil.example.com/a.jpg"]}},
		{"id":"2","value":{"category":"spending","spendingUpTo":"300","freeHour":"1"}}
	]`)
	got := parseStampEntries(raw)
	if len(got) != 1 {
		t.Fatalf("expected only stamp without evil URL, got %d", len(got))
	}
	if got[0].ConditionDescription != "300" {
		t.Fatalf("unexpected stamp kept: %+v", got[0])
	}
}

func TestParseReservedEntries_RejectsForeignSignageURL(t *testing.T) {
	raw := []byte(`[
		{"id":"1","value":{"category":"credit_card","creditCardName":"SCB","signagePhotos":["https://evil.example.com/x.jpg"]}},
		{"id":"2","value":{"category":"credit_card","creditCardName":"SCB M Visa","rule":"2 hrs","location":"1F"}}
	]`)
	got := parseReservedEntries(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 reserved, got %d", len(got))
	}
	if got[0].ProgramOther == nil || *got[0].ProgramOther != "SCB M Visa" {
		t.Fatalf("unexpected reserved: %+v", got[0])
	}
}

func TestParseEVEntries_RejectsForeignSignageURL(t *testing.T) {
	raw := []byte(`[
		{"id":"1","value":{
			"providerName":"ea_anywhere",
			"signagePhotos":["https://evil.example.com/ev.jpg"],
			"connectors":[{"id":"c1","connectorType":"TYPE_2","total":"1"}]
		}},
		{"id":"2","value":{
			"providerName":"ea_anywhere",
			"location":"B2",
			"connectors":[{"id":"c1","connectorType":"TYPE_2","total":"1"}]
		}}
	]`)
	got := parseEVEntries(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 EV, got %d", len(got))
	}
	if got[0].Floor == nil || *got[0].Floor != "B2" {
		t.Fatalf("unexpected EV kept: %+v", got[0])
	}
}

func TestCleanMediaURLs_RejectsTooMany(t *testing.T) {
	urls := make([]string, maxPrivilegeSignagePhotos+1)
	for i := range urls {
		urls[i] = testMediaPrefix + "u/submissions/" + string(rune('a'+i)) + ".jpg"
	}
	if _, ok := cleanMediaURLs(urls); ok {
		t.Fatal("expected too many photos to fail")
	}
	if _, ok := cleanMediaURLs(nil); !ok {
		t.Fatal("empty signage should be allowed")
	}
	good := []string{testMediaPrefix + "u/submissions/a.jpg"}
	got, ok := cleanMediaURLs(good)
	if !ok || len(got) != 1 {
		t.Fatalf("expected valid media URL to pass, got %v ok=%v", got, ok)
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
