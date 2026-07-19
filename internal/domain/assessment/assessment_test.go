package assessment_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/assessment"
)

func testRand() func(int) int {
	rng := rand.New(rand.NewPCG(7, 11))
	return rng.IntN
}

func vocabPool(size int, withExamples bool) []assessment.VocabCandidate {
	pool := make([]assessment.VocabCandidate, 0, size)
	for i := range size {
		candidate := assessment.VocabCandidate{
			ID:            fmt.Sprintf("N5#%d", i),
			Headword:      fmt.Sprintf("word-%d", i),
			Reading:       fmt.Sprintf("reading-%d", i),
			Gloss:         fmt.Sprintf("gloss-%d", i),
			PartsOfSpeech: []string{"n"},
		}
		if withExamples {
			candidate.Example = fmt.Sprintf("sentence-%d", i)
			candidate.ExampleTranslation = fmt.Sprintf("translation-%d", i)
		}
		pool = append(pool, candidate)
	}
	return pool
}

func grammarPool(size int) []assessment.GrammarCandidate {
	pool := make([]assessment.GrammarCandidate, 0, size)
	for i := range size {
		pool = append(pool, assessment.GrammarCandidate{
			ID:                 fmt.Sprintf("N5#topic-%d", i),
			Example:            fmt.Sprintf("grammar-sentence-%d", i),
			ExampleTranslation: fmt.Sprintf("grammar-translation-%d", i),
		})
	}
	return pool
}

func TestNewSessionValidatesInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	if _, err := assessment.NewSession("", "ja", now); !errors.Is(err, assessment.ErrInvalidAssessment) {
		t.Fatalf("empty id error = %v", err)
	}
	if _, err := assessment.NewSession("a-1", "xx", now); !errors.Is(err, assessment.ErrInvalidAssessment) {
		t.Fatalf("unknown language error = %v", err)
	}

	session, err := assessment.NewSession("a-1", "ja", now)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !slices.Equal(session.Bands, []string{"N5", "N4", "N3", "N2", "N1"}) {
		t.Fatalf("bands = %v", session.Bands)
	}
	if session.Status != assessment.StatusInProgress || session.Version != 1 {
		t.Fatalf("session = %+v", session)
	}
}

func TestBuildStageComposesItemMix(t *testing.T) {
	t.Parallel()

	stage, err := assessment.BuildStage("N5", vocabPool(20, true), grammarPool(6), testRand())
	if err != nil {
		t.Fatalf("BuildStage: %v", err)
	}
	if len(stage.Items) != 10 {
		t.Fatalf("items = %d, want 10", len(stage.Items))
	}
	counts := map[assessment.ItemKind]int{}
	for _, item := range stage.Items {
		counts[item.Kind]++
		if len(item.Options) != assessment.OptionCount {
			t.Fatalf("options = %d, want %d", len(item.Options), assessment.OptionCount)
		}
		if item.CorrectIndex < 0 || item.CorrectIndex >= len(item.Options) {
			t.Fatalf("correct index %d out of range", item.CorrectIndex)
		}
		seen := map[string]bool{}
		for _, option := range item.Options {
			if seen[option] {
				t.Fatalf("duplicate option %q in %v", option, item.Options)
			}
			seen[option] = true
		}
	}
	if counts[assessment.KindVocab] != 6 || counts[assessment.KindGrammar] != 2 || counts[assessment.KindReading] != 2 {
		t.Fatalf("kind counts = %v", counts)
	}
}

func TestVocabDistractorsAreTrapsFromTheSameKanjiFamily(t *testing.T) {
	t.Parallel()

	families := map[string][]assessment.VocabCandidate{
		"gaku": {
			{ID: "N5#1", Headword: "学校", Reading: "がっこう", Gloss: "school", PartsOfSpeech: []string{"n"}},
			{ID: "N5#2", Headword: "学生", Reading: "がくせい", Gloss: "student", PartsOfSpeech: []string{"n"}},
			{ID: "N5#3", Headword: "大学", Reading: "だいがく", Gloss: "university", PartsOfSpeech: []string{"n"}},
			{ID: "N5#4", Headword: "学期", Reading: "がっき", Gloss: "school term", PartsOfSpeech: []string{"n"}},
		},
		"hana": {
			{ID: "N5#5", Headword: "花見", Reading: "はなみ", Gloss: "blossom viewing", PartsOfSpeech: []string{"n"}},
			{ID: "N5#6", Headword: "花火", Reading: "はなび", Gloss: "fireworks", PartsOfSpeech: []string{"n"}},
			{ID: "N5#7", Headword: "花束", Reading: "はなたば", Gloss: "bouquet", PartsOfSpeech: []string{"n"}},
			{ID: "N5#8", Headword: "花園", Reading: "はなぞの", Gloss: "flower garden", PartsOfSpeech: []string{"n"}},
		},
	}
	familyByGloss := map[string]string{}
	var pool []assessment.VocabCandidate
	for family, members := range families {
		for _, member := range members {
			familyByGloss[member.Gloss] = family
			pool = append(pool, member)
		}
	}

	stage, err := assessment.BuildStage("N5", pool, nil, testRand())
	if err != nil {
		t.Fatalf("BuildStage: %v", err)
	}
	for _, item := range stage.Items {
		correctFamily := familyByGloss[item.Options[item.CorrectIndex]]
		for _, option := range item.Options {
			if familyByGloss[option] != correctFamily {
				t.Fatalf("prompt %q mixes families in options %v", item.Prompt, item.Options)
			}
		}
	}
}

