package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
)

func newProgressHandler(t *testing.T, provider *fakeProgressProvider) *httpapi.Handler {
	t.Helper()
	handler, err := httpapi.NewHandler(
		fakeStatusProvider{}, &fakeReferenceProvider{}, &fakeLessonImporter{}, &fakeLessonLibrary{},
		&fakeLessonPromptBuilder{}, &fakeLessonResultRecorder{}, provider, &fakeAgentTokenManager{},
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func TestProgressRoutesUseTheLearnersCalendarDate(t *testing.T) {
	t.Parallel()

	provider := &fakeProgressProvider{}
	handler := newProgressHandler(t, provider)

	due := lessonRequest(http.MethodGet, "/reviews/due", "user-1", "")
	due.QueryStringParameters = map[string]string{"language": "ja", "date": "2026-07-19"}
	response, _ := handler.Handle(context.Background(), due)
	if response.StatusCode != http.StatusOK || provider.dueQuery.DueOn.Format(time.DateOnly) != "2026-07-19" {
		t.Fatalf("due response = %d, query = %+v", response.StatusCode, provider.dueQuery)
	}

	summary := lessonRequest(http.MethodGet, "/progress", "user-1", "")
	summary.QueryStringParameters = map[string]string{"date": "2026-07-19"}
	response, _ = handler.Handle(context.Background(), summary)
	if response.StatusCode != http.StatusOK || provider.summaryQuery.DueOn.Format(time.DateOnly) != "2026-07-19" {
		t.Fatalf("summary response = %d, query = %+v", response.StatusCode, provider.summaryQuery)
	}
}

func TestReviewGradeUsesTheLearnersCalendarDate(t *testing.T) {
	t.Parallel()

	provider := &fakeProgressProvider{}
	handler := newProgressHandler(t, provider)
	body := `{"language":"ja","kind":"vocab","itemId":"N4#1416220","grade":"good","reviewedOn":"2026-07-19"}`
	response, _ := handler.Handle(context.Background(), lessonRequest(http.MethodPost, "/reviews/grade", "user-1", body))
	if response.StatusCode != http.StatusOK || provider.gradeCommand.ReviewedOn.Format(time.DateOnly) != "2026-07-19" {
		t.Fatalf("response = %d, command = %+v", response.StatusCode, provider.gradeCommand)
	}
}
