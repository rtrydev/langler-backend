package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

func lessonRequest(method, path, owner, body string) events.APIGatewayV2HTTPRequest {
	req := events.APIGatewayV2HTTPRequest{RawPath: path, Body: body, Headers: map[string]string{"Idempotency-Key": "lesson-test-key"}}
	req.RequestContext.HTTP.Method = method
	if owner != "" {
		req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
				Claims: map[string]string{"sub": owner},
			},
		}
	}
	return req
}

func newLessonHandler(
	t *testing.T,
	importer *fakeLessonImporter,
	library *fakeLessonLibrary,
	prompts *fakeLessonPromptBuilder,
) *httpapi.Handler {
	t.Helper()
	h, err := httpapi.NewHandler(fakeStatusProvider{}, &fakeReferenceProvider{}, importer, library, prompts, &fakeLessonTopicAdvisor{}, &fakeLessonResultRecorder{}, &fakeProgressProvider{}, &fakeGlossaryProvider{}, &fakeAgentTokenManager{}, &fakeAssessmentProvider{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

const validLessonBody = `{
	"schemaVersion": "1.0",
	"lessonId": "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
	"language": "ja",
	"level": "N4",
	"title": "Weekend plans in Kyoto",
	"readingStage": "connected",
	"exercises": [
		{
			"exerciseId": "ex-1",
			"type": "cloze",
			"points": 8,
			"referencedVocab": ["N4#1416220"],
			"payload": {
				"text": "先週の{{1}}に行きました。",
				"blanks": [{"index": 1, "answer": "週末"}]
			}
		}
	]
}`

func TestHandleLessonImport(t *testing.T) {
	t.Parallel()

	t.Run("decodes document and returns created", func(t *testing.T) {
		t.Parallel()

		importer := &fakeLessonImporter{result: inbound.LessonImportResult{
			Created: true,
			Stored: inbound.StoredLesson{
				CreatedAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
				Lesson: lesson.Lesson{
					ID:       "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
					Title:    "Weekend plans in Kyoto",
					Language: "ja",
					Level:    "N4",
					Exercises: []lesson.Exercise{
						{ID: "ex-1", Type: lesson.TypeCloze, Points: 8, ReferencedVocab: []string{"N4#1416220"}},
					},
				},
			},
		}}
		h := newLessonHandler(t, importer, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, err := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "user-1", validLessonBody))
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
		}
		if importer.command.Owner != "user-1" {
			t.Errorf("Owner = %q", importer.command.Owner)
		}
		if importer.command.ContentHash == "" {
			t.Error("ContentHash is empty")
		}
		if importer.command.IdempotencyKey != "lesson-test-key" {
			t.Errorf("IdempotencyKey = %q", importer.command.IdempotencyKey)
		}
		if importer.command.Lesson.Exercises[0].Cloze == nil {
			t.Error("cloze payload was not decoded")
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["created"] != true || body["exerciseCount"].(float64) != 1 {
			t.Errorf("body = %v", body)
		}
	})

	t.Run("maps validation error to issue list", func(t *testing.T) {
		t.Parallel()

		importer := &fakeLessonImporter{err: &lesson.ValidationError{Issues: []lesson.Issue{
			{Path: "exercises[0].payload.blanks[0].answer", Message: "must not be empty"},
		}}}
		h := newLessonHandler(t, importer, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "user-1", validLessonBody))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
		var body struct {
			Error  string `json:"error"`
			Issues []struct {
				Path    string `json:"path"`
				Message string `json:"message"`
			} `json:"issues"`
		}
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.Issues) != 1 || body.Issues[0].Path != "exercises[0].payload.blanks[0].answer" {
			t.Errorf("issues = %v", body.Issues)
		}
	})

	t.Run("maps conflicting idempotency replay to conflict", func(t *testing.T) {
		t.Parallel()

		importer := &fakeLessonImporter{err: lesson.ErrIdempotencyConflict}
		h := newLessonHandler(t, importer, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, err := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "user-1", validLessonBody))
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if resp.StatusCode != http.StatusConflict || !strings.Contains(resp.Body, "idempotency key") {
			t.Fatalf("response = %+v", resp)
		}
	})

	t.Run("rejects malformed json with a readable issue", func(t *testing.T) {
		t.Parallel()

		h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "user-1", `{"broken":`))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
		if !strings.Contains(resp.Body, "issues") {
			t.Errorf("body = %s, want issue list", resp.Body)
		}
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		t.Parallel()

		h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		body := strings.Replace(validLessonBody, `"schemaVersion"`, `"surprise": 1, "schemaVersion"`, 1)
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "user-1", body))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
		if !strings.Contains(resp.Body, "surprise") {
			t.Errorf("body = %s, want mention of unknown field", resp.Body)
		}
	})

	t.Run("rejects oversized body", func(t *testing.T) {
		t.Parallel()

		h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "user-1", strings.Repeat("x", 300*1024)))
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})

	t.Run("requires authenticated user", func(t *testing.T) {
		t.Parallel()

		h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/import", "", validLessonBody))
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})
}

