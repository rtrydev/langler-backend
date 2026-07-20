package lessons

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

const (
	referenceVocabSlice   = 20
	matchedTopicVocabPool = 30
	freeTopicVocabPool    = 40
	topicMatchLimit       = 2
	referenceGrammarSlice = 8
	maxPromptTopicLen     = 120
	referencePageLimit    = 1000
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
	var pool bool
	if request.includeReference {
		vocab, grammar, pool, err = s.referenceSlice(ctx, request)
		if err != nil {
			return inbound.LessonPrompt{}, err
		}
	}

	return inbound.LessonPrompt{Prompt: composePrompt(request, vocab, grammar, pool)}, nil
}

type promptRequest struct {
	owner            string
	language         domain.Language
	level            domain.Level
	topic            string
	topicSlug        reference.TopicTag
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
		owner:            strings.TrimSpace(query.Owner),
		language:         domain.Language(query.Language),
		level:            domain.Level(strings.ToUpper(query.Level)),
		topic:            strings.TrimSpace(query.Topic),
		stage:            domain.ReadingStage(query.ReadingStage),
		length:           query.Length,
		includeReference: query.IncludeReference,
	}

	if slug := strings.TrimSpace(query.TopicSlug); slug != "" {
		tag, err := reference.NewTopicTag(slug)
		if err != nil {
			add("topicSlug", "must contain only lowercase letters, digits, and hyphens")
		} else {
			request.topicSlug = tag
		}
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
			request.exerciseTypes = []domain.ExerciseType{domain.TypeScriptPractice, domain.TypeMatching, domain.TypeMultipleChoice, domain.TypeCloze}
		}
	}

	if len(issues) > 0 {
		return promptRequest{}, &domain.ValidationError{Issues: issues}
	}
	return request, nil
}

func (s *Service) referenceSlice(ctx context.Context, request promptRequest) ([]reference.VocabEntry, []reference.GrammarTopic, bool, error) {
	lang, err := reference.NewLanguage(string(request.language))
	if err != nil {
		return nil, nil, false, err
	}
	level, err := reference.NewLevel(string(request.level))
	if err != nil {
		return nil, nil, false, err
	}

	vocab, pool, err := s.vocabSlice(ctx, request, lang, level)
	if err != nil {
		return nil, nil, false, err
	}
	grammar, err := s.grammarSlice(ctx, request, lang, level)
	if err != nil {
		return nil, nil, false, err
	}
	return vocab, grammar, pool, nil
}

