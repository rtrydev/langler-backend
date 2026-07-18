package lessons

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

const (
	referenceVocabSlice   = 20
	referenceGrammarSlice = 8
	maxPromptTopicLen     = 120
)

var languageNames = map[domain.Language]string{
	"ja": "Japanese",
	"my": "Burmese",
	"pl": "Polish",
}

var lengthGuidance = map[string]string{
	"short":    "3-4 exercises, roughly 10 minutes",
	"standard": "5-7 exercises, roughly 15-20 minutes",
	"long":     "8-10 exercises, roughly 25-30 minutes",
}

func (s *Service) Build(ctx context.Context, query inbound.LessonPromptQuery) (inbound.LessonPrompt, error) {
	request, err := normalizePromptQuery(query)
	if err != nil {
		return inbound.LessonPrompt{}, err
	}

	var vocab []reference.VocabEntry
	var grammar []reference.GrammarTopic
	if request.includeReference {
		vocab, grammar, err = s.referenceSlice(ctx, request)
		if err != nil {
			return inbound.LessonPrompt{}, err
		}
	}

	return inbound.LessonPrompt{Prompt: composePrompt(request, vocab, grammar)}, nil
}

type promptRequest struct {
	language         domain.Language
	level            domain.Level
	topic            string
	exerciseTypes    []domain.ExerciseType
	stage            domain.ReadingStage
	length           string
	includeReference bool
}

func normalizePromptQuery(query inbound.LessonPromptQuery) (promptRequest, error) {
	var issues []domain.Issue
	add := func(path, message string, args ...any) {
		issues = append(issues, domain.Issue{Path: path, Message: fmt.Sprintf(message, args...)})
	}

	request := promptRequest{
		language:         domain.Language(query.Language),
		level:            domain.Level(strings.ToUpper(query.Level)),
		topic:            strings.TrimSpace(query.Topic),
		stage:            domain.ReadingStage(query.ReadingStage),
		length:           query.Length,
		includeReference: query.IncludeReference,
	}

	if !domain.KnownLanguage(request.language) {
		add("language", "must be one of %s", joinStrings(domain.Languages()))
	} else if !domain.KnownLevel(request.language, request.level) {
		add("level", "must be one of %s for language %q", joinStrings(domain.LevelsFor(request.language)), request.language)
	}

	if request.stage == "" {
		request.stage = domain.StageConnected
	}
	if !domain.KnownStage(request.stage) {
		add("readingStage", "must be %q or %q", domain.StageConnected, domain.StageFoundational)
	}

	if request.length == "" {
		request.length = "standard"
	}
	if _, ok := lengthGuidance[request.length]; !ok {
		add("length", `must be "short", "standard", or "long"`)
	}

	if utf8.RuneCountInString(request.topic) > maxPromptTopicLen {
		add("topic", "must be at most %d characters", maxPromptTopicLen)
	}

	if len(query.ExerciseTypes) == 0 {
		add("exerciseTypes", "must contain at least one exercise type")
	}
	for i, raw := range query.ExerciseTypes {
		t := domain.ExerciseType(raw)
		if !domain.KnownExerciseType(t) {
			add(fmt.Sprintf("exerciseTypes[%d]", i), "unknown exercise type %q; valid types are %s", raw, joinStrings(domain.ExerciseTypes()))
			continue
		}
		if !slices.Contains(request.exerciseTypes, t) {
			request.exerciseTypes = append(request.exerciseTypes, t)
		}
	}
	if request.stage == domain.StageConnected && !slices.Contains(request.exerciseTypes, domain.TypeReading) {
		request.exerciseTypes = append(request.exerciseTypes, domain.TypeReading)
	}
	if request.stage == domain.StageFoundational {
		request.exerciseTypes = slices.DeleteFunc(request.exerciseTypes, func(t domain.ExerciseType) bool {
			return t == domain.TypeReading
		})
		if len(request.exerciseTypes) == 0 && len(issues) == 0 {
			request.exerciseTypes = []domain.ExerciseType{domain.TypeScriptPractice, domain.TypeMatching, domain.TypeCloze}
		}
	}

	if len(issues) > 0 {
		return promptRequest{}, &domain.ValidationError{Issues: issues}
	}
	return request, nil
}

