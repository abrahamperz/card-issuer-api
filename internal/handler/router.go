package handler

import (
	"net/http"
)

func NewRouter(
	cardHandler *CardHandler,
	cardholderHandler *CardholderHandler,
	batchHandler *BatchHandler,
	healthHandler *HealthHandler,
	middlewares ...func(http.Handler) http.Handler,
) http.Handler {
	mux := http.NewServeMux()

	// Health and readiness routes
	mux.HandleFunc("GET /health", healthHandler.HandleHealth)
	mux.HandleFunc("GET /ready", healthHandler.HandleReady)

	// Cardholder routes
	mux.HandleFunc("POST /v1/cardholders", cardholderHandler.HandleCreateCardholder)
	mux.HandleFunc("GET /v1/cardholders", cardholderHandler.HandleListCardholders)
	mux.HandleFunc("GET /v1/cardholders/{id}", cardholderHandler.HandleGetCardholder)
	mux.HandleFunc("PUT /v1/cardholders/{id}", cardholderHandler.HandleUpdateCardholder)

	// Card routes
	mux.HandleFunc("POST /v1/cards", cardHandler.HandleCreateCard)
	mux.HandleFunc("GET /v1/cards", cardHandler.HandleListCards)
	mux.HandleFunc("GET /v1/cards/{id}", cardHandler.HandleGetCard)
	mux.HandleFunc("POST /v1/cards/{id}/issue", cardHandler.HandleIssueCard)
	mux.HandleFunc("POST /v1/cards/{id}/suspend", cardHandler.HandleSuspendCard)
	mux.HandleFunc("POST /v1/cards/{id}/reactivate", cardHandler.HandleReactivateCard)
	mux.HandleFunc("POST /v1/cards/{id}/close", cardHandler.HandleCloseCard)

	// Batch routes
	mux.HandleFunc("POST /v1/batch/card-status", batchHandler.HandleBatchStatusUpdate)
	mux.HandleFunc("GET /v1/batch/{id}", batchHandler.HandleGetBatchOperation)

	// Apply middleware chain
	var handler http.Handler = mux
	// Apply in reverse order so the first middleware in the slice is the outermost
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}
