package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cadguanliq/internal/review"
)

type characterizationReviewRepository struct {
	startedDrawingNo string
	startedUserID    string
	startResult      review.ReviewCase
	completed        []review.CompletedAction
}

func (r *characterizationReviewRepository) List(context.Context) ([]review.Flow, error) {
	return []review.Flow{}, nil
}

func (r *characterizationReviewRepository) Find(context.Context, string) (review.Flow, error) {
	return review.Flow{}, nil
}

func (r *characterizationReviewRepository) Create(context.Context, review.SaveFlowInput, string) (review.Flow, error) {
	return review.Flow{}, nil
}

func (r *characterizationReviewRepository) Update(context.Context, string, review.SaveFlowInput, string) (review.Flow, error) {
	return review.Flow{}, nil
}

func (r *characterizationReviewRepository) SetEnabled(context.Context, string, bool, string) (review.Flow, error) {
	return review.Flow{}, nil
}

func (r *characterizationReviewRepository) StartCase(_ context.Context, drawingNo, userID string) (review.ReviewCase, error) {
	r.startedDrawingNo = drawingNo
	r.startedUserID = userID
	return r.startResult, nil
}

func (r *characterizationReviewRepository) ListCases(context.Context) ([]review.ReviewCase, error) {
	return []review.ReviewCase{}, nil
}

func (r *characterizationReviewRepository) SubmitNode(context.Context, string, review.SubmitNodeInput, string) (review.ReviewCase, error) {
	return review.ReviewCase{}, nil
}

func (r *characterizationReviewRepository) CompletedActions(context.Context) ([]review.CompletedAction, error) {
	return r.completed, nil
}

func (r *characterizationReviewRepository) ActiveCaseAssigneeByDrawingNo(context.Context, string) (string, error) {
	return "", nil
}

func TestReviewCharacterizationRequiresDrawingNumber(t *testing.T) {
	repository := &characterizationReviewRepository{}
	request := authenticatedRequest(http.MethodPost, "/api/review-cases", `{"drawingNo":"  "}`)
	recorder := httptest.NewRecorder()

	ReviewCases(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestReviewCharacterizationStartsCaseWithTrimmedDrawingNumber(t *testing.T) {
	repository := &characterizationReviewRepository{startResult: review.ReviewCase{ID: "case-1", DrawingNo: "JG001"}}
	request := authenticatedRequest(http.MethodPost, "/api/review-cases", `{"drawingNo":"  JG001  "}`)
	recorder := httptest.NewRecorder()

	ReviewCases(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusCreated)
	}
	if repository.startedDrawingNo != "JG001" || repository.startedUserID != "user-1" {
		t.Fatalf("start args = drawing %q, user %q", repository.startedDrawingNo, repository.startedUserID)
	}
}

func TestReviewCharacterizationRejectsMalformedSubmit(t *testing.T) {
	repository := &characterizationReviewRepository{}
	request := authenticatedRequest(http.MethodPost, "/api/review-cases/case-1/submit", `{"action":"pass"}`)
	recorder := httptest.NewRecorder()

	ReviewCaseResource(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestReviewCharacterizationCompletedActions(t *testing.T) {
	repository := &characterizationReviewRepository{completed: []review.CompletedAction{{ID: "action-1", DrawingNo: "JG001"}}}
	request := authenticatedRequest(http.MethodGet, "/api/review-cases/completed", "")
	recorder := httptest.NewRecorder()

	ReviewCaseResource(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusOK)
	}
}
