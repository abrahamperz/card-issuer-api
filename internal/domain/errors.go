package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict: version mismatch")
	ErrInvalidTransition   = errors.New("invalid state transition")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrTenantRequired      = errors.New("tenant required in context")
	ErrInvalidInput        = errors.New("invalid input")
	ErrDuplicatePAN        = errors.New("duplicate PAN")
	ErrBatchPartialFailure = errors.New("batch partial failure")
	ErrIdempotencyConflict = errors.New("idempotency conflict: same key in-progress")
)

type DomainError struct {
	Code    string
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}
