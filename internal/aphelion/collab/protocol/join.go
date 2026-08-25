package protocol

import "sdmm/internal/aphelion/collab/model"

type JoinRequest struct {
	BaseURL              string
	Origin               string
	Token                string
	SessionID            string
	AcknowledgedRevision model.Revision
}