func TestSentenceDistractorsShareContentWithTheCorrectAnswer(t *testing.T) {
	t.Parallel()

	grammar := []assessment.GrammarCandidate{
		{ID: "N5#g1", Example: "私は学校へ行きます", ExampleTranslation: "I go to school every morning"},
		{ID: "N5#g2", Example: "学校で勉強します", ExampleTranslation: "I study at school with friends"},
		{ID: "N5#g3", Example: "兄は大学へ行きました", ExampleTranslation: "My brother went to school yesterday"},
		{ID: "N5#g4", Example: "学校の先生は優しいです", ExampleTranslation: "The school teachers are kind"},
		{ID: "N5#g5", Example: "天気がいいです", ExampleTranslation: "The weather is nice today"},
		{ID: "N5#g6", Example: "猫がかわいいです", ExampleTranslation: "The cat is very cute"},
		{ID: "N5#g7", Example: "音楽を聞きます", ExampleTranslation: "Music plays on the radio"},
		{ID: "N5#g8", Example: "映画を見ます", ExampleTranslation: "The film starts in the evening"},
	}
	schoolTranslations := map[string]bool{
		"I go to school every morning": true, "I study at school with friends": true,
		"My brother went to school yesterday": true, "The school teachers are kind": true,
	}

	stage, err := assessment.BuildStage("N5", vocabPool(20, false), grammar, testRand())
	if err != nil {
		t.Fatalf("BuildStage: %v", err)
	}
	for _, item := range stage.Items {
		if item.Kind != assessment.KindGrammar {
			continue
		}
		correctIsSchool := schoolTranslations[item.Options[item.CorrectIndex]]
		if !correctIsSchool {
			continue
		}
		for _, option := range item.Options {
			if !schoolTranslations[option] {
				t.Fatalf("prompt %q has unrelated distractor in %v", item.Prompt, item.Options)
			}
		}
	}
}

func TestBuildStageFallsBackToVocabWhenPoolsAreThin(t *testing.T) {
	t.Parallel()

	stage, err := assessment.BuildStage("N5", vocabPool(20, false), nil, testRand())
	if err != nil {
		t.Fatalf("BuildStage: %v", err)
	}
	if len(stage.Items) != 10 {
		t.Fatalf("items = %d, want 10", len(stage.Items))
	}
	for _, item := range stage.Items {
		if item.Kind != assessment.KindVocab {
			t.Fatalf("kind = %s, want vocab fallback", item.Kind)
		}
	}
}

func TestBuildStageRejectsInsufficientVocabulary(t *testing.T) {
	t.Parallel()

	if _, err := assessment.BuildStage("A1", vocabPool(3, false), nil, testRand()); !errors.Is(err, assessment.ErrInsufficientReference) {
		t.Fatalf("error = %v, want ErrInsufficientReference", err)
	}
}

func newStartedSession(t *testing.T) assessment.Session {
	t.Helper()
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	session, err := assessment.NewSession("a-1", "ja", now)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	stage, err := assessment.BuildStage(session.Bands[0], vocabPool(20, true), grammarPool(6), testRand())
	if err != nil {
		t.Fatalf("BuildStage: %v", err)
	}
	session.Stages = []assessment.Stage{stage}
	return session
}

func answersFor(stage assessment.Stage, correct int) []int {
	answers := make([]int, len(stage.Items))
	for i, item := range stage.Items {
		if i < correct {
			answers[i] = item.CorrectIndex
		} else {
			answers[i] = (item.CorrectIndex + 1) % len(item.Options)
		}
	}
	return answers
}

