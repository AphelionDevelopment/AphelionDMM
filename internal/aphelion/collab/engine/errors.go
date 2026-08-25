package engine

import (
	"errors"
	"fmt"

	"sdmm/internal/aphelion/collab/model"
)

type Code string

const (
	CodeInvalidOperation    Code = "invalid_operation"
	CodeWrongDocument       Code = "wrong_document"
	CodeWrongEnvironment    Code = "wrong_environment"
	CodeUnknownBaseRevision Code = "unknown_base_revision"
	CodeBaseHashMismatch    Code = "base_hash_mismatch"
	CodeOutOfBounds         Code = "out_of_bounds"
	CodePreconditionFailed  Code = "precondition_failed"
	CodeOperationNotFound   Code = "operation_not_found"
	CodeActorMismatch       Code = "actor_mismatch"
	CodeAlreadyInverted     Code = "already_inverted"
)

type Rejection struct {
	Code            Code
	CurrentRevision model.Revision
	CurrentMapHash  string
	Cause           error
}

func (rejection *Rejection) Error() string {
	return fmt.Sprintf("operation rejected (%s) at revision %d: %v", rejection.Code, rejection.CurrentRevision, rejection.Cause)
}

func (rejection *Rejection) Unwrap() error {
	return rejection.Cause
}

func CodeOf(err error) Code {
	var rejection *Rejection
	if errors.As(err, &rejection) {
		return rejection.Code
	}
	return ""
}
