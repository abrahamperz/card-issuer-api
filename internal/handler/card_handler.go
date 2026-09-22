package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/handler/dto"
)

// CardService interface defines the required methods for card operations
type CardService interface {
	CreateCard(ctx context.Context, cardholderID uuid.UUID) (*domain.Card, error)
	GetCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error)
	ListCards(ctx context.Context, status *domain.CardStatus, limit, offset int) ([]*domain.Card, int, error)
	IssueCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error)
	SuspendCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error)
	ReactivateCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error)
	CloseCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error)
}

type CardHandler struct {
	service CardService
}

func NewCardHandler(service CardService) *CardHandler {
	return &CardHandler{service: service}
}

func (h *CardHandler) HandleCreateCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx) // Ensure tenant exists

	var req dto.CreateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_INPUT")
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "VALIDATION_FAILED")
		return
	}

	chID, err := uuid.Parse(req.CardholderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cardholder_id format", "INVALID_INPUT")
		return
	}

	card, err := h.service.CreateCard(ctx, chID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, dto.CardToResponse(card))
}

func (h *CardHandler) HandleGetCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	idStr := extractPathParam(r, "id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing card id", "INVALID_INPUT")
		return
	}

	cardID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid card id format", "INVALID_INPUT")
		return
	}

	card, err := h.service.GetCard(ctx, cardID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.CardToResponse(card))
}

func (h *CardHandler) HandleListCards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	statusStr := r.URL.Query().Get("status")
	var statusPtr *domain.CardStatus
	if statusStr != "" {
		s := domain.CardStatus(strings.ToUpper(statusStr))
		statusPtr = &s
	}

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	cards, total, err := h.service.ListCards(ctx, statusPtr, limit, offset)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	respCards := make([]dto.CardResponse, 0, len(cards))
	for _, c := range cards {
		respCards = append(respCards, dto.CardToResponse(c))
	}

	writeJSON(w, http.StatusOK, dto.ListResponse[dto.CardResponse]{
		Data:   respCards,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *CardHandler) HandleIssueCard(w http.ResponseWriter, r *http.Request) {
	h.handleStateTransition(w, r, h.service.IssueCard)
}

func (h *CardHandler) HandleSuspendCard(w http.ResponseWriter, r *http.Request) {
	h.handleStateTransition(w, r, h.service.SuspendCard)
}

func (h *CardHandler) HandleReactivateCard(w http.ResponseWriter, r *http.Request) {
	h.handleStateTransition(w, r, h.service.ReactivateCard)
}

func (h *CardHandler) HandleCloseCard(w http.ResponseWriter, r *http.Request) {
	h.handleStateTransition(w, r, h.service.CloseCard)
}

func (h *CardHandler) handleStateTransition(w http.ResponseWriter, r *http.Request, action func(context.Context, uuid.UUID) (*domain.Card, error)) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	idStr := extractPathParam(r, "id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing card id", "INVALID_INPUT")
		return
	}

	cardID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid card id format", "INVALID_INPUT")
		return
	}

	card, err := action(ctx, cardID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.CardToResponse(card))
}

// --- Helper Functions ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, dto.ErrorResponse{
		Error: message,
		Code:  code,
	})
}

func extractPathParam(r *http.Request, param string) string {
	if val := r.PathValue(param); val != "" {
		return val
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func handleDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error(), "CONFLICT")
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "INVALID_TRANSITION")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_INPUT")
	case errors.Is(err, domain.ErrDuplicatePAN):
		writeError(w, http.StatusConflict, "card PAN generation collision, please retry", "DUPLICATE_PAN")
	case errors.Is(err, domain.ErrBatchPartialFailure):
		writeError(w, http.StatusMultiStatus, err.Error(), "PARTIAL_FAILURE")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
	}
}