func TestAnswerValidatesSubmission(t *testing.T) {
	t.Parallel()

	session := newStartedSession(t)
	now := time.Date(2026, 7, 19, 10, 5, 0, 0, time.UTC)

	if _, err := assessment.Answer(session, 1, answersFor(session.Stages[0], 10), now); !errors.Is(err, assessment.ErrInvalidAnswer) {
		t.Fatalf("wrong stage error = %v", err)
	}
	if _, err := assessment.Answer(session, 0, []int{0, 1}, now); !errors.Is(err, assessment.ErrInvalidAnswer) {
		t.Fatalf("short answers error = %v", err)
	}
	bad := answersFor(session.Stages[0], 10)
	bad[0] = 9
	if _, err := assessment.Answer(session, 0, bad, now); !errors.Is(err, assessment.ErrInvalidAnswer) {
		t.Fatalf("out of range error = %v", err)
	}

	answered, err := assessment.Answer(session, 0, answersFor(session.Stages[0], 10), now)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if _, err := assessment.Answer(answered, 0, answersFor(answered.Stages[0], 10), now); !errors.Is(err, assessment.ErrInvalidAnswer) && !errors.Is(err, assessment.ErrAlreadyCompleted) {
		t.Fatalf("double answer error = %v", err)
	}
}

func TestAnswerAdvancesOnPassAndStopsOnFail(t *testing.T) {
	t.Parallel()

	session := newStartedSession(t)
	now := time.Date(2026, 7, 19, 10, 5, 0, 0, time.UTC)

	passed, err := assessment.Answer(session, 0, answersFor(session.Stages[0], 8), now)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if passed.Status != assessment.StatusInProgress || passed.Version != 2 {
		t.Fatalf("passed session = status %s version %d", passed.Status, passed.Version)
	}
	if band, ok := assessment.NextBand(passed); !ok || band != "N4" {
		t.Fatalf("next band = %q %v", band, ok)
	}

	failed, err := assessment.Answer(session, 0, answersFor(session.Stages[0], 7), now)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if failed.Status != assessment.StatusCompleted {
		t.Fatalf("failed session status = %s", failed.Status)
	}
	if failed.EstimatedLevel != "N5" || !failed.Floor {
		t.Fatalf("failed result = %q floor=%v", failed.EstimatedLevel, failed.Floor)
	}
	if _, ok := assessment.NextBand(failed); ok {
		t.Fatal("completed session offers next band")
	}
}

func runToCompletion(t *testing.T, correctByBand map[string]int) assessment.Session {
	t.Helper()
	session := newStartedSession(t)
	now := time.Date(2026, 7, 19, 10, 5, 0, 0, time.UTC)
	for session.Status == assessment.StatusInProgress {
		index := len(session.Stages) - 1
		stage := session.Stages[index]
		next, err := assessment.Answer(session, index, answersFor(stage, correctByBand[stage.Band]), now)
		if err != nil {
			t.Fatalf("Answer %s: %v", stage.Band, err)
		}
		session = next
		if band, ok := assessment.NextBand(session); ok {
			built, err := assessment.BuildStage(band, vocabPool(20, true), grammarPool(6), testRand())
			if err != nil {
				t.Fatalf("BuildStage %s: %v", band, err)
			}
			session.Stages = append(session.Stages, built)
		}
	}
	return session
}

func TestSeededAnswerPatternsYieldOrderedEstimates(t *testing.T) {
	t.Parallel()

	patterns := map[string]map[string]int{
		"N5": {"N5": 10, "N4": 3, "N3": 0, "N2": 0, "N1": 0},
		"N3": {"N5": 10, "N4": 9, "N3": 8, "N2": 4, "N1": 0},
		"N1": {"N5": 10, "N4": 10, "N3": 9, "N2": 8, "N1": 8},
	}
	for trueLevel, correctByBand := range patterns {
		session := runToCompletion(t, correctByBand)
		if session.EstimatedLevel != trueLevel {
			t.Fatalf("pattern %s estimated %s", trueLevel, session.EstimatedLevel)
		}
	}
}

func TestConfidenceReflectsMargin(t *testing.T) {
	t.Parallel()

	decisive := runToCompletion(t, map[string]int{"N5": 10, "N4": 10, "N3": 2})
	if decisive.EstimatedLevel != "N4" || decisive.Confidence != assessment.ConfidenceHigh {
		t.Fatalf("decisive = %s %s", decisive.EstimatedLevel, decisive.Confidence)
	}

	borderline := runToCompletion(t, map[string]int{"N5": 10, "N4": 8, "N3": 7})
	if borderline.EstimatedLevel != "N4" || borderline.Confidence != assessment.ConfidenceLow {
		t.Fatalf("borderline = %s %s", borderline.EstimatedLevel, borderline.Confidence)
	}

	ceiling := runToCompletion(t, map[string]int{"N5": 10, "N4": 10, "N3": 10, "N2": 10, "N1": 10})
	if ceiling.EstimatedLevel != "N1" || ceiling.Confidence != assessment.ConfidenceHigh || ceiling.Floor {
		t.Fatalf("ceiling = %s %s floor=%v", ceiling.EstimatedLevel, ceiling.Confidence, ceiling.Floor)
	}
}
