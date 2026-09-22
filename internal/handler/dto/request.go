package dto

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

type CreateCardholderRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

func (r *CreateCardholderRequest) Validate() error {
	if strings.TrimSpace(r.FirstName) == "" {
		return errors.New("first_name is required")
	}
	if strings.TrimSpace(r.LastName) == "" {
		return errors.New("last_name is required")
	}
	return nil
}

type UpdateCardholderRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type CreateCardRequest struct {
	CardholderID string `json:"cardholder_id"` // UUID as string
}

func (r *CreateCardRequest) Validate() error {
	if strings.TrimSpace(r.CardholderID) == "" {
		return errors.New("cardholder_id is required")
	}
	if _, err := uuid.Parse(r.CardholderID); err != nil {
		return errors.New("cardholder_id must be a valid UUID")
	}
	return nil
}

type BatchStatusUpdateRequest struct {
	CardIDs []string `json:"card_ids"` // UUIDs as strings
	Action  string   `json:"action"`   // SUSPEND, ACTIVATE, CLOSE, REACTIVATE
	Mode    string   `json:"mode"`     // atomic, partial
}

func (r *BatchStatusUpdateRequest) Validate() error {
	if len(r.CardIDs) == 0 {
		return errors.New("card_ids must contain at least one ID")
	}
	for _, id := range r.CardIDs {
		if _, err := uuid.Parse(id); err != nil {
			return errors.New("all card_ids must be valid UUIDs")
		}
	}

	action := strings.ToUpper(r.Action)
	if action != "SUSPEND" && action != "ACTIVATE" && action != "CLOSE" && action != "REACTIVATE" {
		return errors.New("invalid action")
	}

	mode := strings.ToLower(r.Mode)
	if mode != "atomic" && mode != "partial" {
		return errors.New("invalid mode, must be 'atomic' or 'partial'")
	}

	return nil
}