func (s *Service) vocabSlice(ctx context.Context, request promptRequest, lang reference.Language, level reference.Level) ([]reference.VocabEntry, bool, error) {
	covered, err := s.coveredIDs(ctx, request.owner, string(request.language), progress.KindVocab)
	if err != nil {
		return nil, false, err
	}

	if request.topicSlug != "" {
		topic, err := s.topicBySlug(ctx, lang, level, request.topicSlug)
		if err != nil {
			return nil, false, err
		}
		ids := selectUncoveredFirst(topic.VocabIDs, func(id string) string { return id }, covered, referenceVocabSlice)
		entries, err := s.reader.VocabByIDs(ctx, lang, ids)
		return entries, false, err
	}

	if request.topic != "" {
		if ids, err := s.semantic.SimilarVocabIDs(ctx, lang, level, request.topic, freeTopicVocabPool); err == nil && len(ids) > 0 {
			ids = selectUncoveredFirst(ids, func(id string) string { return id }, covered, matchedTopicVocabPool)
			entries, err := s.reader.VocabByIDs(ctx, lang, ids)
			return entries, true, err
		}

		topics, err := s.reader.Topics(ctx, outbound.TopicFilter{Language: lang, Level: level})
		if err != nil {
			return nil, false, err
		}
		if matched := matchTopics(request.topic, topics); len(matched) > 0 {
			ids := selectUncoveredFirst(topicVocabIDs(matched), func(id string) string { return id }, covered, matchedTopicVocabPool)
			entries, err := s.reader.VocabByIDs(ctx, lang, ids)
			return entries, true, err
		}
	}

	var entries []reference.VocabEntry
	cursor := ""
	for {
		page, err := s.reader.Vocab(ctx, outbound.VocabFilter{Language: lang, Level: level, Limit: referencePageLimit, Cursor: cursor})
		if err != nil {
			return nil, false, err
		}
		entries = append(entries, page.Entries...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if request.topic != "" {
		return selectUncoveredFirst(entries, func(e reference.VocabEntry) string { return e.ID }, covered, freeTopicVocabPool), true, nil
	}
	return selectUncoveredFirst(entries, func(e reference.VocabEntry) string { return e.ID }, covered, referenceVocabSlice), false, nil
}

func matchTopics(text string, topics []reference.Topic) []reference.Topic {
	tokens := topicTokens(text)
	if len(tokens) == 0 {
		return nil
	}
	type scoredTopic struct {
		topic reference.Topic
		score int
	}
	var scored []scoredTopic
	for _, topic := range topics {
		keywords := make(map[string]bool, len(topic.Keywords))
		for _, keyword := range topic.Keywords {
			keywords[strings.ToLower(keyword)] = true
		}
		score := 0
		for _, token := range tokens {
			if keywords[token] || keywords[strings.TrimSuffix(token, "s")] {
				score++
			}
		}
		if score > 0 {
			scored = append(scored, scoredTopic{topic: topic, score: score})
		}
	}
	slices.SortStableFunc(scored, func(a, b scoredTopic) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return strings.Compare(string(a.topic.Slug), string(b.topic.Slug))
	})
	matched := make([]reference.Topic, 0, topicMatchLimit)
	for _, entry := range scored[:min(len(scored), topicMatchLimit)] {
		matched = append(matched, entry.topic)
	}
	return matched
}

func topicTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if utf8.RuneCountInString(field) >= 3 {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func topicVocabIDs(topics []reference.Topic) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, topic := range topics {
		for _, id := range topic.VocabIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (s *Service) grammarSlice(ctx context.Context, request promptRequest, lang reference.Language, level reference.Level) ([]reference.GrammarTopic, error) {
	covered, err := s.coveredIDs(ctx, request.owner, string(request.language), progress.KindGrammar)
	if err != nil {
		return nil, err
	}

	var topics []reference.GrammarTopic
	cursor := ""
	for {
		page, err := s.reader.Grammar(ctx, outbound.GrammarFilter{Language: lang, Level: level, Limit: referencePageLimit, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		topics = append(topics, page.Topics...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return selectUncoveredFirst(topics, func(t reference.GrammarTopic) string { return t.ID }, covered, referenceGrammarSlice), nil
}

func (s *Service) topicBySlug(ctx context.Context, lang reference.Language, level reference.Level, slug reference.TopicTag) (reference.Topic, error) {
	topics, err := s.reader.Topics(ctx, outbound.TopicFilter{Language: lang, Level: level, Slug: slug})
	if err != nil {
		return reference.Topic{}, err
	}
	for _, topic := range topics {
		if topic.Slug == slug {
			return topic, nil
		}
	}
	return reference.Topic{}, &domain.ValidationError{Issues: []domain.Issue{{
		Path:    "topicSlug",
		Message: fmt.Sprintf("no topic %q exists for level %s", slug, level),
	}}}
}

func (s *Service) coveredIDs(ctx context.Context, owner, language string, kind progress.ItemKind) (map[string]bool, error) {
	if owner == "" {
		return nil, nil
	}
	ids, err := s.coverage.CoveredItemIDs(ctx, owner, language, kind)
	if err != nil {
		return nil, err
	}
	covered := make(map[string]bool, len(ids))
	for _, id := range ids {
		covered[id] = true
	}
	return covered, nil
}

func selectUncoveredFirst[T any](items []T, id func(T) string, covered map[string]bool, limit int) []T {
	selected := make([]T, 0, min(limit, len(items)))
	for _, item := range items {
		if len(selected) == limit {
			return selected
		}
		if !covered[id(item)] {
			selected = append(selected, item)
		}
	}
	for _, item := range items {
		if len(selected) == limit {
			return selected
		}
		if covered[id(item)] {
			selected = append(selected, item)
		}
	}
	return selected
}

func composePrompt(request promptRequest, vocab []reference.VocabEntry, grammar []reference.GrammarTopic, pool bool) string {
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
	has := func(t domain.ExerciseType) bool { return slices.Contains(request.exerciseTypes, t) }

	fmt.Fprintf(&b, "You are generating a lesson for Langler, a personal language-learning app. Follow every instruction exactly and return only JSON.\n\n")
	fmt.Fprintf(&b, "## Lesson parameters\n")
	fmt.Fprintf(&b, "- Language: %s (%s)\n", languageNames[request.language], request.language)
	fmt.Fprintf(&b, "- Level: %s\n", levelLabel)
	if request.topic != "" {
		fmt.Fprintf(&b, "- Topic: %s\n", request.topic)
	}
	fmt.Fprintf(&b, "- Reading stage: %s\n", request.stage)
	fmt.Fprintf(&b, "- Exercise types to use: %s\n", strings.Join(types, ", "))
	fmt.Fprintf(&b, "  Use only these exercise types. Every exercise's \"type\" must be one of them; never add an exercise of any other type.\n")
	fmt.Fprintf(&b, "- Length: %s (%s)\n\n", request.length, lengthGuidance[request.length])

	if request.stage == domain.StageConnected {
		fmt.Fprintf(&b, "## Story requirement\n")
		fmt.Fprintf(&b, "The lesson must open with a \"reading\" exercise whose payload has \"genre\": \"short_story\". It is the first exercise in the array: the learner meets the lesson's language there before anything tests it.\n")
		fmt.Fprintf(&b, "- Write a short, original story appropriate for %s: a coherent miniature narrative on the topic, not disconnected sentences.\n", levelLabel)
		fmt.Fprintf(&b, "- Work every target vocabulary and grammar item into the story in natural context and introduce as little language outside the reference list as possible.\n")
		fmt.Fprintf(&b, "- Annotate every target vocabulary word in \"annotations\" with its reading and gloss so the learner can decode it on first contact.\n")
		fmt.Fprintf(&b, "- Give the story a title and 2-4 multiple-choice comprehension questions answerable from the passage alone.\n\n")
		if request.language == "pl" {
			fmt.Fprintf(&b, "- Write idiomatic contemporary Polish, not sentence-by-sentence English calques. Keep function words and inflection natural while maximizing coverage by the supplied CEFR-banded vocabulary; treat the CEFR labels as frequency-based approximations.\n")
			fmt.Fprintf(&b, "- Keep at least 85%% of content-word occurrences within the selected level or easier. A higher-level word is allowed only when the story cannot remain natural without it, and should be annotated.\n\n")
		}
		if request.language == "my" {
			fmt.Fprintf(&b, "- Write natural contemporary Burmese in canonical Unicode, never Zawgyi. Treat the CEFR labels as approximate frequency bands, and keep at least 85%% of segmented content words at the selected band or easier.\n")
			fmt.Fprintf(&b, "- Put the supplied Hybrid Burmese romanization in annotation readings. Annotate each new word once; do not insert romanization into the Burmese passage itself.\n\n")
		}
		fmt.Fprintf(&b, "## Teaching flow\n")
		fmt.Fprintf(&b, "This is the learner's first encounter with the material; the lesson teaches, it does not quiz prior knowledge.\n")
		fmt.Fprintf(&b, "- After the story, order the exercises you include from recognition to production, using only the selected types: recognition (matching, multiple choice, script practice) before controlled production (cloze, ordering) before free production (translation, writing). Skip any of these the selection does not include; never add one to complete the sequence.\n")
		fmt.Fprintf(&b, "- Never require a word or pattern that the story or an earlier exercise has not already introduced.\n")
		fmt.Fprintf(&b, "- Give every cloze blank in the first half of the lesson a \"hint\"; later exercises may drop hints as the learner warms up.\n")
		fmt.Fprintf(&b, "- Keep the first exercises after the story answerable straight from the story context and raise the difficulty gradually toward free production at the end.\n\n")
	} else {
		fmt.Fprintf(&b, "## Foundational lesson\n")
		fmt.Fprintf(&b, "This learner cannot decode connected text yet. Do not include any \"reading\" exercise or story.\n")
		fmt.Fprintf(&b, "Focus on script practice, individual words, and very short sentences that build glyph recognition and sound-script associations.\n")
		fmt.Fprintf(&b, "Introduce every glyph or word first (script practice or matching that shows its reading and meaning) before any exercise asks the learner to recall it, and raise the difficulty gradually.\n")
		if request.language == "my" {
			fmt.Fprintf(&b, "Use script_practice items with a Burmese character or short legal syllable, its Hybrid Burmese romanization, and its sound or meaning. Store canonical Unicode and never emit an unattached combining mark inside a word.\n")
		}
		fmt.Fprintf(&b, "Set \"readingStage\": \"foundational\" in the output.\n\n")
	}

	if len(vocab) > 0 || len(grammar) > 0 {
		fmt.Fprintf(&b, "## Reference data\n")
		fmt.Fprintf(&b, "Ground the lesson in these vetted items. When an exercise uses one, put its id in that exercise's referencedVocab or referencedGrammar array. Use only ids listed here; never invent ids.\n")
		if pool {
			fmt.Fprintf(&b, "The vocabulary list is a candidate pool, larger than the lesson needs: pick roughly %d items that genuinely fit the topic, build the lesson on those, and leave the rest out. Reference only the items you actually use; never force an unrelated word in just because it is listed.\n", referenceVocabSlice)
		}
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
	fmt.Fprintf(&b, "exerciseId values must be unique. Payload shapes for the allowed types (only these types may appear):\n")
	if has(domain.TypeCloze) {
		fmt.Fprintf(&b, "- cloze: {\"text\": \"sentence with {{1}} and {{2}} markers\", \"blanks\": [{\"index\": 1, \"answer\": \"...\", \"hint\": \"optional\"}], \"wordBank\": [\"...\"]} - every {{n}} marker needs exactly one blank with that index. Always include a wordBank: every blank's answer plus 3-6 plausible same-level distractors, in random order, so the learner selects instead of typing.\n")
	}
	if has(domain.TypeTranslation) {
		fmt.Fprintf(&b, "- translation: {\"source\": \"<sentence in %s>\", \"reference\": \"<English translation>\"}\n", languageNames[request.language])
	}
	if has(domain.TypeOrdering) {
		fmt.Fprintf(&b, "- ordering: {\"items\": [\"...\", \"...\"], \"translation\": \"optional\"} - list 2-20 items in the CORRECT order; the app shuffles them for the learner.\n")
	}
	if has(domain.TypeMatching) {
		fmt.Fprintf(&b, "- matching: {\"pairs\": [{\"left\": \"<target language>\", \"right\": \"<meaning>\"}]} - 2-20 pairs.\n")
	}
	if has(domain.TypeMultipleChoice) {
		fmt.Fprintf(&b, "- multiple_choice: {\"questions\": [{\"question\": \"...\", \"options\": [\"...\", \"...\", \"...\", \"...\"], \"answer\": \"<must exactly equal one option>\"}]} - 1-10 questions, each with 3-4 options and exactly one correct answer. Make distractors plausible: same word class and level, wrong in meaning or usage.\n")
	}
	if has(domain.TypeReading) {
		fmt.Fprintf(&b, "- reading: {\"genre\": \"short_story\", \"title\": \"...\", \"passage\": \"...\", \"annotations\": [{\"surface\": \"...\", \"reading\": \"...\", \"gloss\": \"...\"}], \"questions\": [{\"question\": \"...\", \"kind\": \"multiple_choice\", \"options\": [\"...\"], \"answer\": \"<must equal one option>\"}]} - comprehension questions must use \"kind\": \"multiple_choice\" so the app can grade them; only use {\"kind\": \"short_answer\", \"answer\": \"...\", \"alternates\": [\"...\"]} when a question genuinely cannot be closed-form, and then list every accepted phrasing in alternates.\n")
	}
	if has(domain.TypeWritingPrompt) {
		fmt.Fprintf(&b, "- writing_prompt: put the writing task in the exercise's \"prompt\"; payload is optional: {\"guidance\": \"...\", \"modelAnswer\": \"...\"}\n")
	}
	if has(domain.TypeScriptPractice) {
		if request.language == "pl" {
			fmt.Fprintf(&b, "- script_practice: Polish orthography only. Use choice items {\"kind\": \"choice\", \"glyph\": \"<sentence or cue with a blank>\", \"meaning\": \"<brief rule hint>\", \"options\": [\"<correct spelling>\", \"<plausible contrast>\"], \"answer\": \"<must exactly equal one option>\"} and dictation-style recall items {\"kind\": \"dictation\", \"glyph\": \"<definition or cloze cue; no audio>\", \"meaning\": \"<optional contrast hint>\", \"answer\": \"<correct Polish word>\"}. Exercise ó/u, rz/ż, ch/h, Polish diacritics, or digraphs; never use tracing or stroke-order tasks.\n")
		} else {
			fmt.Fprintf(&b, "- script_practice: {\"items\": [{\"glyph\": \"<character or short word>\", \"reading\": \"...\", \"meaning\": \"...\"}]}\n")
		}
	}
	fmt.Fprintf(&b, "\n")
	if request.language == "my" {
		fmt.Fprintf(&b, "- Every Burmese string must use canonical Unicode, never Zawgyi, and must contain only orthographically legal medial, vowel, tone, asat, kinzi, and virama combinations.\n")
		fmt.Fprintf(&b, "- Reading annotations and script-practice readings use the supplied Hybrid Burmese romanization exactly; romanization is a learner-controlled aid, not part of Burmese running text.\n")
	}
	fmt.Fprintf(&b, "## Constraints\n")
	fmt.Fprintf(&b, "- Plain text only in every string: no HTML, no markdown, no control characters.\n")
	if request.language == "ja" {
		fmt.Fprintf(&b, "- All Japanese content (cloze texts, reading passages, translation sources, ordering items, practice glyphs) must be written in Japanese script.\n")
	}
	if request.language == "pl" {
		fmt.Fprintf(&b, "- Preserve Polish diacritics exactly (ą, ć, ę, ł, ń, ó, ś, ź, ż) in stories, answers, annotations, and spelling options.\n")
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
