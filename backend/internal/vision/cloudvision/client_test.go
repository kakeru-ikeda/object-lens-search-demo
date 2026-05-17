package cloudvision

import (
	"context"
	"testing"

	"github.com/googleapis/gax-go/v2"
	visionpb "google.golang.org/genproto/googleapis/cloud/vision/v1"

	"object-lens-search-demo/backend/internal/model"
)

type stubAnnotator struct {
	req *visionpb.BatchAnnotateImagesRequest
}

func (s *stubAnnotator) BatchAnnotateImages(ctx context.Context, req *visionpb.BatchAnnotateImagesRequest, opts ...gax.CallOption) (*visionpb.BatchAnnotateImagesResponse, error) {
	s.req = req
	return &visionpb.BatchAnnotateImagesResponse{Responses: []*visionpb.AnnotateImageResponse{{
		TextAnnotations:  []*visionpb.EntityAnnotation{{Description: "Coca-Cola", Score: 0.99}},
		LogoAnnotations:  []*visionpb.EntityAnnotation{{Description: "Coca-Cola", Score: 0.98}},
		LabelAnnotations: []*visionpb.EntityAnnotation{{Description: "Beverage", Score: 0.77}},
		WebDetection: &visionpb.WebDetection{
			WebEntities:             []*visionpb.WebDetection_WebEntity{{Description: "Cola", Score: 0.88}},
			BestGuessLabels:         []*visionpb.WebDetection_WebLabel{{Label: "coca cola can"}},
			FullMatchingImages:      []*visionpb.WebDetection_WebImage{{Url: "https://example.com/full.jpg", Score: 0.95}},
			PartialMatchingImages:   []*visionpb.WebDetection_WebImage{{Url: "https://example.com/partial.jpg", Score: 0.81}},
			VisuallySimilarImages:   []*visionpb.WebDetection_WebImage{{Url: "https://example.com/similar.jpg", Score: 0.64}},
			PagesWithMatchingImages: []*visionpb.WebDetection_WebPage{{Url: "https://example.com/page", PageTitle: "Example page", Score: 0.7, FullMatchingImages: []*visionpb.WebDetection_WebImage{{Url: "https://example.com/full.jpg"}}}},
		},
	}}}, nil
}

func (s *stubAnnotator) Close() error {
	return nil
}

func TestExtractEvidenceRequestsVisionFeatures(t *testing.T) {
	annotator := &stubAnnotator{}
	client := &Client{Annotator: annotator, MaxResults: 5}
	resp, err := client.ExtractEvidence(context.Background(), testRequest("data:image/jpeg;base64,aW1hZ2U="))
	if err != nil {
		t.Fatalf("unexpected extract error: %v", err)
	}
	if annotator.req == nil || len(annotator.req.Requests) != 1 {
		t.Fatalf("expected one annotate request, got %#v", annotator.req)
	}
	features := annotator.req.Requests[0].Features
	want := []visionpb.Feature_Type{visionpb.Feature_WEB_DETECTION, visionpb.Feature_TEXT_DETECTION, visionpb.Feature_LOGO_DETECTION, visionpb.Feature_LABEL_DETECTION}
	if len(features) != len(want) {
		t.Fatalf("expected %d features, got %d", len(want), len(features))
	}
	for i, feature := range features {
		if feature.Type != want[i] {
			t.Fatalf("feature %d: expected %v, got %v", i, want[i], feature.Type)
		}
	}
	if len(resp.Evidence.OCR) != 1 || resp.Evidence.OCR[0].Text != "Coca-Cola" {
		t.Fatalf("unexpected OCR evidence: %#v", resp.Evidence.OCR)
	}
	if len(resp.Evidence.WebEntities) != 1 || resp.Evidence.WebEntities[0].Text != "Cola" {
		t.Fatalf("unexpected web evidence: %#v", resp.Evidence.WebEntities)
	}
	if len(resp.Evidence.BestGuessLabels) != 1 || resp.Evidence.BestGuessLabels[0] != "coca cola can" {
		t.Fatalf("unexpected best guess labels: %#v", resp.Evidence.BestGuessLabels)
	}
	if len(resp.Evidence.MatchingImageURLs) != 2 || resp.Evidence.MatchingImageURLs[0] != "https://example.com/full.jpg" || resp.Evidence.MatchingImageURLs[1] != "https://example.com/partial.jpg" {
		t.Fatalf("unexpected matching image URLs: %#v", resp.Evidence.MatchingImageURLs)
	}
	if len(resp.Evidence.RelatedImages) != 3 {
		t.Fatalf("expected classified related images, got %#v", resp.Evidence.RelatedImages)
	}
	if resp.Evidence.RelatedImages[0].MatchType != "full_match" || resp.Evidence.RelatedImages[0].PageURL != "https://example.com/page" || resp.Evidence.RelatedImages[0].PageTitle != "Example page" {
		t.Fatalf("unexpected full match metadata: %#v", resp.Evidence.RelatedImages[0])
	}
	if resp.Evidence.RelatedImages[1].MatchType != "partial_match" || resp.Evidence.RelatedImages[2].MatchType != "visually_similar" {
		t.Fatalf("unexpected related image ordering/types: %#v", resp.Evidence.RelatedImages)
	}
}

func testRequest(image string) model.ExtractEvidenceRequest {
	return model.ExtractEvidenceRequest{ImageDataURL: image}
}
