package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

func TestHandleVocab(t *testing.T) {
	t.Parallel()

	example := &domain.Example{
		Text:        "学校に行きます。",
		Translation: "I go to school.",
		SourceID:    "tatoeba",
		License:     "CC BY 2.0 FR",
	}
	provider := &fakeReferenceProvider{vocab: inbound.VocabResult{
		Entries: []domain.VocabEntry{{
			Headword:      "学校",
			Reading:       "がっこう",
			Gloss:         []string{"school"},
			PartsOfSpeech: []string{"n"},
			Level:         "N5",
			FreqBand:      2,
			Example:       example,
			SourceID:      "jmdict-simplified",
			License:       "CC BY-SA 4.0 (EDRDG)",
		}},
		NextCursor: "next-token",
	}}
	h := newHandler(t, fakeStatusProvider{}, provider)

	resp, err := h.Handle(context.Background(), getRequest("/reference/vocab", map[string]string{
		"lang":   "ja",
		"level":  "N5",
		"topic":  "daily-life",
		"limit":  "25",
		"cursor": "abc",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, body %s", resp.StatusCode, resp.Body)
	}

	wantQuery := inbound.VocabQuery{Language: "ja", Level: "N5", Topic: "daily-life", Limit: 25, Cursor: "abc"}
	if provider.vocabQuery != wantQuery {
		t.Errorf("query = %+v, want %+v", provider.vocabQuery, wantQuery)
	}

	var body struct {
		Items []struct {
			Headword string   `json:"headword"`
			Reading  string   `json:"reading"`
			Gloss    []string `json:"gloss"`
			Pos      []string `json:"pos"`
			Level    string   `json:"level"`
			FreqBand int      `json:"freqBand"`
			Example  *struct {
				Text        string `json:"text"`
				Translation string `json:"translation"`
			} `json:"example"`
			SourceID string `json:"sourceId"`
			License  string `json:"license"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Items))
	}
	item := body.Items[0]
	if item.Headword != "学校" || item.Reading != "がっこう" || item.Level != "N5" || item.FreqBand != 2 {
		t.Errorf("item = %+v, want the fake entry", item)
	}
	if item.Example == nil || item.Example.Translation != "I go to school." {
		t.Errorf("example = %+v, want the fake example", item.Example)
	}
	if item.SourceID != "jmdict-simplified" || item.License != "CC BY-SA 4.0 (EDRDG)" {
		t.Errorf("source metadata = %q/%q, want jmdict-simplified/CC BY-SA 4.0 (EDRDG)", item.SourceID, item.License)
	}
	if body.NextCursor != "next-token" {
		t.Errorf("nextCursor = %q, want %q", body.NextCursor, "next-token")
	}
}

func TestHandleGrammar(t *testing.T) {
	t.Parallel()

	provider := &fakeReferenceProvider{grammar: inbound.GrammarResult{
		Topics: []domain.GrammarTopic{{
			TopicID:     "particle-wa",
			Name:        "Topic particle は",
			Level:       "N5",
			Description: "Marks the topic of the sentence.",
			Example:     &domain.Example{Text: "私は学生です。", Translation: "I am a student."},
			SourceID:    "langler-curated",
			License:     "CC BY-SA 4.0",
		}},
	}}
	h := newHandler(t, fakeStatusProvider{}, provider)

	resp, err := h.Handle(context.Background(), getRequest("/reference/grammar", map[string]string{
		"lang":  "ja",
		"level": "N5",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, body %s", resp.StatusCode, resp.Body)
	}

	var body struct {
		Items []struct {
			TopicID     string `json:"topicId"`
			Name        string `json:"name"`
			Level       string `json:"level"`
			Description string `json:"description"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].TopicID != "particle-wa" {
		t.Fatalf("items = %+v, want particle-wa", body.Items)
	}
}

func TestHandleScripts(t *testing.T) {
	t.Parallel()

	provider := &fakeReferenceProvider{scripts: inbound.ScriptResult{
		Glyphs: []domain.ScriptGlyph{{
			Glyph:         "犬",
			ScriptType:    "kanji",
			Name:          "dog",
			Meanings:      []string{"dog"},
			Readings:      map[string][]string{"on": {"ケン"}, "kun": {"いぬ"}},
			Level:         "N5",
			Grade:         1,
			StrokeCount:   4,
			StrokeDataRef: "kanjivg/072ac.svg",
			SourceID:      "kanjidic2",
			License:       "CC BY-SA 4.0 (EDRDG)",
		}},
	}}
	h := newHandler(t, fakeStatusProvider{}, provider)

	resp, err := h.Handle(context.Background(), getRequest("/reference/scripts", map[string]string{
		"lang":  "ja",
		"type":  "kanji",
		"level": "N5",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, body %s", resp.StatusCode, resp.Body)
	}

	wantQuery := inbound.ScriptQuery{Language: "ja", ScriptType: "kanji", Level: "N5"}
	if provider.scriptQuery != wantQuery {
		t.Errorf("query = %+v, want %+v", provider.scriptQuery, wantQuery)
	}

	var body struct {
		Items []struct {
			Glyph         string              `json:"glyph"`
			ScriptType    string              `json:"scriptType"`
			Readings      map[string][]string `json:"readings"`
			StrokeCount   int                 `json:"strokeCount"`
			StrokeDataRef string              `json:"strokeDataRef"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Items))
	}
	item := body.Items[0]
	if item.Glyph != "犬" || item.StrokeDataRef != "kanjivg/072ac.svg" {
		t.Errorf("item = %+v, want the fake kanji", item)
	}
	if len(item.Readings["kun"]) != 1 || item.Readings["kun"][0] != "いぬ" {
		t.Errorf("kun readings = %v, want [いぬ]", item.Readings["kun"])
	}
}

func TestHandleReadings(t *testing.T) {
	t.Parallel()
	provider := &fakeReferenceProvider{readings: inbound.ReadingResult{
		Passages: []domain.ReadingPassage{{
			ID: "A2#story", Text: "မနက်ခင်းမှာ မေ ဈေးကို သွားတယ်။", Level: "A2",
			LevelApproximate: true, Coverage: 0.92, SourceID: "myanmar-wikipedia", License: "CC BY-SA 4.0",
		}},
		NextCursor: "more",
	}}
	h := newHandler(t, fakeStatusProvider{}, provider)
	response, err := h.Handle(context.Background(), getRequest("/reference/readings", map[string]string{
		"lang": "my", "level": "A2", "limit": "10",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.StatusCode, response.Body)
	}
	if provider.readingQuery != (inbound.ReadingQuery{Language: "my", Level: "A2", Limit: 10}) {
		t.Errorf("query = %+v", provider.readingQuery)
	}
	var body struct {
		Items []struct {
			Text     string  `json:"text"`
			Coverage float64 `json:"coverage"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Coverage != 0.92 || body.NextCursor != "more" {
		t.Errorf("body = %+v", body)
	}
}

func TestReferenceErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		params     map[string]string
		err        error
		wantStatus int
	}{
		{
			name:       "missing lang",
			params:     nil,
			err:        domain.ErrInvalidLanguage,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid level",
			params:     map[string]string{"lang": "ja", "level": "bogus level"},
			err:        domain.ErrInvalidLevel,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid cursor",
			params:     map[string]string{"lang": "ja"},
			err:        domain.ErrInvalidCursor,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "storage failure",
			params:     map[string]string{"lang": "ja"},
			err:        context.DeadlineExceeded,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(t, fakeStatusProvider{}, &fakeReferenceProvider{err: tt.err})
			resp, err := h.Handle(context.Background(), getRequest("/reference/vocab", tt.params))
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("StatusCode = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			var body map[string]string
			if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			if body["error"] == "" {
				t.Error("error message missing from body")
			}
		})
	}
}

func TestInvalidLimitRejected(t *testing.T) {
	t.Parallel()

	h := newHandler(t, fakeStatusProvider{}, &fakeReferenceProvider{})
	for _, limit := range []string{"abc", "-1", "0", "1.5"} {
		resp, err := h.Handle(context.Background(), getRequest("/reference/vocab", map[string]string{
			"lang":  "ja",
			"limit": limit,
		}))
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("limit %q: StatusCode = %d, want %d", limit, resp.StatusCode, http.StatusBadRequest)
		}
	}
}
