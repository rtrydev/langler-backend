package assessment

import (
	"errors"
	"math"
	"slices"
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
	stageVocabItems   = 6
	stageGrammarItems = 2
	stageReadingItems = 2
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
	Reading            string
	Gloss              string
	PartsOfSpeech      []string
	Example            string
	ExampleTranslation string
}

type GrammarCandidate struct {
	ID                 string
	Example            string
	ExampleTranslation string
}

type choice struct {
	id      string
	prompt  string
	answer  string
	reading string
	pos     []string
}

func BuildStage(band string, difficulty float64, vocab []VocabCandidate, grammar []GrammarCandidate, intn func(int) int) (Stage, error) {
	if band == "" || difficulty < 0 || difficulty > 1 || intn == nil {
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
			vocabPool = append(vocabPool, choice{
				id: candidate.ID, prompt: candidate.Headword, answer: candidate.Gloss,
				reading: candidate.Reading, pos: candidate.PartsOfSpeech,
			})
		}
		if candidate.Example != "" && candidate.ExampleTranslation != "" {
			readingPool = append(readingPool, choice{id: candidate.ID, prompt: candidate.Example, answer: candidate.ExampleTranslation})
		}
	}

	sentenceSlack := int(math.Round((1 - difficulty) * 2))
	items := buildChoiceItems(KindGrammar, grammarPool, stageGrammarItems, sentenceSlack, intn, sentenceTrapScore)
	items = append(items, buildChoiceItems(KindReading, readingPool, stageReadingItems, sentenceSlack, intn, sentenceTrapScore)...)
	items = append(items, buildChoiceItems(KindVocab, vocabPool, stageItems-len(items), 0, intn, vocabTrapScore)...)
	if len(items) < minStageItems {
		return Stage{}, ErrInsufficientReference
	}
	shuffle(items, intn)
	return Stage{Band: band, Items: items}, nil
}

func buildChoiceItems(kind ItemKind, pool []choice, count, slack int, intn func(int) int, trapScore func(correct, candidate choice) int) []Item {
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
		traps := rankedTraps(correct, distinct, i, trapScore)
		if len(traps) < OptionCount-1 {
			continue
		}
		pick := traps[:min(len(traps), OptionCount-1+slack)]
		shuffle(pick, intn)
		options := make([]string, 0, OptionCount)
		for _, trap := range pick[:OptionCount-1] {
			options = append(options, trap.answer)
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

func rankedTraps(correct choice, distinct []choice, correctIndex int, trapScore func(correct, candidate choice) int) []choice {
	traps := make([]choice, 0, len(distinct)-1)
	for i, candidate := range distinct {
		if i != correctIndex {
			traps = append(traps, candidate)
		}
	}
	slices.SortStableFunc(traps, func(a, b choice) int {
		return trapScore(correct, b) - trapScore(correct, a)
	})
	return traps
}

func vocabTrapScore(correct, candidate choice) int {
	score := 4 * sharedRuneCount(hanRunes(correct.prompt), hanRunes(candidate.prompt))
	if sharesAny(correct.pos, candidate.pos) {
		score += 3
	}
	if strings.HasPrefix(correct.answer, "to ") == strings.HasPrefix(candidate.answer, "to ") {
		score += 2
	}
	score += min(2, commonPrefixRunes(correct.reading, candidate.reading))
	return score
}

func sentenceTrapScore(correct, candidate choice) int {
	score := 2 * sharedWordCount(correct.answer, candidate.answer)
	score += sharedRuneCount(hanRunes(correct.prompt), hanRunes(candidate.prompt))
	score += 2 * min(4, sharedStructureCount(correct.answer, candidate.answer))
	score += min(6, commonSuffixRunes(sentenceBody(correct.prompt), sentenceBody(candidate.prompt)))
	if hasNegation(correct.answer) != hasNegation(candidate.answer) {
		score -= 4
	}
	if isQuestion(correct.answer) != isQuestion(candidate.answer) {
		score -= 4
	}
	return score
}

func hanRunes(value string) map[rune]bool {
	runes := map[rune]bool{}
	for _, r := range value {
		if r >= 0x3400 && r <= 0x9FFF {
			runes[r] = true
		}
	}
	return runes
}

func sharedRuneCount(a, b map[rune]bool) int {
	count := 0
	for r := range a {
		if b[r] {
			count++
		}
	}
	return count
}

var trapStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "of": true, "in": true,
	"on": true, "at": true, "is": true, "are": true, "was": true, "were": true,
	"i": true, "he": true, "she": true, "it": true, "you": true, "we": true,
	"they": true, "my": true, "his": true, "her": true, "and": true, "or": true,
	"for": true, "with": true, "that": true, "this": true, "not": true,
	"do": true, "did": true, "have": true, "has": true, "had": true,
}

func contentWords(value string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if len(word) > 1 && !trapStopwords[word] {
			words[word] = true
		}
	}
	return words
}

func sharedWordCount(a, b string) int {
	words := contentWords(a)
	count := 0
	for word := range contentWords(b) {
		if words[word] {
			count++
		}
	}
	return count
}

var structureWords = map[string]bool{
	"will": true, "would": true, "shall": true, "can": true, "could": true,
	"must": true, "should": true, "might": true, "may": true,
	"do": true, "does": true, "did": true, "done": true,
	"is": true, "are": true, "am": true, "was": true, "were": true,
	"be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true,
	"not": true, "no": true, "never": true,
	"if": true, "because": true, "when": true, "while": true,
	"before": true, "after": true, "until": true, "than": true,
	"please": true, "want": true, "going": true, "used": true, "too": true,
}

func structureSignature(value string) map[string]bool {
	normalized := strings.ReplaceAll(strings.ToLower(value), "n't", " not")
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(normalized, func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if structureWords[word] {
			words[word] = true
		}
	}
	return words
}

func sharedStructureCount(a, b string) int {
	words := structureSignature(a)
	count := 0
	for word := range structureSignature(b) {
		if words[word] {
			count++
		}
	}
	return count
}

func hasNegation(value string) bool {
	signature := structureSignature(value)
	return signature["not"] || signature["no"] || signature["never"]
}

func isQuestion(value string) bool {
	return strings.Contains(value, "?")
}

func sentenceBody(value string) string {
	return strings.TrimRight(value, "。．.!?！？…」』\" ")
}

func commonSuffixRunes(a, b string) int {
	left := []rune(a)
	right := []rune(b)
	count := 0
	for count < len(left) && count < len(right) && left[len(left)-1-count] == right[len(right)-1-count] {
		count++
	}
	return count
}

func sharesAny(a, b []string) bool {
	for _, value := range a {
		if slices.Contains(b, value) {
			return true
		}
	}
	return false
}

func commonPrefixRunes(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	left := []rune(a)
	right := []rune(b)
	count := 0
	for count < len(left) && count < len(right) && left[count] == right[count] {
		count++
	}
	return count
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
