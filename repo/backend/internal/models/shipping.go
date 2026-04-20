package models

import "time"

// ShippingRegion represents a geographic delivery zone.
type ShippingRegion struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	CutoffTime string    `json:"cutoff_time"` // "HH:MM:SS" from DB TIME column
	Timezone   string    `json:"timezone"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ShippingTemplate defines fees and timing for a delivery method within a region.
type ShippingTemplate struct {
	ID             uint64    `json:"id"`
	RegionID       uint64    `json:"region_id"`
	DeliveryMethod string    `json:"delivery_method"` // "pickup" | "courier"
	MinWeightKg    float64   `json:"min_weight_kg"`
	MaxWeightKg    float64   `json:"max_weight_kg"`
	MinQuantity    int       `json:"min_quantity"`
	MaxQuantity    int       `json:"max_quantity"`
	FeeAmount      float64   `json:"fee_amount"`
	Currency       string    `json:"currency"`
	LeadTimeHours  int       `json:"lead_time_hours"`
	WindowHours    int       `json:"window_hours"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EstimateResult is returned by the shipping estimate endpoint.
type EstimateResult struct {
	Fee                    float64 `json:"fee"`
	Currency               string  `json:"currency"`
	EstimatedArrivalWindow string  `json:"estimated_arrival_window,omitempty"`
}
