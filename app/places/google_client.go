package places

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googlePlacesAutocompleteURL = "https://places.googleapis.com/v1/places:autocomplete"
	googlePlacesDetailsBaseURL  = "https://places.googleapis.com/v1/places/"
	googlePlacesHTTPTimeout     = 5 * time.Second
	defaultBiasRadiusMeters     = 30000.0
	defaultBiasLat              = 13.7563
	defaultBiasLng              = 100.5018
)

// GooglePlacesClient calls Google Places API (New).
type GooglePlacesClient interface {
	Autocomplete(ctx context.Context, req GoogleAutocompleteRequest) ([]PlacePrediction, error)
	PlaceDetails(ctx context.Context, req GooglePlaceDetailsRequest) (*PlaceDetails, error)
}

type GoogleAutocompleteRequest struct {
	Input        string
	LanguageCode string
	SessionToken string
	Latitude     *float64
	Longitude    *float64
}

type GooglePlaceDetailsRequest struct {
	PlaceID      string
	LanguageCode string
	SessionToken string
}

type PlacePrediction struct {
	PlaceID         string  `json:"placeId"`
	Name            string  `json:"name"`
	Address         string  `json:"address"`
	DistanceMeters  *int    `json:"distanceMeters,omitempty"`
}

type PlaceDetails struct {
	PlaceID     string  `json:"placeId"`
	Name        string  `json:"name"`
	Address     string  `json:"address"`
	AddressLine string  `json:"addressLine,omitempty"`
	Subdistrict string  `json:"subdistrict,omitempty"`
	District    string  `json:"district,omitempty"`
	Province    string  `json:"province,omitempty"`
	PostalCode  string  `json:"postalCode,omitempty"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

type AutocompleteResponse struct {
	Predictions []PlacePrediction `json:"predictions"`
}

type googlePlacesHTTPClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewGooglePlacesClient(apiKey string) GooglePlacesClient {
	return &googlePlacesHTTPClient{
		apiKey: strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: googlePlacesHTTPTimeout,
		},
	}
}

func (c *googlePlacesHTTPClient) configured() bool {
	return c != nil && c.apiKey != ""
}

type googleAutocompleteBody struct {
	Input                 string                      `json:"input"`
	LanguageCode          string                      `json:"languageCode,omitempty"`
	RegionCode            string                      `json:"regionCode,omitempty"`
	IncludedRegionCodes   []string                    `json:"includedRegionCodes,omitempty"`
	SessionToken          string                      `json:"sessionToken,omitempty"`
	Origin                *googleLatLng               `json:"origin,omitempty"`
	LocationBias          *googleLocationBias         `json:"locationBias,omitempty"`
	IncludeQueryPredictions bool                      `json:"includeQueryPredictions"`
}

type googleLatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type googleLocationBias struct {
	Circle googleCircle `json:"circle"`
}

type googleCircle struct {
	Center googleLatLng `json:"center"`
	Radius float64      `json:"radius"`
}

type googleAutocompleteResponse struct {
	Suggestions []struct {
		PlacePrediction *struct {
			Place           string `json:"place"`
			PlaceID         string `json:"placeId"`
			DistanceMeters  *int   `json:"distanceMeters"`
			Text            *struct {
				Text string `json:"text"`
			} `json:"text"`
			StructuredFormat *struct {
				MainText *struct {
					Text string `json:"text"`
				} `json:"mainText"`
				SecondaryText *struct {
					Text string `json:"text"`
				} `json:"secondaryText"`
			} `json:"structuredFormat"`
		} `json:"placePrediction"`
	} `json:"suggestions"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

type googlePlaceDetailsResponse struct {
	ID                string                   `json:"id"`
	FormattedAddress  string                   `json:"formattedAddress"`
	AddressComponents []googleAddressComponent `json:"addressComponents"`
	DisplayName       *struct {
		Text string `json:"text"`
	} `json:"displayName"`
	Location *googleLatLng `json:"location"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (c *googlePlacesHTTPClient) Autocomplete(
	ctx context.Context,
	req GoogleAutocompleteRequest,
) ([]PlacePrediction, error) {
	if !c.configured() {
		return nil, ErrGooglePlacesNotConfigured
	}

	input := strings.TrimSpace(req.Input)
	if input == "" {
		return []PlacePrediction{}, nil
	}

	lat, lng := defaultBiasLat, defaultBiasLng
	if req.Latitude != nil && req.Longitude != nil {
		lat, lng = *req.Latitude, *req.Longitude
	}

	body := googleAutocompleteBody{
		Input:               input,
		LanguageCode:        normalizeLanguage(req.LanguageCode),
		RegionCode:          "TH",
		IncludedRegionCodes: []string{"th"},
		SessionToken:        strings.TrimSpace(req.SessionToken),
		Origin:              &googleLatLng{Latitude: lat, Longitude: lng},
		LocationBias: &googleLocationBias{
			Circle: googleCircle{
				Center: googleLatLng{Latitude: lat, Longitude: lng},
				Radius: defaultBiasRadiusMeters,
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal autocomplete body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, googlePlacesAutocompleteURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create autocomplete request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Goog-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("autocomplete request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read autocomplete response: %w", err)
	}

	var parsed googleAutocompleteResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode autocomplete response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("google autocomplete error: %s (%s)", parsed.Error.Message, parsed.Error.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google autocomplete http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	out := make([]PlacePrediction, 0, len(parsed.Suggestions))
	for _, suggestion := range parsed.Suggestions {
		pred := suggestion.PlacePrediction
		if pred == nil {
			continue
		}
		placeID := strings.TrimSpace(pred.PlaceID)
		if placeID == "" {
			placeID = strings.TrimPrefix(strings.TrimSpace(pred.Place), "places/")
		}
		if placeID == "" {
			continue
		}

		name := ""
		address := ""
		if pred.StructuredFormat != nil {
			if pred.StructuredFormat.MainText != nil {
				name = strings.TrimSpace(pred.StructuredFormat.MainText.Text)
			}
			if pred.StructuredFormat.SecondaryText != nil {
				address = strings.TrimSpace(pred.StructuredFormat.SecondaryText.Text)
			}
		}
		if name == "" && pred.Text != nil {
			name = strings.TrimSpace(pred.Text.Text)
		}
		if name == "" {
			name = placeID
		}
		if address == "" && pred.Text != nil {
			full := strings.TrimSpace(pred.Text.Text)
			if full != "" && full != name {
				address = full
			}
		}

		out = append(out, PlacePrediction{
			PlaceID:        placeID,
			Name:           name,
			Address:        address,
			DistanceMeters: pred.DistanceMeters,
		})
	}
	return out, nil
}

func (c *googlePlacesHTTPClient) PlaceDetails(
	ctx context.Context,
	req GooglePlaceDetailsRequest,
) (*PlaceDetails, error) {
	if !c.configured() {
		return nil, ErrGooglePlacesNotConfigured
	}

	placeID := strings.TrimPrefix(strings.TrimSpace(req.PlaceID), "places/")
	if placeID == "" {
		return nil, fmt.Errorf("place id is required")
	}

	endpoint, err := url.Parse(googlePlacesDetailsBaseURL + url.PathEscape(placeID))
	if err != nil {
		return nil, fmt.Errorf("build details url: %w", err)
	}
	query := endpoint.Query()
	query.Set("languageCode", normalizeLanguage(req.LanguageCode))
	if token := strings.TrimSpace(req.SessionToken); token != "" {
		query.Set("sessionToken", token)
	}
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create details request: %w", err)
	}
	httpReq.Header.Set("X-Goog-Api-Key", c.apiKey)
	httpReq.Header.Set("X-Goog-FieldMask", "id,displayName,formattedAddress,addressComponents,location")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("details request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read details response: %w", err)
	}

	var parsed googlePlaceDetailsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode details response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("google details error: %s (%s)", parsed.Error.Message, parsed.Error.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google details http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if parsed.Location == nil {
		return nil, fmt.Errorf("google details missing location")
	}

	name := placeID
	if parsed.DisplayName != nil && strings.TrimSpace(parsed.DisplayName.Text) != "" {
		name = strings.TrimSpace(parsed.DisplayName.Text)
	}
	address := strings.TrimSpace(parsed.FormattedAddress)
	if address == "" {
		address = name
	}
	parts := parseAddressComponents(parsed.AddressComponents)
	addressLine := parts.AddressLine
	if addressLine == "" {
		addressLine = address
	}
	id := strings.TrimPrefix(strings.TrimSpace(parsed.ID), "places/")
	if id == "" {
		id = placeID
	}

	return &PlaceDetails{
		PlaceID:     id,
		Name:        name,
		Address:     address,
		AddressLine: addressLine,
		Subdistrict: parts.Subdistrict,
		District:    parts.District,
		Province:    parts.Province,
		PostalCode:  parts.PostalCode,
		Latitude:    parsed.Location.Latitude,
		Longitude:   parsed.Location.Longitude,
	}, nil
}

func normalizeLanguage(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if strings.HasPrefix(code, "th") {
		return "th"
	}
	if code == "" {
		return "en"
	}
	if i := strings.IndexByte(code, '-'); i > 0 {
		return code[:i]
	}
	return code
}
