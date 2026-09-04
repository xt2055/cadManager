package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/drawing"
	"cadguanliq/internal/http/middleware"
)

type characterizationDrawingRepository struct {
	createdDrawing drawing.Drawing
	createdPart    drawing.Part
	parts          []drawing.Part
	updatePartErr  error
	createCalled   bool
	updateCalled   bool
}

func (r *characterizationDrawingRepository) List(context.Context, drawing.ListFilter) (drawing.Page[drawing.Drawing], error) {
	return drawing.Page[drawing.Drawing]{List: []drawing.Drawing{}, Page: 1, PageSize: 20}, nil
}

func (r *characterizationDrawingRepository) Find(context.Context, string) (drawing.Drawing, error) {
	return r.createdDrawing, nil
}

func (r *characterizationDrawingRepository) FindByNo(context.Context, string) (drawing.Drawing, error) {
	return r.createdDrawing, nil
}

func (r *characterizationDrawingRepository) Create(_ context.Context, input drawing.CreateDrawingInput, userID string) (drawing.Drawing, error) {
	r.createCalled = true
	r.createdDrawing = drawing.Drawing{ID: "drawing-1", No: input.No, Name: input.Name, Project: input.Project, CreatedByID: userID, Revision: 1}
	return r.createdDrawing, nil
}

func (r *characterizationDrawingRepository) Update(context.Context, string, drawing.UpdateDrawingInput, string) (drawing.Drawing, error) {
	return r.createdDrawing, nil
}

func (r *characterizationDrawingRepository) SetStatusByNo(context.Context, string, drawing.Status, drawing.Status, string) (drawing.Drawing, error) {
	return r.createdDrawing, nil
}

func (r *characterizationDrawingRepository) ListParts(context.Context, string) ([]drawing.Part, error) {
	return r.parts, nil
}

func (r *characterizationDrawingRepository) FindPart(context.Context, string) (drawing.Part, error) {
	return r.createdPart, nil
}

func (r *characterizationDrawingRepository) CreatePart(_ context.Context, drawingID string, input drawing.CreatePartInput, userID string) (drawing.Part, error) {
	r.createdPart = drawing.Part{ID: "part-1", DrawingID: drawingID, No: input.No, Name: input.Name, CreatedBy: userID, Revision: 1}
	return r.createdPart, nil
}

func (r *characterizationDrawingRepository) UpdatePart(_ context.Context, _ string, _ drawing.UpdatePartInput, _ string) (drawing.Part, error) {
	r.updateCalled = true
	if r.updatePartErr != nil {
		return drawing.Part{}, r.updatePartErr
	}
	return r.createdPart, nil
}

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	return request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{
		ID:          "user-1",
		Account:     "tester",
		DisplayName: "测试用户",
		Roles:       []string{"user"},
	}))
}

func responseEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}

func TestDrawingCharacterizationRequiresAuthentication(t *testing.T) {
	repository := &characterizationDrawingRepository{}
	request := httptest.NewRequest(http.MethodGet, "/api/drawings", nil)
	recorder := httptest.NewRecorder()

	Drawings(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestDrawingCharacterizationCreatesDrawing(t *testing.T) {
	repository := &characterizationDrawingRepository{}
	request := authenticatedRequest(http.MethodPost, "/api/drawings", `{"no":"JG001","name":"总图","project":"P001"}`)
	recorder := httptest.NewRecorder()

	Drawings(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusCreated)
	}
	if !repository.createCalled {
		t.Fatal("repository Create was not called")
	}
	envelope := responseEnvelope(t, recorder)
	if envelope["code"] != float64(0) {
		t.Fatalf("response code = %v, expected 0", envelope["code"])
	}
}

func TestDrawingCharacterizationPatchRequiresRevision(t *testing.T) {
	repository := &characterizationDrawingRepository{}
	request := authenticatedRequest(http.MethodPatch, "/api/drawings/drawing-1", `{"name":"新名称"}`)
	recorder := httptest.NewRecorder()

	DrawingResource(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusPreconditionRequired)
	}
}

func TestDrawingCharacterizationReadsStructure(t *testing.T) {
	repository := &characterizationDrawingRepository{parts: []drawing.Part{{ID: "part-1", DrawingID: "drawing-1", No: "P001", Name: "零件"}}}
	request := authenticatedRequest(http.MethodGet, "/api/drawings/drawing-1/structure", "")
	recorder := httptest.NewRecorder()

	DrawingResource(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusOK)
	}
	var envelope struct {
		Code int            `json:"code"`
		Data []drawing.Part `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 || len(envelope.Data) != 1 || envelope.Data[0].No != "P001" {
		t.Fatalf("unexpected structure response: %+v", envelope)
	}
}

func TestPartCharacterizationRevisionConflictReturns409(t *testing.T) {
	repository := &characterizationDrawingRepository{updatePartErr: drawing.ErrRevisionConflict}
	request := authenticatedRequest(http.MethodPatch, "/api/parts/part-1", `{"expectedRevision":1,"name":"新名称"}`)
	recorder := httptest.NewRecorder()

	PartResource(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusConflict)
	}
	if !repository.updateCalled {
		t.Fatal("repository UpdatePart was not called")
	}
}

func TestDrawingCharacterizationRepositoryErrorIsNotSilentlySuccessful(t *testing.T) {
	repository := &characterizationDrawingRepository{updatePartErr: errors.New("database unavailable")}
	request := authenticatedRequest(http.MethodPatch, "/api/parts/part-1", `{"expectedRevision":1,"name":"新名称"}`)
	recorder := httptest.NewRecorder()

	PartResource(repository).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, expected %d", recorder.Code, http.StatusInternalServerError)
	}
}
