package dto

import (
	"time"

	"github.com/novopayment/card-issuer-api/internal/domain"
)

// CardResponse is the API response for a card.
// SECURITY: Never includes PAN, encrypted data, or internal fields like Version.
type CardResponse struct {
	ID           string `json:"id"`
	CardholderID string `json:"cardholder_id"`
	LastFour     string `json:"last_four,omitempty"`
	ExpiryMonth  int    `json:"expiry_month,omitempty"`
	ExpiryYear   int    `json:"expiry_year,omitempty"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// CardToResponse maps a domain Card to a safe API response.
func CardToResponse(card *domain.Card) CardResponse {
	if card == nil {
		return CardResponse{}
	}
	return CardResponse{
		ID:           card.ID.String(),
		CardholderID: card.CardholderID.String(),
		LastFour:     card.LastFourDigits,
		ExpiryMonth:  card.ExpiryMonth,
		ExpiryYear:   card.ExpiryYear,
		Status:       string(card.Status),
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}

// CardholderResponse is the API response for a cardholder.
type CardholderResponse struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CardholderToResponse maps a domain Cardholder to an API response.
func CardholderToResponse(ch *domain.Cardholder) CardholderResponse {
	if ch == nil {
		return CardholderResponse{}
	}
	return CardholderResponse{
		ID:        ch.ID.String(),
		FirstName: ch.FirstName,
		LastName:  ch.LastName,
		Email:     ch.Email,
		Phone:     ch.Phone,
		CreatedAt: ch.CreatedAt.Format(time.RFC3339),
		UpdatedAt: ch.UpdatedAt.Format(time.RFC3339),
	}
}

// BatchResponse is the API response for a batch operation.
type BatchResponse struct {
	BatchID   string              `json:"batch_id"`
	Status    string              `json:"status"`
	Mode      string              `json:"mode"`
	Action    string              `json:"action"`
	Total     int                 `json:"total"`
	Succeeded int                 `json:"succeeded"`
	Failed    int                 `json:"failed"`
	Results   []BatchItemResponse `json:"results,omitempty"`
}

// BatchItemResponse is the per-card result in a batch.
type BatchItemResponse struct {
	CardID  string `json:"card_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// BatchToResponse maps a domain BatchOperation to an API response.
func BatchToResponse(op *domain.BatchOperation) BatchResponse {
	if op == nil {
		return BatchResponse{}
	}

	results := make([]BatchItemResponse, 0, len(op.Results))
	for _, res := range op.Results {
		results = append(results, BatchItemResponse{
			CardID:  res.CardID.String(),
			Success: res.Success,
			Error:   res.Error,
		})
	}

	return BatchResponse{
		BatchID:   op.ID.String(),
		Status:    string(op.Status),
		Mode:      string(op.Mode),
		Action:    string(op.Action),
		Total:     op.TotalCount,
		Succeeded: op.SuccessCount,
		Failed:    op.FailureCount,
		Results:   results,
	}
}

// ListResponse is a generic paginated list response.
type ListResponse[T any] struct {
	Data   []T `json:"data"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// ErrorResponse is returned for all API errors.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}
