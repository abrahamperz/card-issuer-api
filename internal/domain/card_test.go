package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

func TestPAN_MaskingAndSecurity(t *testing.T) {
	rawPAN := domain.PAN("4111111111111234")

	// 1. String() must be masked
	maskedStr := rawPAN.String()
	if strings.Contains(maskedStr, "411111111111") {
		t.Fatalf("PAN leaked in String(): %s", maskedStr)
	}
	if !strings.HasSuffix(maskedStr, "1234") {
		t.Fatalf("Masked PAN should end with last 4 digits: %s", maskedStr)
	}

	// 2. MarshalJSON() must serialize to masked string
	jsonBytes, err := json.Marshal(rawPAN)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if strings.Contains(string(jsonBytes), "411111111111") {
		t.Fatalf("PAN leaked in JSON output: %s", string(jsonBytes))
	}

	// 3. GoString() must be masked
	goStr := rawPAN.GoString()
	if strings.Contains(goStr, "411111111111") {
		t.Fatalf("PAN leaked in GoString(): %s", goStr)
	}

	// 4. LogValue() must be masked
	logVal := rawPAN.LogValue()
	if strings.Contains(logVal.String(), "411111111111") {
		t.Fatalf("PAN leaked in LogValue(): %s", logVal.String())
	}
}

func TestPAN_LuhnValidation(t *testing.T) {
	tests := []struct {
		name    string
		pan     domain.PAN
		wantErr bool
	}{
		{"Valid Visa", "4532015112830366", false},
		{"Valid Mastercard", "5425233430109903", false},
		{"Invalid Check Digit", "4532015112830367", true},
		{"Too Short", "123456", true},
		{"Too Long", "123456789012345678901", true},
		{"Non-numeric", "453201511283ABCD", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pan.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestGeneratePAN(t *testing.T) {
	for i := 0; i < 20; i++ {
		pan := domain.GeneratePAN()
		if err := pan.Validate(); err != nil {
			t.Fatalf("GeneratePAN produced invalid Luhn PAN: %s, error: %v", pan, err)
		}
		if !strings.HasPrefix(pan.RawValue(), "400000") {
			t.Fatalf("Generated PAN must start with BIN 400000: %s", pan)
		}
	}
}

func TestCard_LifecycleStateMachine(t *testing.T) {
	tenantID := uuid.New()
	cardholderID := uuid.New()

	t.Run("Valid Full Lifecycle Flow", func(t *testing.T) {
		card := domain.NewCard(tenantID, cardholderID)
		if card.Status != domain.CardStatusPending {
			t.Fatalf("expected PENDING, got %s", card.Status)
		}
		if card.Version != 1 {
			t.Fatalf("expected initial version 1, got %d", card.Version)
		}

		// PENDING -> ACTIVE (Issue)
		pan := domain.GeneratePAN()
		err := card.Issue(pan, 12, 2028)
		if err != nil {
			t.Fatalf("Issue failed: %v", err)
		}
		if card.Status != domain.CardStatusActive {
			t.Fatalf("expected ACTIVE, got %s", card.Status)
		}
		if card.Version != 2 {
			t.Fatalf("expected version increment to 2, got %d", card.Version)
		}

		// ACTIVE -> SUSPENDED (Suspend)
		err = card.Suspend()
		if err != nil {
			t.Fatalf("Suspend failed: %v", err)
		}
		if card.Status != domain.CardStatusSuspended {
			t.Fatalf("expected SUSPENDED, got %s", card.Status)
		}

		// SUSPENDED -> ACTIVE (Reactivate)
		err = card.Reactivate()
		if err != nil {
			t.Fatalf("Reactivate failed: %v", err)
		}
		if card.Status != domain.CardStatusActive {
			t.Fatalf("expected ACTIVE, got %s", card.Status)
		}

		// ACTIVE -> CLOSED (Close)
		err = card.Close()
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		if card.Status != domain.CardStatusClosed {
			t.Fatalf("expected CLOSED, got %s", card.Status)
		}

		// CLOSED is terminal — ANY further transition must fail
		if err := card.Reactivate(); err == nil {
			t.Fatal("expected error reactivating CLOSED card, got nil")
		}
		if err := card.Suspend(); err == nil {
			t.Fatal("expected error suspending CLOSED card, got nil")
		}
		if err := card.Close(); err == nil {
			t.Fatal("expected error re-closing CLOSED card, got nil")
		}
	})

	t.Run("Invalid Transitions", func(t *testing.T) {
		// Cannot suspend a PENDING card
		pendingCard := domain.NewCard(tenantID, cardholderID)
		if err := pendingCard.Suspend(); err == nil {
			t.Fatal("expected error suspending PENDING card, got nil")
		}
		if err := pendingCard.Reactivate(); err == nil {
			t.Fatal("expected error reactivating PENDING card, got nil")
		}

		// Can close a PENDING card (cancellation before issuance)
		if err := pendingCard.Close(); err != nil {
			t.Fatalf("expected closing PENDING card to be allowed, got: %v", err)
		}
	})
}
