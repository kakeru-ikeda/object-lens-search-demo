package vision

import (
	"context"

	"object-lens-search-demo/backend/internal/model"
)

type EvidenceExtractor interface {
	ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error)
	Close() error
}
