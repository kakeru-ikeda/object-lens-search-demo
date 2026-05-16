package mock

import (
	"context"

	"object-lens-search-demo/backend/internal/model"
)

type Client struct{}

func (c *Client) ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return &model.ExtractEvidenceResponse{
		Evidence: model.VisualEvidence{
			OCR:             []model.EvidenceItem{{Text: "sample text", Score: 0.99}},
			Logos:           []model.EvidenceItem{{Text: "sample logo", Score: 0.98}},
			WebEntities:     []model.EvidenceItem{{Text: "sample object", Score: 0.87}},
			BestGuessLabels: []string{"sample object"},
			Labels:          []model.EvidenceItem{{Text: "product", Score: 0.76}},
		},
		Provider: "cloud-vision-mock",
	}, nil
}

func (c *Client) Close() error {
	return nil
}
