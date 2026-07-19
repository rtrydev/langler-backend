package assessment

import (
	"errors"
	"strings"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
)

var (
	ErrInvalidAssessment     = errors.New("assessment is invalid")
	ErrInvalidAnswer         = errors.New("assessment answer is invalid")
	ErrAlreadyCompleted      = errors.New("assessment is already completed")
	ErrNotFound              = errors.New("assessment not found")
	ErrConflict              = errors.New("assessment changed concurrently")
	ErrInsufficientReference = errors.New("reference data is insufficient to assemble a placement stage")
	ErrStorageFailure        = errors.New("assessment storage failed")
)

type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

type ItemKind string

const (
	KindVocab   ItemKind = "vocab"
	KindGrammar ItemKind = "grammar"
	KindReading ItemKind = "reading"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

const (
	stageVocabItems   = 5
	stageGrammarItems = 2
	stageReadingItems = 1
	stageItems        = stageVocabItems + stageGrammarItems + stageReadingItems
	minStageItems     = 4
	OptionCount       = 4
	passRatio         = 0.75
)

type Item struct {
	Kind         ItemKind
	Prompt       string
	Options      []string
	CorrectIndex int
	ReferenceID  string
}

type Stage struct {
	Band       string
	Items      []Item
	Answers    []int
	Answered   bool
	Correct    int
	AnsweredAt time.Time
}

type Session struct {
	ID             string
	Language       string
	Status         Status
	Bands          []string
	Stages         []Stage
	EstimatedLevel string
	Confidence     Confidence
	Floor          bool
	StartedAt      time.Time
	CompletedAt    time.Time
	Version        int
}

func NewSession(id, language string, now time.Time) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, ErrInvalidAssessment
	}
	if !lesson.KnownLanguage(lesson.Language(language)) {
		return Session{}, ErrInvalidAssessment
	}
	levels := lesson.LevelsFor(lesson.Language(language))
	bands := make([]string, 0, len(levels))
	for _, level := range levels {
		bands = append(bands, string(level))
	}
	return Session{
		ID:        id,
		Language:  language,
		Status:    StatusInProgress,
		Bands:     bands,
		StartedAt: now.UTC(),
		Version:   1,
	}, nil
}

type VocabCandidate struct {
	ID                 string
	Headword           string
	Gloss              string
	Example            string
	ExampleTranslation string
}

type GrammarCandidate struct {
	ID                 string
	Example            string
	ExampleTranslation string
}

type choice struct {
	id     string
	prompt string
	answer string
}

func BuildStage(band string, vocab []VocabCandidate, grammar []GrammarCandidate, intn func(int) int) (Stage, error) {
	if band == "" || intn == nil {
		return Stage{}, ErrInvalidAssessment
	}
	grammarPool := make([]choice, 0, len(grammar))
	for _, candidate := range grammar {
		if candidate.Example != "" && candidate.ExampleTranslation != "" {
			grammarPool = append(grammarPool, choice{id: candidate.ID, prompt: candidate.Example, answer: candidate.ExampleTranslation})
		}
	}
	readingPool := make([]choice, 0, len(vocab))
	vocabPool := make([]choice, 0, len(vocab))
	for _, candidate := range vocab {
		if candidate.Headword != "" && candidate.Gloss != "" {
			vocabPool = append(vocabPool, choice{id: candidate.ID, prompt: candidate.Headword, answer: candidate.Gloss})
		}
		if candidate.Example != "" && candidate.ExampleTranslation != "" {
			readingPool = append(readingPool, choice{id: candidate.ID, prompt: candidate.Example, answer: candidate.ExampleTranslation})
		}
	}

	items := buildChoiceItems(KindGrammar, grammarPool, stageGrammarItems, intn)
	items = append(items, buildChoiceItems(KindReading, readingPool, stageReadingItems, intn)...)
	items = append(items, buildChoiceItems(KindVocab, vocabPool, stageItems-len(items), intn)...)
	if len(items) < minStageItems {
		return Stage{}, ErrInsufficientReference
	}
	shuffle(items, intn)
	return Stage{Band: band, Items: items}, nil
}

