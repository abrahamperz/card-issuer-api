package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/handler/dto"
)

type BatchService interface {
	ProcessBatch(ctx context.Context, action domain.BatchAction, mode domain.BatchMode, cardIDs []uuid.UUID, idempotencyKey string) (*domain.BatchOperation, error)
	GetBatchOperation(ctx context.Context, id uuid.UUID) (*domain.BatchOperation, error)
}

type BatchHandler struct {
	service BatchService
}

func NewBatchHandler(service BatchService) *BatchHandler {
	return &BatchHandler{service: service}
}

func (h *BatchHandler) HandleBatchStatusUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	var req dto.BatchStatusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_INPUT")
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "VALIDATION_FAILED")
		return
	}

	var action domain.BatchAction
	switch strings.ToUpper(req.Action) {
	case "SUSPEND":
		action = domain.BatchActionSuspend
	case "ACTIVATE":
		action = domain.BatchActionActivate
	case "CLOSE":
		action = domain.BatchActionClose
	case "REACTIVATE":
		action = domain.BatchActionReactivate
	default:
		writeError(w, http.StatusBadRequest, "invalid action: "+req.Action, "INVALID_INPUT")
		return
	}

	var mode domain.BatchMode
	if strings.ToLower(req.Mode) == "atomic" {
		mode = domain.BatchModeAtomic
	} else {
		mode = domain.BatchModePartial
	}

	cardIDs := make([]uuid.UUID, 0, len(req.CardIDs))
	for _, idStr := range req.CardIDs {
		cardID, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid card id format: "+idStr, "INVALID_INPUT")
			return
		}
		cardIDs = append(cardIDs, cardID)
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	op, err := h.service.ProcessBatch(ctx, action, mode, cardIDs, idempotencyKey)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.BatchToResponse(op))
}

func (h *BatchHandler) HandleGetBatchOperation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = domain.MustTenantID(ctx)

	idStr := extractPathParam(r, "id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing batch id", "INVALID_INPUT")
		return
	}

	batchID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid batch id format", "INVALID_INPUT")
		return
	}

	op, err := h.service.GetBatchOperation(ctx, batchID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.BatchToResponse(op))
}