func (s *Service) referenceSlice(ctx context.Context, request promptRequest) ([]reference.VocabEntry, []reference.GrammarTopic, error) {
	lang, err := reference.NewLanguage(string(request.language))
	if err != nil {
		return nil, nil, err
	}
	level, err := reference.NewLevel(string(request.level))
	if err != nil {
		return nil, nil, err
	}

	vocabPage, err := s.reader.Vocab(ctx, outbound.VocabFilter{Language: lang, Level: level, Limit: referenceVocabSlice})
	if err != nil {
		return nil, nil, err
	}
	grammarPage, err := s.reader.Grammar(ctx, outbound.GrammarFilter{Language: lang, Level: level, Limit: referenceGrammarSlice})
	if err != nil {
		return nil, nil, err
	}
	return vocabPage.Entries, grammarPage.Topics, nil
}

func composePrompt(request promptRequest, vocab []reference.VocabEntry, grammar []reference.GrammarTopic) string {
	var b strings.Builder
	levelLabel := string(request.level)
	if request.language == "ja" {
		levelLabel = "JLPT " + levelLabel
	} else {
		levelLabel = "CEFR " + levelLabel
	}

	types := make([]string, 0, len(request.exerciseTypes))
	for _, t := range request.exerciseTypes {
		types = append(types, string(t))
	}

	fmt.Fprintf(&b, "You are generating a lesson for Langler, a personal language-learning app. Follow every instruction exactly and return only JSON.\n\n")
	fmt.Fprintf(&b, "## Lesson parameters\n")
	fmt.Fprintf(&b, "- Language: %s (%s)\n", languageNames[request.language], request.language)
	fmt.Fprintf(&b, "- Level: %s\n", levelLabel)
	if request.topic != "" {
		fmt.Fprintf(&b, "- Topic: %s\n", request.topic)
	}
	fmt.Fprintf(&b, "- Reading stage: %s\n", request.stage)
	fmt.Fprintf(&b, "- Exercise types to use: %s\n", strings.Join(types, ", "))
	fmt.Fprintf(&b, "- Length: %s (%s)\n\n", request.length, lengthGuidance[request.length])

	if request.stage == domain.StageConnected {
		fmt.Fprintf(&b, "## Story requirement\n")
		fmt.Fprintf(&b, "The lesson must end with a \"reading\" exercise whose payload has \"genre\": \"short_story\".\n")
		fmt.Fprintf(&b, "- Write a short, original story appropriate for %s: a coherent miniature narrative on the topic, not disconnected sentences.\n", levelLabel)
		fmt.Fprintf(&b, "- Use the lesson's target vocabulary and grammar in context and introduce as little language outside the reference list as possible.\n")
		fmt.Fprintf(&b, "- Give the story a title and 2-4 comprehension questions about its content.\n\n")
	} else {
		fmt.Fprintf(&b, "## Foundational lesson\n")
		fmt.Fprintf(&b, "This learner cannot decode connected text yet. Do not include any \"reading\" exercise or story.\n")
		fmt.Fprintf(&b, "Focus on script practice, individual words, and very short sentences that build glyph recognition and sound-script associations.\n")
		fmt.Fprintf(&b, "Set \"readingStage\": \"foundational\" in the output.\n\n")
	}

	if len(vocab) > 0 || len(grammar) > 0 {
		fmt.Fprintf(&b, "## Reference data\n")
		fmt.Fprintf(&b, "Ground the lesson in these vetted items. When an exercise uses one, put its id in that exercise's referencedVocab or referencedGrammar array. Use only ids listed here; never invent ids.\n")
		if len(vocab) > 0 {
			fmt.Fprintf(&b, "Vocabulary (id | headword | reading | gloss):\n")
			for _, entry := range vocab {
				fmt.Fprintf(&b, "- %s | %s | %s | %s\n", entry.ID, entry.Headword, entry.Reading, strings.Join(entry.Gloss, "; "))
			}
		}
		if len(grammar) > 0 {
			fmt.Fprintf(&b, "Grammar topics (id | name | description):\n")
			for _, topic := range grammar {
				fmt.Fprintf(&b, "- %s | %s | %s\n", topic.ID, topic.Name, topic.Description)
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Output format\n")
	fmt.Fprintf(&b, "Return exactly one JSON object and nothing else: no markdown fences, no commentary, no trailing text.\n")
	fmt.Fprintf(&b, "Top-level fields:\n")
	fmt.Fprintf(&b, "- \"schemaVersion\": \"%s\"\n", domain.SchemaVersion)
	fmt.Fprintf(&b, "- \"lessonId\": a freshly generated random UUID v4\n")
	fmt.Fprintf(&b, "- \"language\": \"%s\"\n", request.language)
	fmt.Fprintf(&b, "- \"level\": \"%s\"\n", request.level)
	fmt.Fprintf(&b, "- \"title\": short lesson title (max 160 chars)\n")
	fmt.Fprintf(&b, "- \"description\": one or two sentences about the lesson (optional)\n")
	fmt.Fprintf(&b, "- \"topic\": the lesson topic (optional)\n")
	fmt.Fprintf(&b, "- \"tags\": up to 10 short tags (optional)\n")
	fmt.Fprintf(&b, "- \"readingStage\": \"%s\"\n", request.stage)
	fmt.Fprintf(&b, "- \"sourceModel\": your model name (optional)\n")
	fmt.Fprintf(&b, "- \"estimatedMinutes\": whole minutes to finish the lesson (optional)\n")
	fmt.Fprintf(&b, "- \"exercises\": array of exercise objects\n\n")
	fmt.Fprintf(&b, "Every exercise object has:\n")
	fmt.Fprintf(&b, "{\"exerciseId\": \"ex-1\", \"type\": \"<type>\", \"prompt\": \"<learner-facing instruction>\", \"points\": 1-20, \"referencedVocab\": [\"<id>\"], \"referencedGrammar\": [\"<id>\"], \"payload\": {...}}\n")
	fmt.Fprintf(&b, "exerciseId values must be unique. Payload shapes by type:\n")
	fmt.Fprintf(&b, "- cloze: {\"text\": \"sentence with {{1}} and {{2}} markers\", \"blanks\": [{\"index\": 1, \"answer\": \"...\", \"hint\": \"optional\"}]} - every {{n}} marker needs exactly one blank with that index.\n")
	fmt.Fprintf(&b, "- translation: {\"source\": \"<sentence in %s>\", \"reference\": \"<English translation>\"}\n", languageNames[request.language])
	fmt.Fprintf(&b, "- ordering: {\"items\": [\"...\", \"...\"], \"translation\": \"optional\"} - list 2-20 items in the CORRECT order; the app shuffles them for the learner.\n")
	fmt.Fprintf(&b, "- matching: {\"pairs\": [{\"left\": \"<target language>\", \"right\": \"<meaning>\"}]} - 2-20 pairs.\n")
	fmt.Fprintf(&b, "- reading: {\"genre\": \"short_story\", \"title\": \"...\", \"passage\": \"...\", \"annotations\": [{\"surface\": \"...\", \"reading\": \"...\", \"gloss\": \"...\"}], \"questions\": [{\"question\": \"...\", \"kind\": \"multiple_choice\", \"options\": [\"...\"], \"answer\": \"<must equal one option>\"}]} - questions may also use {\"kind\": \"short_answer\", \"answer\": \"...\"} with no options.\n")
	fmt.Fprintf(&b, "- writing_prompt: put the writing task in the exercise's \"prompt\"; payload is optional: {\"guidance\": \"...\", \"modelAnswer\": \"...\"}\n")
	fmt.Fprintf(&b, "- script_practice: {\"items\": [{\"glyph\": \"<character or short word>\", \"reading\": \"...\", \"meaning\": \"...\"}]}\n\n")
	fmt.Fprintf(&b, "## Constraints\n")
	fmt.Fprintf(&b, "- Plain text only in every string: no HTML, no markdown, no control characters.\n")
	if request.language == "ja" {
		fmt.Fprintf(&b, "- All Japanese content (cloze texts, reading passages, translation sources, ordering items, practice glyphs) must be written in Japanese script.\n")
	}
	fmt.Fprintf(&b, "- Keep every referencedVocab/referencedGrammar id exactly as given in the reference data; omit the arrays when an exercise uses none.\n")
	fmt.Fprintf(&b, "- The output must parse as strict JSON (double quotes, no trailing commas, no comments).\n")

	return b.String()
}

func joinStrings[T ~string](values []T) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", string(v)))
	}
	return strings.Join(quoted, ", ")
}
