package submissions

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/RinTanth/go-backend/app/mediaurl"
)

const maxPrivilegeSignagePhotos = 5

type stampEntryDraft struct {
	ID    string           `json:"id"`
	Value stampValueDraft  `json:"value"`
}

type stampValueDraft struct {
	Category       string   `json:"category"`
	SpendingUpTo   string   `json:"spendingUpTo"`
	OtherBenefits  string   `json:"otherBenefits"`
	ActivityName   string   `json:"activityName"`
	Conditions     string   `json:"conditions"`
	FreeHour       string   `json:"freeHour"`
	BankName       *string  `json:"bankName"`
	CardName       *string  `json:"cardName"`
	MembershipName *string  `json:"membershipName"`
	StampLocation  string   `json:"stampLocation"`
	SignagePhotos  []string `json:"signagePhotos"`
}

type reservedEntryDraft struct {
	ID    string             `json:"id"`
	Value reservedValueDraft `json:"value"`
}

type reservedValueDraft struct {
	Category       string   `json:"category"`
	CreditCardName string   `json:"creditCardName"`
	MembershipName string   `json:"membershipName"`
	CompanyName    string   `json:"companyName"`
	Rule           string   `json:"rule"`
	Location       string   `json:"location"`
	SignagePhotos  []string `json:"signagePhotos"`
}

type evEntryDraft struct {
	ID    string       `json:"id"`
	Value evValueDraft `json:"value"`
}

type evValueDraft struct {
	ProviderName  *string           `json:"providerName"`
	Connectors    []evConnectorDraft `json:"connectors"`
	Rule          string            `json:"rule"`
	Location      string            `json:"location"`
	SignagePhotos []string          `json:"signagePhotos"`
}

type evConnectorDraft struct {
	ID            string  `json:"id"`
	ConnectorType *string `json:"connectorType"`
	Total         string  `json:"total"`
}

type publishedStamp struct {
	ValidationType       string
	ProgramOther         *string
	ConditionDescription string
	Notes                *string
	ValidationLocation   *string
	ProgramName          *string
	ProgramProvider      *string
	ProgramCategory      *string
	FreeMinutes          *int
	MinSpend             float64
	SignagePhotos        []string
}

type publishedReserved struct {
	ReservationType string
	ProgramOther    *string
	Conditions      *string
	Floor           *string
	SignagePhotos   []string
}

type publishedEV struct {
	ProviderName  string
	Floor         *string
	Rule          *string
	Connectors    []publishedEVConnector
	SignagePhotos []string
}

type publishedEVConnector struct {
	ConnectorType string
	PowerType     string
	PowerKW       int
}

func parseStampEntries(raw json.RawMessage) []publishedStamp {
	entries := unmarshalPrivilegeEntries[stampEntryDraft](raw)
	out := make([]publishedStamp, 0, len(entries))
	for _, entry := range entries {
		if published, ok := mapStampEntry(entry.Value); ok {
			out = append(out, published)
		}
	}
	return out
}

func parseReservedEntries(raw json.RawMessage) []publishedReserved {
	entries := unmarshalPrivilegeEntries[reservedEntryDraft](raw)
	out := make([]publishedReserved, 0, len(entries))
	for _, entry := range entries {
		if published, ok := mapReservedEntry(entry.Value); ok {
			out = append(out, published)
		}
	}
	return out
}

func parseEVEntries(raw json.RawMessage) []publishedEV {
	entries := unmarshalPrivilegeEntries[evEntryDraft](raw)
	out := make([]publishedEV, 0, len(entries))
	for _, entry := range entries {
		if published, ok := mapEVEntry(entry.Value); ok {
			out = append(out, published)
		}
	}
	return out
}

func unmarshalPrivilegeEntries[T any](raw json.RawMessage) []T {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var entries []T
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	return entries
}

