package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CardStatus string

const (
	CardStatusPending   CardStatus = "PENDING"
	CardStatusActive    CardStatus = "ACTIVE"
	CardStatusSuspended CardStatus = "SUSPENDED"
	CardStatusClosed    CardStatus = "CLOSED"
)

var ValidTransitions = map[CardStatus][]CardStatus{
	CardStatusPending:   {CardStatusActive, CardStatusClosed},
	CardStatusActive:    {CardStatusSuspended, CardStatusClosed},
	CardStatusSuspended: {CardStatusActive, CardStatusClosed},
	CardStatusClosed:    {},
}

type PAN string

func (p PAN) String() string {
	s := string(p)
	if len(s) < 4 {
		return "****"
	}
	return "****-****-****-" + s[len(s)-4:]
}

func (p PAN) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p PAN) GoString() string {
	return p.String()
}

func (p PAN) LogValue() slog.Value {
	return slog.StringValue(p.String())
}

func (p PAN) Validate() error {
	s := string(p)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 13 || len(s) > 19 {
		return errors.New("invalid PAN length")
	}

	sum := 0
	alternate := false
	for i := len(s) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return errors.New("invalid PAN format")
		}

		if alternate {
			n *= 2
			if n > 9 {
				n = (n % 10) + 1
			}
		}
		sum += n
		alternate = !alternate
	}

	if sum%10 != 0 {
		return errors.New("failed Luhn check")
	}

	return nil
}

func (p PAN) LastFour() string {
	s := string(p)
	if len(s) < 4 {
		return s
	}
	return s[len(s)-4:]
}

func (p PAN) RawValue() string {
	return string(p)
}

func GeneratePAN() PAN {
	prefix := "400000"
	length := 16
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Generate middle digits
	b := make([]byte, length-len(prefix)-1)
	for i := range b {
		b[i] = byte(r.Intn(10)) + '0'
	}

	partial := prefix + string(b)

	// Calculate check digit
	sum := 0
	alternate := true
	for i := len(partial) - 1; i >= 0; i-- {
		n := int(partial[i] - '0')
		if alternate {
			n *= 2
			if n > 9 {
				n = (n % 10) + 1
			}
		}
		sum += n
		alternate = !alternate
	}

	checkDigit := (10 - (sum % 10)) % 10

	return PAN(fmt.Sprintf("%s%d", partial, checkDigit))
}

type Card struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	CardholderID   uuid.UUID
	PANEncrypted   []byte
	PANBlindIndex  string
	LastFourDigits string
	ExpiryMonth    int
	ExpiryYear     int
	Status         CardStatus
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewCard(tenantID, cardholderID uuid.UUID) *Card {
	now := time.Now().UTC()
	return &Card{
		ID:           uuid.New(),
		TenantID:     tenantID,
		CardholderID: cardholderID,
		Status:       CardStatusPending,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (c *Card) canTransitionTo(newStatus CardStatus) bool {
	allowed, ok := ValidTransitions[c.Status]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == newStatus {
			return true
		}
	}
	return false
}

func (c *Card) Issue(pan PAN, expiryMonth, expiryYear int) error {
	if c.Status != CardStatusPending {
		return fmt.Errorf("%w: cannot issue card from status %s (must be PENDING)", ErrInvalidTransition, c.Status)
	}
	if err := pan.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	c.LastFourDigits = pan.LastFour()
	c.ExpiryMonth = expiryMonth
	c.ExpiryYear = expiryYear
	c.Status = CardStatusActive
	c.UpdatedAt = time.Now().UTC()
	c.Version++
	return nil
}

func (c *Card) Suspend() error {
	if c.Status != CardStatusActive {
		return fmt.Errorf("%w: cannot suspend card from status %s (must be ACTIVE)", ErrInvalidTransition, c.Status)
	}

	c.Status = CardStatusSuspended
	c.UpdatedAt = time.Now().UTC()
	c.Version++
	return nil
}

func (c *Card) Reactivate() error {
	if c.Status != CardStatusSuspended {
		return fmt.Errorf("%w: cannot reactivate card from status %s (must be SUSPENDED)", ErrInvalidTransition, c.Status)
	}

	c.Status = CardStatusActive
	c.UpdatedAt = time.Now().UTC()
	c.Version++
	return nil
}

func (c *Card) Close() error {
	if c.Status == CardStatusClosed {
		return fmt.Errorf("%w: card is already CLOSED (terminal state)", ErrInvalidTransition)
	}

	c.Status = CardStatusClosed
	c.UpdatedAt = time.Now().UTC()
	c.Version++
	return nil
}