func buildChoiceItems(kind ItemKind, pool []choice, count int, intn func(int) int) []Item {
	if count <= 0 {
		return nil
	}
	distinct := dedupe(pool)
	if len(distinct) < OptionCount {
		return nil
	}
	shuffle(distinct, intn)
	items := make([]Item, 0, count)
	for i := 0; i < len(distinct) && len(items) < count; i++ {
		correct := distinct[i]
		options := make([]string, 0, OptionCount)
		for j := 1; len(options) < OptionCount-1 && j < len(distinct); j++ {
			distractor := distinct[(i+j)%len(distinct)]
			options = append(options, distractor.answer)
		}
		if len(options) < OptionCount-1 {
			continue
		}
		position := intn(len(options) + 1)
		options = append(options[:position], append([]string{correct.answer}, options[position:]...)...)
		items = append(items, Item{
			Kind:         kind,
			Prompt:       correct.prompt,
			Options:      options,
			CorrectIndex: position,
			ReferenceID:  correct.id,
		})
	}
	return items
}

func dedupe(pool []choice) []choice {
	seenPrompt := make(map[string]bool, len(pool))
	seenAnswer := make(map[string]bool, len(pool))
	distinct := make([]choice, 0, len(pool))
	for _, entry := range pool {
		if seenPrompt[entry.prompt] || seenAnswer[entry.answer] {
			continue
		}
		seenPrompt[entry.prompt] = true
		seenAnswer[entry.answer] = true
		distinct = append(distinct, entry)
	}
	return distinct
}

func shuffle[T any](values []T, intn func(int) int) {
	for i := len(values) - 1; i > 0; i-- {
		j := intn(i + 1)
		values[i], values[j] = values[j], values[i]
	}
}

func Answer(session Session, stageIndex int, answers []int, now time.Time) (Session, error) {
	if session.Status == StatusCompleted {
		return Session{}, ErrAlreadyCompleted
	}
	if len(session.Stages) == 0 || stageIndex != len(session.Stages)-1 {
		return Session{}, ErrInvalidAnswer
	}
	stage := session.Stages[stageIndex]
	if stage.Answered {
		return Session{}, ErrInvalidAnswer
	}
	if len(answers) != len(stage.Items) {
		return Session{}, ErrInvalidAnswer
	}
	correct := 0
	for i, answer := range answers {
		if answer < 0 || answer >= len(stage.Items[i].Options) {
			return Session{}, ErrInvalidAnswer
		}
		if answer == stage.Items[i].CorrectIndex {
			correct++
		}
	}

	stage.Answers = append([]int(nil), answers...)
	stage.Answered = true
	stage.Correct = correct
	stage.AnsweredAt = now.UTC()
	session.Stages = append(append([]Stage(nil), session.Stages[:stageIndex]...), stage)
	session.Version++

	if !Passed(stage) || len(session.Stages) == len(session.Bands) {
		return complete(session, now), nil
	}
	return session, nil
}

func Passed(stage Stage) bool {
	return len(stage.Items) > 0 && accuracy(stage) >= passRatio
}

func accuracy(stage Stage) float64 {
	if len(stage.Items) == 0 {
		return 0
	}
	return float64(stage.Correct) / float64(len(stage.Items))
}

func NextBand(session Session) (string, bool) {
	if session.Status != StatusInProgress || len(session.Stages) >= len(session.Bands) {
		return "", false
	}
	return session.Bands[len(session.Stages)], true
}

func complete(session Session, now time.Time) Session {
	session.Status = StatusCompleted
	session.CompletedAt = now.UTC()

	last := session.Stages[len(session.Stages)-1]
	if Passed(last) {
		session.EstimatedLevel = last.Band
		if accuracy(last) >= 0.9 {
			session.Confidence = ConfidenceHigh
		} else {
			session.Confidence = ConfidenceMedium
		}
		return session
	}
	if len(session.Stages) == 1 {
		session.EstimatedLevel = session.Bands[0]
		session.Floor = true
		switch failed := accuracy(last); {
		case failed <= 0.4:
			session.Confidence = ConfidenceHigh
		case failed <= 0.6:
			session.Confidence = ConfidenceMedium
		default:
			session.Confidence = ConfidenceLow
		}
		return session
	}
	passed := session.Stages[len(session.Stages)-2]
	session.EstimatedLevel = passed.Band
	switch margin := accuracy(passed) - accuracy(last); {
	case margin >= 0.35:
		session.Confidence = ConfidenceHigh
	case margin >= 0.15:
		session.Confidence = ConfidenceMedium
	default:
		session.Confidence = ConfidenceLow
	}
	return session
}