func mapStampEntry(value stampValueDraft) (publishedStamp, bool) {
	category := strings.ToLower(strings.TrimSpace(value.Category))
	validationType, ok := mapStampCategory(category)
	if !ok {
		return publishedStamp{}, false
	}

	location := trimToPtr(value.StampLocation)
	notes := trimToPtr(value.OtherBenefits)
	freeMinutes := parseFreeHourMinutes(value.FreeHour)
	minSpend := 0.0
	if parsed := parseMoney(&value.SpendingUpTo); parsed != nil {
		minSpend = *parsed
	}
	signagePhotos, ok := cleanMediaURLs(value.SignagePhotos)
	if !ok {
		return publishedStamp{}, false
	}

	out := publishedStamp{
		ValidationType:     validationType,
		Notes:              notes,
		ValidationLocation: location,
		FreeMinutes:        freeMinutes,
		MinSpend:           minSpend,
		SignagePhotos:      signagePhotos,
	}

	switch category {
	case "spending":
		out.ConditionDescription = firstNonEmpty(
			strings.TrimSpace(value.SpendingUpTo),
			"spending condition",
		)
	case "activity":
		out.ProgramOther = trimToPtr(value.ActivityName)
		out.ConditionDescription = firstNonEmpty(
			strings.TrimSpace(value.Conditions),
			strings.TrimSpace(value.ActivityName),
			"activity condition",
		)
	case "bank_card":
		bank := labelOrRaw(bankLabels, value.BankName)
		card := labelOrRaw(cardLabels, value.CardName)
		if bank != "" && card != "" {
			out.ProgramName = &card
			out.ProgramProvider = &bank
			cat := "credit_card"
			out.ProgramCategory = &cat
		} else if card != "" {
			out.ProgramOther = &card
		} else if bank != "" {
			out.ProgramOther = &bank
		}
		out.ConditionDescription = firstNonEmpty(
			joinNonEmpty(" · ", bank, card),
			"credential condition",
		)
	case "membership":
		membership := labelOrRaw(membershipLabels, value.MembershipName)
		if membership != "" {
			out.ProgramName = &membership
			provider := membership
			out.ProgramProvider = &provider
			cat := "membership"
			out.ProgramCategory = &cat
		}
		out.ConditionDescription = firstNonEmpty(
			membership,
			"membership condition",
		)
	default:
		out.ConditionDescription = "other condition"
	}

	return out, true
}

func mapReservedEntry(value reservedValueDraft) (publishedReserved, bool) {
	category := strings.ToLower(strings.TrimSpace(value.Category))
	reservationType, ok := mapReservedCategory(category)
	if !ok {
		return publishedReserved{}, false
	}

	var programOther *string
	switch category {
	case "credit_card":
		programOther = trimToPtr(value.CreditCardName)
	case "members":
		programOther = trimToPtr(value.MembershipName)
	case "company":
		programOther = trimToPtr(value.CompanyName)
	}
	if programOther == nil {
		return publishedReserved{}, false
	}
	signagePhotos, ok := cleanMediaURLs(value.SignagePhotos)
	if !ok {
		return publishedReserved{}, false
	}

	return publishedReserved{
		ReservationType: reservationType,
		ProgramOther:    programOther,
		Conditions:      trimToPtr(value.Rule),
		Floor:           trimToPtr(value.Location),
		SignagePhotos:   signagePhotos,
	}, true
}

func mapEVEntry(value evValueDraft) (publishedEV, bool) {
	providerSlug := ""
	if value.ProviderName != nil {
		providerSlug = strings.ToLower(strings.TrimSpace(*value.ProviderName))
	}
	providerName := evProviderLabels[providerSlug]
	if providerName == "" {
		providerName = strings.TrimSpace(providerSlug)
	}
	if providerName == "" {
		return publishedEV{}, false
	}

	connectors := make([]publishedEVConnector, 0)
	for _, connector := range value.Connectors {
		mapped, ok := mapEVConnector(connector)
		if !ok {
			continue
		}
		count := parsePositiveInt(connector.Total, 1)
		for i := 0; i < count; i++ {
			connectors = append(connectors, mapped)
		}
	}
	if len(connectors) == 0 {
		return publishedEV{}, false
	}
	signagePhotos, ok := cleanMediaURLs(value.SignagePhotos)
	if !ok {
		return publishedEV{}, false
	}

	return publishedEV{
		ProviderName:  providerName,
		Floor:         trimToPtr(value.Location),
		Rule:          trimToPtr(value.Rule),
		Connectors:    connectors,
		SignagePhotos: signagePhotos,
	}, true
}