func TestHandleLessonPrompt(t *testing.T) {
	t.Parallel()

	prompts := &fakeLessonPromptBuilder{result: inbound.LessonPrompt{Prompt: "generated"}}
	h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, prompts)
	body := `{"language":"ja","level":"n4","topic":"Travel","exerciseTypes":["cloze"],"includeReference":false}`
	resp, err := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/prompt", "user-1", body))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
	}
	if prompts.query.Language != "ja" || prompts.query.IncludeReference {
		t.Errorf("query = %+v", prompts.query)
	}
	if !strings.Contains(resp.Body, "generated") {
		t.Errorf("body = %s", resp.Body)
	}
}

func TestHandleLessonList(t *testing.T) {
	t.Parallel()

	library := &fakeLessonLibrary{list: inbound.LessonListResult{
		Lessons: []inbound.StoredLesson{
			{
				CreatedAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
				Lesson: lesson.Lesson{
					ID:           "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
					Language:     "ja",
					Level:        "N4",
					Title:        "Weekend plans in Kyoto",
					ReadingStage: lesson.StageConnected,
					Exercises: []lesson.Exercise{
						{ID: "ex-1", Type: lesson.TypeReading, Points: 12, Reading: &lesson.Reading{Genre: lesson.GenreShortStory}},
					},
				},
			},
		},
		NextCursor: "cursor-1",
	}}
	h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
	resp, err := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons", "user-1", ""))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
	}
	var body struct {
		Items []struct {
			LessonID string `json:"lessonId"`
			HasStory bool   `json:"hasStory"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Items) != 1 || !body.Items[0].HasStory || body.NextCursor != "cursor-1" {
		t.Errorf("body = %+v", body)
	}
	if library.listQuery.Owner != "user-1" {
		t.Errorf("Owner = %q", library.listQuery.Owner)
	}
}

func TestHandleLessonGetAndDelete(t *testing.T) {
	t.Parallel()

	t.Run("get returns full document", func(t *testing.T) {
		t.Parallel()

		library := &fakeLessonLibrary{stored: inbound.StoredLesson{
			CreatedAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
			Lesson: lesson.Lesson{
				SchemaVersion: lesson.SchemaVersion,
				ID:            "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
				Language:      "ja",
				Level:         "N4",
				Title:         "Weekend plans in Kyoto",
				ReadingStage:  lesson.StageConnected,
				Exercises: []lesson.Exercise{
					{
						ID:    "ex-1",
						Type:  lesson.TypeCloze,
						Cloze: &lesson.Cloze{Text: "{{1}}です。", Blanks: []lesson.Blank{{Index: 1, Answer: "犬"}}},
					},
				},
			},
		}}
		h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons/3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00", "user-1", ""))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
		}
		if library.query.ID != "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00" {
			t.Errorf("ID = %q", library.query.ID)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		exercise := body["exercises"].([]any)[0].(map[string]any)
		payload := exercise["payload"].(map[string]any)
		if payload["text"] != "{{1}}です。" {
			t.Errorf("payload = %v", payload)
		}
	})

	t.Run("not found maps to 404", func(t *testing.T) {
		t.Parallel()

		library := &fakeLessonLibrary{err: lesson.ErrNotFound}
		h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons/3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00", "user-1", ""))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})

	t.Run("delete returns 204", func(t *testing.T) {
		t.Parallel()

		library := &fakeLessonLibrary{}
		h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodDelete, "/lessons/3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00", "user-1", ""))
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})
}

func TestHandleLessonResult(t *testing.T) {
	t.Parallel()

	completed := time.Date(2026, 7, 18, 12, 1, 0, 0, time.UTC)
	recorder := &fakeLessonResultRecorder{result: lesson.Result{
		AttemptID:   "11111111-1111-4111-8111-111111111111",
		LessonID:    "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		CompletedAt: completed,
		Score:       8,
		MaxScore:    8,
	}}
	h, err := httpapi.NewHandler(fakeStatusProvider{}, &fakeReferenceProvider{}, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{}, &fakeLessonTopicAdvisor{}, recorder, &fakeProgressProvider{}, &fakeGlossaryProvider{}, &fakeAgentTokenManager{}, &fakeAssessmentProvider{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	body := `{"attemptId":"11111111-1111-4111-8111-111111111111","startedAt":"2026-07-18T12:00:00Z","completedAt":"2026-07-18T12:01:00Z","completedOn":"2026-07-18","score":8,"maxScore":8,"autoScore":8,"autoMax":8,"selfScore":0,"selfMax":0,"exercises":[{"exerciseId":"ex-1","type":"cloze","grading":"auto","score":8,"maxScore":8,"correct":1,"total":1}]}`
	resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00/results", "user-1", body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
	}
	if recorder.command.Owner != "user-1" || recorder.command.Result.LessonID != "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00" {
		t.Fatalf("command = %+v", recorder.command)
	}
	if recorder.command.CompletedOn.Format(time.DateOnly) != "2026-07-18" {
		t.Fatalf("CompletedOn = %v", recorder.command.CompletedOn)
	}
}

func TestLessonTopicsReturnsSuggestions(t *testing.T) {
	t.Parallel()

	advisor := &fakeLessonTopicAdvisor{result: inbound.LessonTopicsResult{Topics: []inbound.LessonTopic{
		{Slug: "food-dining", Name: "Food & dining", Description: "Meals and cooking", WordCount: 41, CoveredCount: 12},
	}}}
	h, err := httpapi.NewHandler(fakeStatusProvider{}, &fakeReferenceProvider{}, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{}, advisor, &fakeLessonResultRecorder{}, &fakeProgressProvider{}, &fakeGlossaryProvider{}, &fakeAgentTokenManager{}, &fakeAssessmentProvider{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	req := lessonRequest(http.MethodGet, "/lessons/topics", "user-1", "")
	req.QueryStringParameters = map[string]string{"lang": "ja", "level": "N5"}
	resp, _ := h.Handle(context.Background(), req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	if advisor.query.Owner != "user-1" || advisor.query.Language != "ja" || advisor.query.Level != "N5" {
		t.Fatalf("query = %+v", advisor.query)
	}
	var body struct {
		Topics []struct {
			Slug         string `json:"slug"`
			Name         string `json:"name"`
			WordCount    int    `json:"wordCount"`
			CoveredCount int    `json:"coveredCount"`
		} `json:"topics"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Topics) != 1 || body.Topics[0].Slug != "food-dining" || body.Topics[0].WordCount != 41 || body.Topics[0].CoveredCount != 12 {
		t.Fatalf("body = %s", resp.Body)
	}
}

