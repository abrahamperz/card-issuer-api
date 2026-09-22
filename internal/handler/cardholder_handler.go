package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/handler/dto"
)

type CardholderService interface {
	CreateCardholder(ctx context.Context, firstName, lastName, email, phone string) (*domain.Cardholder, error)
	GetCardholder(ctx context.Context, id uuid.UUID) (*domain.Cardholder, error)
	ListCardholders(ctx context.Context, limit, offset int) ([]*domain.Cardholder, int, error)
	UpdateCardholder(ctx context.Context, id uuid.UUID, firstName, lastName, email, phone string) (*domain.Cardholder, error)
}

type CardholderHandler struct {
	service CardholderService
}

func NewCardholderHandler(service CardholderService) *CardholderHandler {
	return &CardholderHandler{service: service}
}

func (h *CardholderHandler) HandleCreateCardholder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	var req dto.CreateCardholderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_INPUT")
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "VALIDATION_FAILED")
		return
	}

	created, err := h.service.CreateCardholder(ctx, req.FirstName, req.LastName, req.Email, req.Phone)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, dto.CardholderToResponse(created))
}

func (h *CardholderHandler) HandleGetCardholder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	idStr := extractPathParam(r, "id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing cardholder id", "INVALID_INPUT")
		return
	}

	chID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cardholder id format", "INVALID_INPUT")
		return
	}

	ch, err := h.service.GetCardholder(ctx, chID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.CardholderToResponse(ch))
}

func (h *CardholderHandler) HandleListCardholders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

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

	cardholders, total, err := h.service.ListCardholders(ctx, limit, offset)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	resps := make([]dto.CardholderResponse, 0, len(cardholders))
	for _, ch := range cardholders {
		resps = append(resps, dto.CardholderToResponse(ch))
	}

	writeJSON(w, http.StatusOK, dto.ListResponse[dto.CardholderResponse]{
		Data:   resps,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *CardholderHandler) HandleUpdateCardholder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	idStr := extractPathParam(r, "id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing cardholder id", "INVALID_INPUT")
		return
	}

	chID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cardholder id format", "INVALID_INPUT")
		return
	}

	var req dto.UpdateCardholderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_INPUT")
		return
	}

	updated, err := h.service.UpdateCardholder(ctx, chID, req.FirstName, req.LastName, req.Email, req.Phone)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.CardholderToResponse(updated))
}
