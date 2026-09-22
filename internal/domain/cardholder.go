package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Cardholder struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	FirstName string
	LastName  string
	Email     string
	Phone     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewCardholder(tenantID uuid.UUID, firstName, lastName string) *Cardholder {
	now := time.Now().UTC()
	return &Cardholder{
		ID:        uuid.New(),
		TenantID:  tenantID,
		FirstName: firstName,
		LastName:  lastName,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (c *Cardholder) Validate() error {
	if c.FirstName == "" {
		return errors.New("first name is required")
	}
	if c.LastName == "" {
		return errors.New("last name is required")
	}
	return nil
}
