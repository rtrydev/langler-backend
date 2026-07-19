package lessons_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

func topicFixtures() []reference.Topic {
	return []reference.Topic{
		{Slug: "food-drink", Name: "Food & drink", Level: "N5", VocabIDs: []string{"N5#1", "N5#2", "N5#3", "N5#4"}},
		{Slug: "travel-transport", Name: "Travel & transport", Level: "N5", VocabIDs: []string{"N5#5", "N5#6"}},
		{Slug: "nature-weather", Name: "Nature & weather", Level: "N4", VocabIDs: []string{"N4#7"}},
	}
}

func TestTopicsReportsCoverageForLevel(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{topics: topicFixtures()}
	coverage := &fakeCoverage{vocab: []string{"N5#1", "N5#2", "N5#3", "N5#6"}}
	svc := newServiceWithCoverage(t, &fakeStore{}, &fakeChecker{}, reader, coverage)

	result, err := svc.Topics(context.Background(), inbound.LessonTopicsQuery{Owner: "user-1", Language: "ja", Level: "n5"})
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(result.Topics) != 2 {
		t.Fatalf("topics = %+v, want 2", result.Topics)
	}
	travel, food := result.Topics[0], result.Topics[1]
	if travel.Slug != "travel-transport" || food.Slug != "food-drink" {
		t.Fatalf("order = %q, %q; want least-covered first", travel.Slug, food.Slug)
	}
	if travel.WordCount != 2 || travel.CoveredCount != 1 {
		t.Errorf("travel counts = %d/%d, want 1/2", travel.CoveredCount, travel.WordCount)
	}
	if food.WordCount != 4 || food.CoveredCount != 3 {
		t.Errorf("food counts = %d/%d, want 3/4", food.CoveredCount, food.WordCount)
	}
	if food.Name != "Food & drink" {
		t.Errorf("name = %q", food.Name)
	}
}

func TestTopicsRequiresOwner(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	_, err := svc.Topics(context.Background(), inbound.LessonTopicsQuery{Language: "ja", Level: "N5"})
	if !errors.Is(err, domain.ErrInvalidOwner) {
		t.Fatalf("error = %v, want ErrInvalidOwner", err)
	}
}

func TestTopicsRejectsUnknownLanguageAndLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		level    string
		path     string
	}{
		{name: "unknown language", language: "xx", level: "N5", path: "language"},
		{name: "unknown level", language: "ja", level: "B1", path: "level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
			_, err := svc.Topics(context.Background(), inbound.LessonTopicsQuery{Owner: "user-1", Language: tt.language, Level: tt.level})
			var validation *domain.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want validation error", err)
			}
			if len(validation.Issues) != 1 || validation.Issues[0].Path != tt.path {
				t.Fatalf("issues = %+v, want single issue at %q", validation.Issues, tt.path)
			}
		})
	}
}

func TestBuildWithTopicSlugSelectsUncoveredTopicWords(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		topics: []reference.Topic{{Slug: "food-drink", Name: "Food & drink", Level: "N5", VocabIDs: []string{"N5#1", "N5#2", "N5#3"}}},
		byID: map[string]reference.VocabEntry{
			"N5#1": {ID: "N5#1", Headword: "水", Reading: "みず", Gloss: []string{"water"}},
			"N5#2": {ID: "N5#2", Headword: "魚", Reading: "さかな", Gloss: []string{"fish"}},
			"N5#3": {ID: "N5#3", Headword: "肉", Reading: "にく", Gloss: []string{"meat"}},
		},
	}
	coverage := &fakeCoverage{vocab: []string{"N5#1"}}
	svc := newServiceWithCoverage(t, &fakeStore{}, &fakeChecker{}, reader, coverage)

	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N5",
		Topic:            "Food & drink",
		TopicSlug:        "food-drink",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	fish := strings.Index(result.Prompt, "N5#2 | 魚")
	meat := strings.Index(result.Prompt, "N5#3 | 肉")
	water := strings.Index(result.Prompt, "N5#1 | 水")
	if fish == -1 || meat == -1 || water == -1 {
		t.Fatalf("prompt missing topic vocabulary:\n%s", result.Prompt)
	}
	if fish >= meat || meat >= water {
		t.Errorf("order = fish@%d meat@%d water@%d, want uncovered before covered", fish, meat, water)
	}
}

func TestBuildRejectsUnknownTopicSlug(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{topics: topicFixtures()})
	_, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N5",
		TopicSlug:        "space-travel",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	if len(validation.Issues) != 1 || validation.Issues[0].Path != "topicSlug" {
		t.Fatalf("issues = %+v", validation.Issues)
	}
}

func TestBuildRejectsMalformedTopicSlug(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	_, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:      "ja",
		Level:         "N5",
		TopicSlug:     "Food & Drink",
		ExerciseTypes: []string{"cloze"},
	})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	if len(validation.Issues) != 1 || validation.Issues[0].Path != "topicSlug" {
		t.Fatalf("issues = %+v", validation.Issues)
	}
}

