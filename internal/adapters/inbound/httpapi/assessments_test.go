package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/domain/assessment"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type fakeAssessmentProvider struct {
	startCommand  inbound.AssessmentStartCommand
	answerCommand inbound.AssessmentAnswerCommand
	view          inbound.AssessmentView
	summaries     []inbound.AssessmentSummary
	levels        []inbound.ProfileLevel
	err           error
}

func (f *fakeAssessmentProvider) Start(_ context.Context, command inbound.AssessmentStartCommand) (inbound.AssessmentView, error) {
	f.startCommand = command
	return f.view, f.err
}

func (f *fakeAssessmentProvider) Answer(_ context.Context, command inbound.AssessmentAnswerCommand) (inbound.AssessmentView, error) {
	f.answerCommand = command
	return f.view, f.err
}

func (f *fakeAssessmentProvider) Assessment(_ context.Context, _, _ string) (inbound.AssessmentView, error) {
	return f.view, f.err
}

func (f *fakeAssessmentProvider) Assessments(_ context.Context, _ string) ([]inbound.AssessmentSummary, error) {
	return f.summaries, f.err
}

func (f *fakeAssessmentProvider) Levels(_ context.Context, _ string) ([]inbound.ProfileLevel, error) {
	return f.levels, f.err
}

func newAssessmentHandler(t *testing.T, provider *fakeAssessmentProvider) *httpapi.Handler {
	t.Helper()
	handler, err := httpapi.NewHandler(
		fakeStatusProvider{}, &fakeReferenceProvider{}, &fakeLessonImporter{}, &fakeLessonLibrary{},
		&fakeLessonPromptBuilder{}, &fakeLessonResultRecorder{}, &fakeProgressProvider{}, &fakeAgentTokenManager{}, provider,
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func inProgressView() inbound.AssessmentView {
	return inbound.AssessmentView{
		ID:        "a-1",
		Language:  "ja",
		Status:    "in_progress",
		StartedAt: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
		Stage: &inbound.AssessmentStageView{
			Index:     0,
			Band:      "N5",
			BandCount: 5,
			Items: []inbound.AssessmentItemView{
				{Kind: "vocab", Prompt: "犬", Options: []string{"dog", "cat", "bird", "fish"}},
			},
		},
	}
}

func TestAssessmentStartReturnsStageWithoutAnswers(t *testing.T) {
	t.Parallel()

	provider := &fakeAssessmentProvider{view: inProgressView()}
	handler := newAssessmentHandler(t, provider)

	request := lessonRequest(http.MethodPost, "/assessments", "user-1", `{"language":"ja"}`)
	response, _ := handler.Handle(context.Background(), request)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.StatusCode, response.Body)
	}
	if provider.startCommand.Owner != "user-1" || provider.startCommand.Language != "ja" {
		t.Fatalf("command = %+v", provider.startCommand)
	}
	if strings.Contains(response.Body, "correctIndex") || strings.Contains(response.Body, "CorrectIndex") {
		t.Fatalf("body leaks correct answers: %s", response.Body)
	}
	var body struct {
		AssessmentID string `json:"assessmentId"`
		Guidance     string `json:"guidance"`
		Stage        *struct {
			Band  string `json:"band"`
			Items []struct {
				Options []string `json:"options"`
			} `json:"items"`
		} `json:"stage"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.AssessmentID != "a-1" || body.Guidance == "" || body.Stage == nil || body.Stage.Band != "N5" || len(body.Stage.Items[0].Options) != 4 {
		t.Fatalf("body = %+v", body)
	}
}

func TestAssessmentAnswerRoutesStageSubmission(t *testing.T) {
	t.Parallel()

	provider := &fakeAssessmentProvider{view: inProgressView()}
	handler := newAssessmentHandler(t, provider)

	request := lessonRequest(http.MethodPost, "/assessments/a-1/answers", "user-1", `{"stageIndex":0,"answers":[0,1,2]}`)
	response, _ := handler.Handle(context.Background(), request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.StatusCode, response.Body)
	}
	command := provider.answerCommand
	if command.AssessmentID != "a-1" || command.StageIndex != 0 || len(command.Answers) != 3 {
		t.Fatalf("command = %+v", command)
	}

	missingStage := lessonRequest(http.MethodPost, "/assessments/a-1/answers", "user-1", `{"answers":[0]}`)
	response, _ = handler.Handle(context.Background(), missingStage)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing stageIndex status = %d", response.StatusCode)
	}
}

func TestAssessmentErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"invalid answer", assessment.ErrInvalidAnswer, http.StatusBadRequest},
		{"insufficient reference", assessment.ErrInsufficientReference, http.StatusBadRequest},
		{"completed", assessment.ErrAlreadyCompleted, http.StatusConflict},
		{"conflict", assessment.ErrConflict, http.StatusConflict},
		{"not found", assessment.ErrNotFound, http.StatusNotFound},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			handler := newAssessmentHandler(t, &fakeAssessmentProvider{err: testCase.err})
			request := lessonRequest(http.MethodPost, "/assessments/a-1/answers", "user-1", `{"stageIndex":0,"answers":[0]}`)
			response, _ := handler.Handle(context.Background(), request)
			if response.StatusCode != testCase.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, testCase.status)
			}
		})
	}
}

func TestAssessmentRoutesRequireAuthentication(t *testing.T) {
	t.Parallel()

	handler := newAssessmentHandler(t, &fakeAssessmentProvider{})
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/assessments"},
		{http.MethodGet, "/assessments"},
		{http.MethodGet, "/assessments/a-1"},
		{http.MethodPost, "/assessments/a-1/answers"},
		{http.MethodGet, "/profile/levels"},
	}
	for _, route := range paths {
		request := lessonRequest(route.method, route.path, "", `{"language":"ja"}`)
		response, _ := handler.Handle(context.Background(), request)
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", route.method, route.path, response.StatusCode)
		}
	}
}

func TestAssessmentListAndLevels(t *testing.T) {
	t.Parallel()

	provider := &fakeAssessmentProvider{
		summaries: []inbound.AssessmentSummary{{
			ID: "a-2", Language: "ja", Status: "completed", EstimatedLevel: "N3", Confidence: "high",
			StartedAt:   time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 7, 19, 10, 12, 0, 0, time.UTC),
		}},
		levels: []inbound.ProfileLevel{{
			Language: "ja", Level: "N3", AssessmentID: "a-2",
			UpdatedAt: time.Date(2026, 7, 19, 10, 12, 0, 0, time.UTC),
		}},
	}
	handler := newAssessmentHandler(t, provider)

	response, _ := handler.Handle(context.Background(), lessonRequest(http.MethodGet, "/assessments", "user-1", ""))
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Body, `"estimatedLevel":"N3"`) {
		t.Fatalf("list = %d %s", response.StatusCode, response.Body)
	}

	response, _ = handler.Handle(context.Background(), lessonRequest(http.MethodGet, "/profile/levels", "user-1", ""))
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Body, `"level":"N3"`) {
		t.Fatalf("levels = %d %s", response.StatusCode, response.Body)
	}
}
