package dto

// AdminCreateSegmentPriceFactorRequest represents the payload to add/update a segment price factor for a level3 value.
type AdminCreateSegmentPriceFactorRequest struct {
	Level3      string  `json:"level3" validate:"required"`
	PriceFactor float64 `json:"price_factor" validate:"required,gt=0"`
}

type AdminCreateSegmentPriceFactorResponse struct {
	Message string `json:"message"`
}

type AdminSegmentPriceFactorItem struct {
	Level3      string  `json:"level3"`
	PriceFactor float64 `json:"price_factor"`
	CreatedAt   string  `json:"created_at"`
}

type AdminListSegmentPriceFactorsResponse struct {
	Message string                        `json:"message"`
	Items   []AdminSegmentPriceFactorItem `json:"items"`
}

type AdminListLevel3OptionsResponse struct {
	Message string   `json:"message"`
	Items   []string `json:"items"`
}