func mapStampCategory(category string) (string, bool) {
	switch category {
	case "spending":
		return "spending", true
	case "activity":
		return "event_ticket", true
	case "bank_card":
		return "credential", true
	case "membership":
		return "membership", true
	default:
		return "", false
	}
}

func mapReservedCategory(category string) (string, bool) {
	switch category {
	case "credit_card":
		return "cardholder", true
	case "company":
		return "tenant", true
	case "members":
		return "other", true
	default:
		return "", false
	}
}

func mapEVConnector(connector evConnectorDraft) (publishedEVConnector, bool) {
	if connector.ConnectorType == nil {
		return publishedEVConnector{}, false
	}
	switch strings.ToUpper(strings.TrimSpace(*connector.ConnectorType)) {
	case "TYPE_1", "TYPE_2":
		return publishedEVConnector{ConnectorType: "AC_Type_2", PowerType: "AC", PowerKW: 7}, true
	case "TESLA":
		return publishedEVConnector{ConnectorType: "Tesla", PowerType: "DC", PowerKW: 150}, true
	case "CCS1", "CCS2":
		return publishedEVConnector{ConnectorType: "CCS2", PowerType: "DC", PowerKW: 50}, true
	case "CHADEMO":
		return publishedEVConnector{ConnectorType: "CHAdeMO", PowerType: "DC", PowerKW: 50}, true
	default:
		return publishedEVConnector{}, false
	}
}

func parseFreeHourMinutes(raw string) *int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	cleaned := strings.Builder{}
	for _, r := range trimmed {
		if (r >= '0' && r <= '9') || r == '.' {
			cleaned.WriteRune(r)
		}
	}
	hours, err := strconv.ParseFloat(cleaned.String(), 64)
	if err != nil || hours <= 0 {
		return nil
	}
	minutes := int(hours * 60)
	if minutes <= 0 {
		return nil
	}
	return &minutes
}

func parsePositiveInt(raw string, fallback int) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 20 {
		return 20
	}
	return n
}

func trimToPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func cleanURLs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// cleanMediaURLs trims, enforces max count, and requires Harjod media-bucket URLs
// (same rules as PATCH privilege signage). Empty input is valid.
func cleanMediaURLs(values []string) ([]string, bool) {
	out := cleanURLs(values)
	if len(out) > maxPrivilegeSignagePhotos {
		return nil, false
	}
	if !mediaurl.ValidMediaURLs(out, mediaurl.MaxURLLen) {
		return nil, false
	}
	return out, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func joinNonEmpty(sep string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, sep)
}

func labelOrRaw(labels map[string]string, raw *string) string {
	if raw == nil {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(*raw))
	if key == "" {
		return ""
	}
	if label, ok := labels[key]; ok {
		return label
	}
	return strings.TrimSpace(*raw)
}

func joinSpecialConditions(values []string) *string {
	parts := cleanURLs(values)
	if len(parts) == 0 {
		return nil
	}
	joined := strings.Join(parts, "\n")
	return &joined
}

var bankLabels = map[string]string{
	"bbl":          "Bangkok Bank",
	"kbank":        "Kasikornbank",
	"ktb":          "Krungthai Bank",
	"scb":          "Siam Commercial Bank",
	"bay":          "Bank of Ayudhya (Krungsri)",
	"tmbthanachart": "TMBThanachart Bank (ttb)",
	"gsb":          "Government Savings Bank",
	"uob":          "UOB Thailand",
	"kkp":          "Kiatnakin Phatra Bank",
	"cimb":         "CIMB Thai Bank",
}

var cardLabels = map[string]string{
	"visa":       "Visa",
	"mastercard": "Mastercard",
	"jcb":        "JCB",
	"unionpay":   "UnionPay",
	"debit":      "Debit Card",
}

var membershipLabels = map[string]string{
	"the1":   "The 1 (Central)",
	"mcard":  "M Card (The Mall)",
	"t1":     "T1 (Dtac Reward)",
	"lotuss": "Lotus's",
	"bigc":   "Big C Loyalty",
}

var evProviderLabels = map[string]string{
	"ea_anywhere":   "EA Anywhere",
	"pea_volta":     "PEA VOLTA",
	"elex_egat":     "EleX by EGAT",
	"tesla":         "Tesla Supercharger",
	"ptt_evstation": "PTT EV Station PluZ",
	"onion":         "Onion EV Charging",
}
