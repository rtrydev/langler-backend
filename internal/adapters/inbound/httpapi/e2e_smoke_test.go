package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/rtrydev/langler-backend/internal/adapters/inbound/httpapi"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoassessments"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoglossary"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamolessons"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoprogress"
	"github.com/rtrydev/langler-backend/internal/adapters/outbound/dynamoref"
	"github.com/rtrydev/langler-backend/internal/application/assessments"
	"github.com/rtrydev/langler-backend/internal/application/lessons"
	progressapp "github.com/rtrydev/langler-backend/internal/application/progress"
	appref "github.com/rtrydev/langler-backend/internal/application/reference"
	"github.com/rtrydev/langler-backend/internal/application/status"
	reference "github.com/rtrydev/langler-backend/internal/domain/reference"
)

type unconfiguredSemantic struct{}

func (unconfiguredSemantic) SimilarVocabIDs(context.Context, reference.Language, reference.Level, string, int) ([]string, error) {
	return nil, fmt.Errorf("semantic search disabled in e2e")
}

func TestE2EAgainstLoadedReferenceData(t *testing.T) {
	endpoint := os.Getenv("DYNAMODB_LOCAL_ENDPOINT")
	table := os.Getenv("E2E_TABLE")
	if endpoint == "" || table == "" {
		t.Skip("set DYNAMODB_LOCAL_ENDPOINT and E2E_TABLE to run")
	}

	client := dynamodb.New(dynamodb.Options{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
		BaseEndpoint: aws.String(endpoint),
	})
	repo, err := dynamoref.NewRepository(client, table)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	refSvc, err := appref.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	statusSvc, err := status.NewService("langler-backend", "local")
	if err != nil {
		t.Fatalf("status.NewService: %v", err)
	}
	lessonRepo, err := dynamolessons.NewRepository(client, table)
	if err != nil {
		t.Fatalf("dynamolessons.NewRepository: %v", err)
	}
	progressRepo, err := dynamoprogress.NewRepository(client, table)
	if err != nil {
		t.Fatalf("dynamoprogress.NewRepository: %v", err)
	}
	progressSvc, err := progressapp.NewService(progressRepo, repo)
	if err != nil {
		t.Fatalf("progress.NewService: %v", err)
	}
	glossaryRepo, err := dynamoglossary.NewRepository(client, table)
	if err != nil {
		t.Fatalf("dynamoglossary.NewRepository: %v", err)
	}
	lessonsSvc, err := lessons.NewService(lessonRepo, repo, repo, progressRepo, unconfiguredSemantic{}, lessonRepo, progressSvc, glossaryRepo)
	if err != nil {
		t.Fatalf("lessons.NewService: %v", err)
	}
	assessmentRepo, err := dynamoassessments.NewRepository(client, table)
	if err != nil {
		t.Fatalf("dynamoassessments.NewRepository: %v", err)
	}
	assessmentSvc, err := assessments.NewService(assessmentRepo, repo)
	if err != nil {
		t.Fatalf("assessments.NewService: %v", err)
	}
	h, err := httpapi.NewHandler(statusSvc, refSvc, lessonsSvc, lessonsSvc, lessonsSvc, lessonsSvc, lessonsSvc, progressSvc, lessonsSvc, &fakeAgentTokenManager{}, assessmentSvc)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	ctx := context.Background()

	call := func(path string, params map[string]string) map[string]any {
		t.Helper()
		resp, err := h.Handle(ctx, getRequest(path, params))
		if err != nil {
			t.Fatalf("Handle %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %v: status %d body %s", path, params, resp.StatusCode, resp.Body)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return body
	}

	for _, level := range []string{"N5", "N4", "N3", "N2", "N1"} {
		body := call("/reference/vocab", map[string]string{"lang": "ja", "level": level, "limit": "5"})
		items := body["items"].([]any)
		if len(items) != 5 {
			t.Fatalf("vocab %s: %d items, want 5", level, len(items))
		}
		first := items[0].(map[string]any)
		for _, field := range []string{"headword", "reading", "gloss", "level", "sourceId", "license"} {
			if first[field] == nil || first[field] == "" {
				t.Errorf("vocab %s: field %q missing in %v", level, field, first)
			}
		}
		t.Logf("vocab %s first: %v %v (%v)", level, first["headword"], first["reading"], first["gloss"])

		grammar := call("/reference/grammar", map[string]string{"lang": "ja", "level": level, "limit": "3"})
		gItems := grammar["items"].([]any)
		if len(gItems) != 3 {
			t.Fatalf("grammar %s: %d items, want 3", level, len(gItems))
		}
		g := gItems[0].(map[string]any)
		if g["topicId"] == nil || g["description"] == nil || g["example"] == nil {
			t.Errorf("grammar %s: incomplete topic %v", level, g)
		}
	}

	kana := call("/reference/scripts", map[string]string{"lang": "ja", "type": "kana", "limit": "200"})
	if n := len(kana["items"].([]any)); n != 200 {
		t.Fatalf("kana page: %d items, want 200", n)
	}
	firstKana := kana["items"].([]any)[0].(map[string]any)
	if firstKana["glyph"] != "あ" {
		t.Errorf("first kana = %v, want あ (gojūon order)", firstKana["glyph"])
	}

	for _, level := range []string{"N5", "N4", "N2", "N1"} {
		kanji := call("/reference/scripts", map[string]string{"lang": "ja", "type": "kanji", "level": level, "limit": "3"})
		items := kanji["items"].([]any)
		if len(items) != 3 {
			t.Fatalf("kanji %s: %d items, want 3", level, len(items))
		}
		k := items[0].(map[string]any)
		if k["strokeDataRef"] == nil || k["readings"] == nil || k["strokeCount"] == nil {
			t.Errorf("kanji %s: incomplete %v", level, k)
		}
		t.Logf("kanji %s first: %v strokes=%v ref=%v", level, k["glyph"], k["strokeCount"], k["strokeDataRef"])
	}

	var cursor string
	seen := map[string]bool{}
	pages := 0
	for {
		params := map[string]string{"lang": "ja", "level": "N5", "limit": "200"}
		if cursor != "" {
			params["cursor"] = cursor
		}
		body := call("/reference/vocab", params)
		for _, item := range body["items"].([]any) {
			hw := fmt.Sprint(item.(map[string]any)["headword"], item.(map[string]any)["reading"])
			seen[hw] = true
		}
		pages++
		next, _ := body["nextCursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	t.Logf("paged through %d pages, %d distinct N5 entries", pages, len(seen))
	if len(seen) < 700 {
		t.Errorf("N5 distinct entries = %d, want >= 700", len(seen))
	}

	for _, want := range []string{"学校がっこう", "犬いぬ", "食べるたべる"} {
		if !seen[want] {
			t.Errorf("core N5 word %s missing", want)
		}
	}

	importKey := fmt.Sprintf("e2e-import-key-%d", time.Now().UnixNano())
	importLessonID := fmt.Sprintf("7b6f7d3e-4a5b-4c6d-8e9f-%012x", time.Now().UnixNano()&0xffffffffffff)

	send := func(method, path, owner string, payload any) (int, map[string]any) {
		t.Helper()
		req := events.APIGatewayV2HTTPRequest{RawPath: path}
		if path == "/lessons/import" {
			req.Headers = map[string]string{"Idempotency-Key": importKey}
		}
		req.RequestContext.HTTP.Method = method
		if owner != "" {
			req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
				JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
					Claims: map[string]string{"sub": owner},
				},
			}
		}
		if payload != nil {
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			req.Body = string(body)
		}
		resp, err := h.Handle(ctx, req)
		if err != nil {
			t.Fatalf("Handle %s %s: %v", method, path, err)
		}
		var body map[string]any
		if resp.Body != "" {
			if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
				t.Fatalf("unmarshal %s %s: %v", method, path, err)
			}
		}
		return resp.StatusCode, body
	}

	statusCode, prompt := send(http.MethodPost, "/lessons/prompt", "e2e-user", map[string]any{
		"language":      "ja",
		"level":         "N5",
		"topic":         "a trip to the school",
		"exerciseTypes": []string{"cloze", "reading"},
	})
	if statusCode != http.StatusOK {
		t.Fatalf("prompt: status %d body %v", statusCode, prompt)
	}
	promptText, _ := prompt["prompt"].(string)
	if !strings.Contains(promptText, "short_story") || !strings.Contains(promptText, "N5#") {
		t.Fatalf("prompt missing story instructions or reference ids: %.400s", promptText)
	}
	if !strings.Contains(promptText, "candidate pool") {
		t.Fatalf("free-text topic prompt missing candidate-pool instruction")
	}

	topicsReq := events.APIGatewayV2HTTPRequest{
		RawPath:               "/lessons/topics",
		QueryStringParameters: map[string]string{"lang": "ja", "level": "N5"},
	}
	topicsReq.RequestContext.HTTP.Method = http.MethodGet
	topicsReq.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
		JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
			Claims: map[string]string{"sub": "e2e-user"},
		},
	}
	topicsResp, err := h.Handle(ctx, topicsReq)
	if err != nil {
		t.Fatalf("Handle GET /lessons/topics: %v", err)
	}
	if topicsResp.StatusCode != http.StatusOK {
		t.Fatalf("topics: status %d body %s", topicsResp.StatusCode, topicsResp.Body)
	}
	var topicList struct {
		Topics []struct {
			Slug         string `json:"slug"`
			Name         string `json:"name"`
			WordCount    int    `json:"wordCount"`
			CoveredCount int    `json:"coveredCount"`
		} `json:"topics"`
	}
	if err := json.Unmarshal([]byte(topicsResp.Body), &topicList); err != nil {
		t.Fatalf("unmarshal topics: %v", err)
	}
	if len(topicList.Topics) != 18 {
		t.Fatalf("topics = %d, want the full 18-slug taxonomy", len(topicList.Topics))
	}
	firstTopic := topicList.Topics[0]
	if firstTopic.WordCount <= 0 {
		t.Fatalf("topic %q has no words", firstTopic.Slug)
	}
	t.Logf("topics: %d for N5, first %q (%d/%d covered)", len(topicList.Topics), firstTopic.Slug, firstTopic.CoveredCount, firstTopic.WordCount)

	statusCode, topicPrompt := send(http.MethodPost, "/lessons/prompt", "e2e-user", map[string]any{
		"language":      "ja",
		"level":         "N5",
		"topic":         firstTopic.Name,
		"topicSlug":     firstTopic.Slug,
		"exerciseTypes": []string{"cloze", "reading"},
	})
	if statusCode != http.StatusOK {
		t.Fatalf("topic prompt: status %d body %v", statusCode, topicPrompt)
	}
	if text, _ := topicPrompt["prompt"].(string); !strings.Contains(text, "N5#") {
		t.Fatalf("topic prompt missing reference ids: %.400s", text)
	}

	statusCode, badTopic := send(http.MethodPost, "/lessons/prompt", "e2e-user", map[string]any{
		"language":      "ja",
		"level":         "N5",
		"topicSlug":     "space-travel",
		"exerciseTypes": []string{"cloze"},
	})
	if statusCode != http.StatusBadRequest {
		t.Fatalf("unknown topic slug: status %d body %v", statusCode, badTopic)
	}

	vocabBody := call("/reference/vocab", map[string]string{"lang": "ja", "level": "N5", "limit": "1"})
	vocabID := vocabBody["items"].([]any)[0].(map[string]any)["id"].(string)

	lessonDoc := map[string]any{
		"schemaVersion": "1.0",
		"lessonId":      importLessonID,
		"language":      "ja",
		"level":         "N5",
		"title":         "E2E smoke lesson",
		"readingStage":  "connected",
		"exercises": []map[string]any{
			{
				"exerciseId": "ex-1",
				"type":       "reading",
				"prompt":     "Read the story and answer.",
				"points":     6,
				"payload": map[string]any{
					"genre":   "short_story",
					"title":   "学校の一日",
					"passage": "今日は学校へ行きました。友達と勉強しました。",
					"questions": []map[string]any{
						{
							"question": "どこへ行きましたか。",
							"kind":     "multiple_choice",
							"options":  []string{"学校", "こうえん"},
							"answer":   "学校",
						},
					},
				},
			},
			{
				"exerciseId":      "ex-2",
				"type":            "cloze",
				"prompt":          "Fill in the blank.",
				"points":          4,
				"referencedVocab": []string{vocabID},
				"payload": map[string]any{
					"text":   "わたしは{{1}}へ行きます。",
					"blanks": []map[string]any{{"index": 1, "answer": "学校"}},
				},
			},
		},
	}

	statusCode, imported := send(http.MethodPost, "/lessons/import", "e2e-user", lessonDoc)
	if statusCode != http.StatusCreated {
		t.Fatalf("import: status %d body %v", statusCode, imported)
	}
	lessonID := imported["lessonId"].(string)
	t.Cleanup(func() {
		send(http.MethodDelete, "/lessons/"+lessonID, "e2e-user", nil)
	})
	if imported["created"] != true || imported["exerciseCount"].(float64) != 2 {
		t.Fatalf("import response = %v", imported)
	}

	statusCode, replay := send(http.MethodPost, "/lessons/import", "e2e-user", lessonDoc)
	if statusCode != http.StatusOK || replay["created"] != false {
		t.Fatalf("duplicate import: status %d body %v", statusCode, replay)
	}

	glossaryWords := func(owner string) []string {
		t.Helper()
		req := getRequest("/glossary", map[string]string{"language": "ja"})
		req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
				Claims: map[string]string{"sub": owner},
			},
		}
		resp, err := h.Handle(ctx, req)
		if err != nil {
			t.Fatalf("Handle GET /glossary: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("glossary: status %d body %s", resp.StatusCode, resp.Body)
		}
		var body struct {
			Languages []struct {
				Language string `json:"language"`
				Words    []struct {
					ItemID string `json:"itemId"`
				} `json:"words"`
			} `json:"languages"`
		}
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatalf("unmarshal glossary: %v", err)
		}
		var ids []string
		for _, language := range body.Languages {
			for _, word := range language.Words {
				ids = append(ids, word.ItemID)
			}
		}
		return ids
	}

	glossaryOwner := fmt.Sprintf("e2e-glossary-user-%d", time.Now().UnixNano())
	glossaryDoc := map[string]any{}
	for key, value := range lessonDoc {
		glossaryDoc[key] = value
	}
	glossaryDoc["lessonId"] = fmt.Sprintf("9d8e9f5a-6c7d-4e8f-a0b1-%012x", time.Now().UnixNano()&0xffffffffffff)
	statusCode, glossaryImported := send(http.MethodPost, "/lessons/import", glossaryOwner, glossaryDoc)
	if statusCode != http.StatusCreated {
		t.Fatalf("glossary import: status %d body %v", statusCode, glossaryImported)
	}
	glossaryLessonID := glossaryImported["lessonId"].(string)
	t.Cleanup(func() {
		send(http.MethodDelete, "/lessons/"+glossaryLessonID, glossaryOwner, nil)
	})
	if words := glossaryWords(glossaryOwner); !slices.Contains(words, vocabID) {
		t.Fatalf("glossary after import = %v, want %s", words, vocabID)
	}
	if statusCode, _ = send(http.MethodDelete, "/lessons/"+glossaryLessonID, glossaryOwner, nil); statusCode != http.StatusNoContent {
		t.Fatalf("glossary delete: status %d, want 204", statusCode)
	}
	if words := glossaryWords(glossaryOwner); len(words) != 0 {
		t.Fatalf("glossary after delete = %v, want empty", words)
	}

	badDoc := map[string]any{
		"schemaVersion": "1.0",
		"lessonId":      "8c7f8e4f-5b6c-4d7e-9fa0-1b2c3d4e5f60",
		"language":      "ja",
		"level":         "N5",
		"title":         "Missing story",
		"readingStage":  "connected",
		"exercises": []map[string]any{
			{
				"exerciseId":      "ex-1",
				"type":            "cloze",
				"referencedVocab": []string{"N5#does-not-exist"},
				"payload": map[string]any{
					"text":   "わたしは{{1}}へ行きます。",
					"blanks": []map[string]any{{"index": 1, "answer": "学校"}},
				},
			},
		},
	}
	statusCode, rejected := send(http.MethodPost, "/lessons/import", "e2e-user", badDoc)
	if statusCode != http.StatusBadRequest || rejected["issues"] == nil {
		t.Fatalf("invalid import: status %d body %v", statusCode, rejected)
	}

	statusCode, list := send(http.MethodGet, "/lessons", "e2e-user", nil)
	if statusCode != http.StatusOK || len(list["items"].([]any)) == 0 {
		t.Fatalf("list: status %d body %v", statusCode, list)
	}

	statusCode, detail := send(http.MethodGet, "/lessons/"+lessonID, "e2e-user", nil)
	if statusCode != http.StatusOK || detail["title"] != "E2E smoke lesson" {
		t.Fatalf("get: status %d body %v", statusCode, detail)
	}

	if statusCode, _ = send(http.MethodGet, "/lessons/"+lessonID, "another-user", nil); statusCode != http.StatusNotFound {
		t.Fatalf("cross-user get: status %d, want 404", statusCode)
	}

	if statusCode, _ = send(http.MethodDelete, "/lessons/"+lessonID, "e2e-user", nil); statusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", statusCode)
	}
	if statusCode, _ = send(http.MethodGet, "/lessons/"+lessonID, "e2e-user", nil); statusCode != http.StatusNotFound {
		t.Fatalf("get after delete: status %d, want 404", statusCode)
	}

	answerStage := func(owner, assessmentID string, view map[string]any, correctly bool) map[string]any {
		t.Helper()
		stage := view["stage"].(map[string]any)
		stageIndex := int(stage["index"].(float64))
		items := stage["items"].([]any)
		stored, err := assessmentRepo.Get(ctx, owner, assessmentID)
		if err != nil {
			t.Fatalf("assessment get: %v", err)
		}
		answers := make([]int, len(items))
		for i := range items {
			correct := stored.Stages[stageIndex].Items[i].CorrectIndex
			if correctly {
				answers[i] = correct
			} else {
				answers[i] = (correct + 1) % len(stored.Stages[stageIndex].Items[i].Options)
			}
		}
		statusCode, next := send(http.MethodPost, "/assessments/"+assessmentID+"/answers", owner, map[string]any{
			"stageIndex": stageIndex,
			"answers":    answers,
		})
		if statusCode != http.StatusOK {
			t.Fatalf("answer stage %d: status %d body %v", stageIndex, statusCode, next)
		}
		return next
	}

	assessOwner := fmt.Sprintf("e2e-assess-user-%d", time.Now().UnixNano())
	statusCode, started := send(http.MethodPost, "/assessments", assessOwner, map[string]any{"language": "ja"})
	if statusCode != http.StatusCreated {
		t.Fatalf("assessment start: status %d body %v", statusCode, started)
	}
	if strings.Contains(fmt.Sprint(started), "correctIndex") {
		t.Fatal("assessment view leaks correct answers")
	}
	assessmentID := started["assessmentId"].(string)
	view := started
	for view["status"] == "in_progress" {
		view = answerStage(assessOwner, assessmentID, view, true)
	}
	result := view["result"].(map[string]any)
	if result["estimatedLevel"] != "N1" {
		t.Fatalf("all-correct estimate = %v, want N1", result["estimatedLevel"])
	}

	statusCode, floorStarted := send(http.MethodPost, "/assessments", assessOwner, map[string]any{"language": "ja"})
	if statusCode != http.StatusCreated {
		t.Fatalf("second assessment start: status %d", statusCode)
	}
	floorView := answerStage(assessOwner, floorStarted["assessmentId"].(string), floorStarted, false)
	if floorView["status"] != "completed" {
		t.Fatalf("all-wrong session status = %v, want completed", floorView["status"])
	}
	floorResult := floorView["result"].(map[string]any)
	if floorResult["estimatedLevel"] != "N5" || floorResult["floor"] != true {
		t.Fatalf("all-wrong result = %v", floorResult)
	}

	statusCode, levels := send(http.MethodGet, "/profile/levels", assessOwner, nil)
	if statusCode != http.StatusOK {
		t.Fatalf("profile levels: status %d", statusCode)
	}
	levelItems := levels["levels"].([]any)
	if len(levelItems) != 1 {
		t.Fatalf("profile levels = %v, want one ja entry", levels)
	}
	if entry := levelItems[0].(map[string]any); entry["language"] != "ja" || entry["level"] != "N5" {
		t.Fatalf("profile level = %v, want latest assessment (N5)", entry)
	}

	statusCode, history := send(http.MethodGet, "/assessments", assessOwner, nil)
	if statusCode != http.StatusOK || len(history["items"].([]any)) != 2 {
		t.Fatalf("assessment history: status %d body %v", statusCode, history)
	}
}