func TestBuildWithoutTopicPrefersUncoveredLevelWords(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		vocab: outbound.VocabPage{Entries: []reference.VocabEntry{
			{ID: "N5#1", Headword: "犬", Gloss: []string{"dog"}},
			{ID: "N5#2", Headword: "猫", Gloss: []string{"cat"}},
		}},
		grammar: outbound.GrammarPage{Topics: []reference.GrammarTopic{
			{ID: "N5#a", Name: "Particle wa", Description: "Topic marker."},
			{ID: "N5#b", Name: "Particle ga", Description: "Subject marker."},
		}},
	}
	coverage := &fakeCoverage{vocab: []string{"N5#1"}, grammar: []string{"N5#a"}}
	svc := newServiceWithCoverage(t, &fakeStore{}, &fakeChecker{}, reader, coverage)

	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N5",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	cat := strings.Index(result.Prompt, "N5#2 | 猫")
	dog := strings.Index(result.Prompt, "N5#1 | 犬")
	if cat == -1 || dog == -1 || cat > dog {
		t.Errorf("vocab order cat@%d dog@%d, want uncovered first", cat, dog)
	}
	ga := strings.Index(result.Prompt, "N5#b | Particle ga")
	wa := strings.Index(result.Prompt, "N5#a | Particle wa")
	if ga == -1 || wa == -1 || ga > wa {
		t.Errorf("grammar order ga@%d wa@%d, want uncovered first", ga, wa)
	}
}

func TestBuildFreeTextTopicMatchesCuratedTopic(t *testing.T) {
	t.Parallel()

	byID := map[string]reference.VocabEntry{
		"N5#10": {ID: "N5#10", Headword: "駅", Reading: "えき", Gloss: []string{"station"}},
		"N5#11": {ID: "N5#11", Headword: "電車", Reading: "でんしゃ", Gloss: []string{"train"}},
		"N5#20": {ID: "N5#20", Headword: "水", Reading: "みず", Gloss: []string{"water"}},
	}
	reader := &fakeReader{
		topics: []reference.Topic{
			{Slug: "travel-transport", Name: "Travel & transport", Level: "N5", Keywords: []string{"trip", "travel", "train"}, VocabIDs: []string{"N5#10", "N5#11"}},
			{Slug: "food-drink", Name: "Food & drink", Level: "N5", Keywords: []string{"food", "eat"}, VocabIDs: []string{"N5#20"}},
		},
		byID: byID,
	}
	svc := newService(t, &fakeStore{}, &fakeChecker{}, reader)

	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N5",
		Topic:            "A weekend trip to Kyoto",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(result.Prompt, "N5#10 | 駅") || !strings.Contains(result.Prompt, "N5#11 | 電車") {
		t.Errorf("prompt missing travel vocabulary:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "N5#20 | 水") {
		t.Errorf("prompt includes vocabulary from an unmatched topic")
	}
	if !strings.Contains(result.Prompt, "candidate pool") {
		t.Errorf("prompt missing candidate-pool instruction")
	}
}

func TestBuildFreeTextTopicMergesTwoBestMatches(t *testing.T) {
	t.Parallel()

	byID := map[string]reference.VocabEntry{
		"N5#10": {ID: "N5#10", Headword: "駅", Gloss: []string{"station"}},
		"N5#20": {ID: "N5#20", Headword: "水", Gloss: []string{"water"}},
		"N5#30": {ID: "N5#30", Headword: "犬", Gloss: []string{"dog"}},
	}
	reader := &fakeReader{
		topics: []reference.Topic{
			{Slug: "travel-transport", Level: "N5", Keywords: []string{"trip"}, VocabIDs: []string{"N5#10"}},
			{Slug: "food-drink", Level: "N5", Keywords: []string{"restaurant", "eat"}, VocabIDs: []string{"N5#20"}},
			{Slug: "nature-weather", Level: "N5", Keywords: []string{"dog"}, VocabIDs: []string{"N5#30"}},
		},
		byID: byID,
	}
	svc := newService(t, &fakeStore{}, &fakeChecker{}, reader)

	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N5",
		Topic:            "eating at a restaurant after a trip",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(result.Prompt, "N5#20 | 水") || !strings.Contains(result.Prompt, "N5#10 | 駅") {
		t.Errorf("prompt missing the two matched topics' vocabulary:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "N5#30 | 犬") {
		t.Errorf("third-ranked topic leaked into the prompt")
	}
}

func TestBuildUnmatchedFreeTextTopicUsesLargerLevelPool(t *testing.T) {
	t.Parallel()

	entries := make([]reference.VocabEntry, 0, 60)
	for i := range 60 {
		entries = append(entries, reference.VocabEntry{ID: fmt.Sprintf("N5#%02d", i), Headword: fmt.Sprintf("word%02d", i), Gloss: []string{"gloss"}})
	}
	reader := &fakeReader{
		vocab: outbound.VocabPage{Entries: entries},
		topics: []reference.Topic{
			{Slug: "travel-transport", Level: "N5", Keywords: []string{"trip"}, VocabIDs: []string{"N5#00"}},
		},
	}
	svc := newService(t, &fakeStore{}, &fakeChecker{}, reader)

	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N5",
		Topic:            "京都へ行く",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(result.Prompt, "candidate pool") {
		t.Errorf("prompt missing candidate-pool instruction")
	}
	if !strings.Contains(result.Prompt, "N5#39 | word39") {
		t.Errorf("prompt pool smaller than 40 entries")
	}
	if strings.Contains(result.Prompt, "N5#40 | word40") {
		t.Errorf("prompt pool larger than 40 entries")
	}
}

func TestBuildWithoutTopicKeepsStrictSlice(t *testing.T) {
	t.Parallel()

	entries := make([]reference.VocabEntry, 0, 30)
	for i := range 30 {
		entries = append(entries, reference.VocabEntry{ID: fmt.Sprintf("N5#%02d", i), Headword: fmt.Sprintf("word%02d", i), Gloss: []string{"gloss"}})
	}
	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{vocab: outbound.VocabPage{Entries: entries}})

	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:         "ja",
		Level:            "N5",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(result.Prompt, "candidate pool") {
		t.Errorf("topic-less prompt should not use pool semantics")
	}
	if strings.Contains(result.Prompt, "N5#20 | word20") {
		t.Errorf("topic-less prompt exceeded the strict %d-word slice", 20)
	}
}