func TestLessonTopicsRequiresAuthenticatedUser(t *testing.T) {
	t.Parallel()

	h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
	resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons/topics", "", ""))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestLessonPromptForwardsOwnerAndTopicSlug(t *testing.T) {
	t.Parallel()

	prompts := &fakeLessonPromptBuilder{result: inbound.LessonPrompt{Prompt: "PROMPT"}}
	h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, prompts)
	body := `{"language":"ja","level":"N5","topic":"Food & dining","topicSlug":"food-dining","exerciseTypes":["cloze"]}`
	resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodPost, "/lessons/prompt", "user-1", body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	if prompts.query.Owner != "user-1" || prompts.query.TopicSlug != "food-dining" {
		t.Fatalf("query = %+v", prompts.query)
	}
}

func TestHandleLessonCompletions(t *testing.T) {
	t.Parallel()

	const id = "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00"

	t.Run("returns recent completions", func(t *testing.T) {
		t.Parallel()

		library := &fakeLessonLibrary{completions: inbound.LessonCompletionsResult{Completions: []inbound.LessonCompletion{
			{AttemptID: "a-2", CompletedAt: time.Date(2026, 7, 19, 9, 30, 0, 0, time.UTC), Score: 7, MaxScore: 8},
			{AttemptID: "a-1", CompletedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC), Score: 5, MaxScore: 8},
		}}}
		h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
		req := lessonRequest(http.MethodGet, "/lessons/"+id+"/results", "user-1", "")
		req.QueryStringParameters = map[string]string{"limit": "2"}
		resp, err := h.Handle(context.Background(), req)
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
		}
		if library.completionsQuery.Owner != "user-1" || library.completionsQuery.ID != id || library.completionsQuery.Limit != 2 {
			t.Errorf("query = %+v", library.completionsQuery)
		}
		var body struct {
			Items []struct {
				AttemptID   string `json:"attemptId"`
				CompletedAt string `json:"completedAt"`
				Score       int    `json:"score"`
				MaxScore    int    `json:"maxScore"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.Items) != 2 || body.Items[0].AttemptID != "a-2" || body.Items[0].CompletedAt != "2026-07-19T09:30:00Z" || body.Items[0].Score != 7 {
			t.Errorf("items = %+v", body.Items)
		}
	})

	t.Run("rejects invalid limit", func(t *testing.T) {
		t.Parallel()

		h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		req := lessonRequest(http.MethodGet, "/lessons/"+id+"/results", "user-1", "")
		req.QueryStringParameters = map[string]string{"limit": "nope"}
		resp, _ := h.Handle(context.Background(), req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})

	t.Run("maps missing lesson to 404", func(t *testing.T) {
		t.Parallel()

		library := &fakeLessonLibrary{err: lesson.ErrNotFound}
		h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons/"+id+"/results", "user-1", ""))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})

	t.Run("requires authenticated user", func(t *testing.T) {
		t.Parallel()

		h := newLessonHandler(t, &fakeLessonImporter{}, &fakeLessonLibrary{}, &fakeLessonPromptBuilder{})
		resp, _ := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons/"+id+"/results", "", ""))
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("StatusCode = %d", resp.StatusCode)
		}
	})
}

func TestHandleLessonListIncludesCompletionSummary(t *testing.T) {
	t.Parallel()

	const id = "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00"
	library := &fakeLessonLibrary{list: inbound.LessonListResult{
		Lessons: []inbound.StoredLesson{
			{
				CreatedAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
				Lesson: lesson.Lesson{
					ID:           id,
					Language:     "ja",
					Level:        "N4",
					Title:        "Weekend plans in Kyoto",
					ReadingStage: lesson.StageConnected,
				},
			},
		},
		Completions: map[string]inbound.LessonCompletionSummary{
			id: {Count: 3, LastCompletedAt: time.Date(2026, 7, 19, 9, 30, 0, 0, time.UTC), LastScore: 7, LastMaxScore: 8},
		},
	}}
	h := newLessonHandler(t, &fakeLessonImporter{}, library, &fakeLessonPromptBuilder{})
	resp, err := h.Handle(context.Background(), lessonRequest(http.MethodGet, "/lessons", "user-1", ""))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d body %s", resp.StatusCode, resp.Body)
	}
	var body struct {
		Items []struct {
			LessonID   string `json:"lessonId"`
			Completion *struct {
				Count           int    `json:"count"`
				LastCompletedAt string `json:"lastCompletedAt"`
				LastScore       int    `json:"lastScore"`
				LastMaxScore    int    `json:"lastMaxScore"`
			} `json:"completion"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Completion == nil {
		t.Fatalf("items = %+v", body.Items)
	}
	completion := body.Items[0].Completion
	if completion.Count != 3 || completion.LastCompletedAt != "2026-07-19T09:30:00Z" || completion.LastScore != 7 || completion.LastMaxScore != 8 {
		t.Errorf("completion = %+v", completion)
	}
}
