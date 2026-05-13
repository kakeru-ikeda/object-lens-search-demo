package cloudvision

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	visionapi "cloud.google.com/go/vision/apiv1"
	"github.com/googleapis/gax-go/v2"
	visionpb "google.golang.org/genproto/googleapis/cloud/vision/v1"

	"object-lens-search-demo/backend/internal/model"
)

const defaultMaxResults = 10

type Annotator interface {
	BatchAnnotateImages(ctx context.Context, req *visionpb.BatchAnnotateImagesRequest, opts ...gax.CallOption) (*visionpb.BatchAnnotateImagesResponse, error)
	Close() error
}

type Client struct {
	Annotator  Annotator
	MaxResults int32
}

func New(ctx context.Context) (*Client, error) {
	annotator, err := visionapi.NewImageAnnotatorClient(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{Annotator: annotator, MaxResults: defaultMaxResults}, nil
}

func (c *Client) ExtractEvidence(ctx context.Context, req model.ExtractEvidenceRequest) (*model.ExtractEvidenceResponse, error) {
	if c.Annotator == nil {
		return nil, errors.New("cloud vision annotator is required")
	}
	imageDataURL := primaryImage(req)
	content, err := dataURLContent(imageDataURL)
	if err != nil {
		return nil, err
	}
	maxResults := c.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	resp, err := c.Annotator.BatchAnnotateImages(ctx, &visionpb.BatchAnnotateImagesRequest{Requests: []*visionpb.AnnotateImageRequest{{
		Image: &visionpb.Image{Content: content},
		Features: []*visionpb.Feature{
			{Type: visionpb.Feature_WEB_DETECTION, MaxResults: maxResults},
			{Type: visionpb.Feature_TEXT_DETECTION, MaxResults: maxResults},
			{Type: visionpb.Feature_LOGO_DETECTION, MaxResults: maxResults},
			{Type: visionpb.Feature_LABEL_DETECTION, MaxResults: maxResults},
		},
	}}})
	if err != nil {
		return nil, fmt.Errorf("annotate image: %w", err)
	}
	if len(resp.Responses) == 0 {
		return &model.ExtractEvidenceResponse{Provider: "cloud-vision"}, nil
	}
	annotation := resp.Responses[0]
	if annotation.Error != nil && annotation.Error.Message != "" {
		return nil, errors.New(annotation.Error.Message)
	}
	return &model.ExtractEvidenceResponse{Evidence: normalize(annotation), Provider: "cloud-vision"}, nil
}

func (c *Client) Close() error {
	if c.Annotator == nil {
		return nil
	}
	return c.Annotator.Close()
}

func primaryImage(req model.ExtractEvidenceRequest) string {
	if req.Crops != nil {
		if strings.TrimSpace(req.Crops.TextEnhancedCrop) != "" {
			return req.Crops.TextEnhancedCrop
		}
		if strings.TrimSpace(req.Crops.TightCrop) != "" {
			return req.Crops.TightCrop
		}
	}
	return req.ImageDataURL
}

func dataURLContent(value string) ([]byte, error) {
	_, payload, ok := strings.Cut(value, ";base64,")
	if !ok {
		return nil, errors.New("invalid image data URL")
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode image data URL: %w", err)
	}
	return decoded, nil
}

func normalize(resp *visionpb.AnnotateImageResponse) model.VisualEvidence {
	evidence := model.VisualEvidence{}
	seenOCR := map[string]bool{}
	for _, text := range resp.TextAnnotations {
		item := evidenceItem(text.Description, text.Score)
		if item.Text == "" || seenOCR[item.Text] {
			continue
		}
		seenOCR[item.Text] = true
		evidence.OCR = append(evidence.OCR, item)
		if len(evidence.OCR) >= 5 {
			break
		}
	}
	for _, logo := range resp.LogoAnnotations {
		if item := evidenceItem(logo.Description, logo.Score); item.Text != "" {
			evidence.Logos = append(evidence.Logos, item)
		}
	}
	for _, label := range resp.LabelAnnotations {
		if item := evidenceItem(label.Description, label.Score); item.Text != "" {
			evidence.Labels = append(evidence.Labels, item)
		}
	}
	if web := resp.WebDetection; web != nil {
		for _, entity := range web.WebEntities {
			if item := evidenceItem(entity.Description, entity.Score); item.Text != "" {
				evidence.WebEntities = append(evidence.WebEntities, item)
			}
		}
		for _, label := range web.BestGuessLabels {
			value := strings.TrimSpace(label.Label)
			if value != "" {
				evidence.BestGuessLabels = append(evidence.BestGuessLabels, value)
			}
		}
		for _, image := range web.FullMatchingImages {
			if url := strings.TrimSpace(image.Url); url != "" {
				evidence.MatchingImageURLs = append(evidence.MatchingImageURLs, url)
			}
		}
		for _, image := range web.PartialMatchingImages {
			if url := strings.TrimSpace(image.Url); url != "" {
				evidence.MatchingImageURLs = append(evidence.MatchingImageURLs, url)
			}
		}
	}
	trimEvidence(&evidence)
	return evidence
}

func evidenceItem(text string, score float32) model.EvidenceItem {
	return model.EvidenceItem{Text: strings.TrimSpace(text), Score: float64(score)}
}

func trimEvidence(e *model.VisualEvidence) {
	e.OCR = topItems(e.OCR, 5)
	e.Logos = topItems(e.Logos, 5)
	e.WebEntities = topItems(e.WebEntities, 10)
	e.Labels = topItems(e.Labels, 10)
	e.BestGuessLabels = uniqueStrings(e.BestGuessLabels, 5)
	e.MatchingImageURLs = uniqueStrings(e.MatchingImageURLs, 5)
}

func topItems(items []model.EvidenceItem, limit int) []model.EvidenceItem {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func uniqueStrings(values []string, limit int) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
		if len(out) >= limit {
			break
		}
	}
	return out
}
