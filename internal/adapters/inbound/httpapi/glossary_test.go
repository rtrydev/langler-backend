package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

func newGlossaryHandler(t *testing.T, provider *fakeGlossaryProvider) *httpapi.Handler {
	t.Helper()
	handler, err := httpapi.NewHandler(
		fakeStatusProvider{}, &fakeReferenceProvider{}, &fakeLessonImporter{}, &fakeLessonLibrary{},
		&fakeLessonPromptBuilder{}, &fakeLessonTopicAdvisor{}, &fakeLessonResultRecorder{}, &fakeProgressProvider{}, provider, &fakeAgentTokenManager{}, &fakeAssessmentProvider{},
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func TestHandleGlossaryReturnsWordsPerLanguage(t *testing.T) {
	t.Parallel()

	provider := &fakeGlossaryProvider{result: inbound.GlossaryResult{Languages: []inbound.GlossaryLanguage{
		{Language: "ja", Words: []inbound.GlossaryWord{{
			ID:          "N4#1416220",
			Headword:    "週末",
			Reading:     "しゅうまつ",
			Gloss:       []string{"weekend"},
			Level:       "N4",
			LessonCount: 2,
			AddedAt:     time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		}}},
	}}}
	handler := newGlossaryHandler(t, provider)

	request := lessonRequest(http.MethodGet, "/glossary", "user-1", "")
	request.QueryStringParameters = map[string]string{"language": "ja"}
	response, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body %s", response.StatusCode, response.Body)
	}
	if provider.query.Owner != "user-1" || provider.query.Language != "ja" {
		t.Fatalf("query = %+v", provider.query)
	}
	var body struct {
		Languages []struct {
			Language string `json:"language"`
			Words    []struct {
				ItemID      string   `json:"itemId"`
				Headword    string   `json:"headword"`
				Reading     string   `json:"reading"`
				Gloss       []string `json:"gloss"`
				Level       string   `json:"level"`
				LessonCount int      `json:"lessonCount"`
				AddedAt     string   `json:"addedAt"`
			} `json:"words"`
		} `json:"languages"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Languages) != 1 || body.Languages[0].Language != "ja" {
		t.Fatalf("languages = %+v", body.Languages)
	}
	word := body.Languages[0].Words[0]
	if word.ItemID != "N4#1416220" || word.Headword != "週末" || word.Gloss[0] != "weekend" || word.LessonCount != 2 {
		t.Errorf("word = %+v", word)
	}
	if word.AddedAt != "2026-07-01T12:00:00Z" {
		t.Errorf("addedAt = %q", word.AddedAt)
	}
}

func TestHandleGlossaryRequiresAuthenticatedUser(t *testing.T) {
	t.Parallel()

	handler := newGlossaryHandler(t, &fakeGlossaryProvider{})
	response, err := handler.Handle(context.Background(), getRequest("/glossary", nil))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.StatusCode)
	}
}
