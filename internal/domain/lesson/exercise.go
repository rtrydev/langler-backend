package lesson

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type ExerciseType string

const (
	TypeCloze          ExerciseType = "cloze"
	TypeTranslation    ExerciseType = "translation"
	TypeOrdering       ExerciseType = "ordering"
	TypeMatching       ExerciseType = "matching"
	TypeMultipleChoice ExerciseType = "multiple_choice"
	TypeReading        ExerciseType = "reading"
	TypeWritingPrompt  ExerciseType = "writing_prompt"
	TypeScriptPractice ExerciseType = "script_practice"
)

const GenreShortStory = "short_story"

func ExerciseTypes() []ExerciseType {
	return []ExerciseType{
		TypeCloze, TypeTranslation, TypeOrdering, TypeMatching,
		TypeMultipleChoice, TypeReading, TypeWritingPrompt, TypeScriptPractice,
	}
}

func KnownExerciseType(t ExerciseType) bool {
	return slices.Contains(ExerciseTypes(), t)
}

type Exercise struct {
	ID                string
	Type              ExerciseType
	Prompt            string
	Points            int
	ReferencedVocab   []string
	ReferencedGrammar []string
	Cloze             *Cloze
	Translation       *Translation
	Ordering          *Ordering
	Matching          *Matching
	MultipleChoice    *MultipleChoice
	Reading           *Reading
	WritingPrompt     *WritingPrompt
	ScriptPractice    *ScriptPractice
}

type Cloze struct {
	Text     string
	Blanks   []Blank
	WordBank []string
}

type Blank struct {
	Index      int
	Answer     string
	Alternates []string
	Hint       string
}

type Translation struct {
	Source    string
	Reference string
}

type Ordering struct {
	Items       []string
	Translation string
}

type Matching struct {
	Pairs []Pair
}

type MultipleChoice struct {
	Questions []MCQuestion
}

type MCQuestion struct {
	Question string
	Options  []string
	Answer   string
}

type Pair struct {
	Left  string
	Right string
}

type Reading struct {
	Genre       string
	Title       string
	Passage     string
	Annotations []Annotation
	Questions   []Question
}

type Annotation struct {
	Surface string
	Reading string
	Gloss   string
}

const (
	KindMultipleChoice = "multiple_choice"
	KindShortAnswer    = "short_answer"
)

type Question struct {
	Question   string
	Kind       string
	Options    []string
	Answer     string
	Alternates []string
}

type WritingPrompt struct {
	Guidance    string
	ModelAnswer string
}

type ScriptPractice struct {
	Items []ScriptItem
}

type ScriptItem struct {
	Glyph   string
	Reading string
	Meaning string
}

const (
	maxExerciseIDLen    = 64
	maxPromptLen        = 500
	maxPoints           = 100
	maxReferencesPerSet = 30
	maxClozeTextLen     = 3000
	maxBlanks           = 20
	maxWordBankEntries  = 40
	maxAnswerLen        = 120
	maxAlternateAnswers = 10
	maxHintLen          = 200
	maxSentenceLen      = 1000
	maxOrderingItems    = 20
	maxOrderingItemLen  = 200
	maxPairs            = 20
	maxPairSideLen      = 120
	maxStoryTitleLen    = 160
	maxPassageLen       = 6000
	maxAnnotations      = 100
	maxSurfaceLen       = 60
	maxGlossLen         = 200
	maxQuestions        = 10
	maxQuestionLen      = 300
	maxOptions          = 6
	maxOptionLen        = 200
	maxShortAnswerLen   = 400
	maxGuidanceLen      = 1000
	maxModelAnswerLen   = 2000
	maxScriptItems      = 30
	maxGlyphLen         = 8
)

var (
	exerciseIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	referenceIDPattern = regexp.MustCompile(`^[A-Z0-9]{1,8}#[A-Za-z0-9_-]{1,64}$`)
	blankMarkerPattern = regexp.MustCompile(`\{\{(\d+)\}\}`)
)

func validateExercise(c *collector, path string, e *Exercise) {
	if !exerciseIDPattern.MatchString(e.ID) {
		c.add(path+".exerciseId", "must be 1-%d letters, digits, hyphens, or underscores", maxExerciseIDLen)
	}
	e.Prompt = strings.TrimSpace(e.Prompt)
	c.text(path+".prompt", e.Prompt, maxPromptLen, false)
	if e.Points < 0 || e.Points > maxPoints {
		c.add(path+".points", "must be between 0 and %d", maxPoints)
	}
	validateReferences(c, path+".referencedVocab", e.ReferencedVocab)
	validateReferences(c, path+".referencedGrammar", e.ReferencedGrammar)

	if !KnownExerciseType(e.Type) {
		c.add(path+".type", "unknown exercise type %q; valid types are %s", string(e.Type), joinValues(ExerciseTypes()))
		return
	}

	payload := path + ".payload"
	switch e.Type {
	case TypeCloze:
		if e.Cloze == nil {
			c.add(payload, "a %q exercise requires a payload with text and blanks", e.Type)
			return
		}
		validateCloze(c, payload, e.Cloze)
	case TypeTranslation:
		if e.Translation == nil {
			c.add(payload, "a %q exercise requires a payload with a source sentence", e.Type)
			return
		}
		validateTranslation(c, payload, e.Translation)
	case TypeOrdering:
		if e.Ordering == nil {
			c.add(payload, "an %q exercise requires a payload with items in the correct order", e.Type)
			return
		}
		validateOrdering(c, payload, e.Ordering)
	case TypeMatching:
		if e.Matching == nil {
			c.add(payload, "a %q exercise requires a payload with pairs", e.Type)
			return
		}
		validateMatching(c, payload, e.Matching)
	case TypeMultipleChoice:
		if e.MultipleChoice == nil {
			c.add(payload, "a %q exercise requires a payload with questions", e.Type)
			return
		}
		validateMultipleChoice(c, payload, e.MultipleChoice)
	case TypeReading:
		if e.Reading == nil {
			c.add(payload, "a %q exercise requires a payload with a genre, title, passage, and questions", e.Type)
			return
		}
		validateReading(c, payload, e.Reading)
	case TypeWritingPrompt:
		if e.Prompt == "" {
			c.add(path+".prompt", "must not be empty for a %q exercise", e.Type)
		}
		if e.WritingPrompt != nil {
			validateWritingPrompt(c, payload, e.WritingPrompt)
		}
	case TypeScriptPractice:
		if e.ScriptPractice == nil {
			c.add(payload, "a %q exercise requires a payload with glyph items", e.Type)
			return
		}
		validateScriptPractice(c, payload, e.ScriptPractice)
	}
}

func validateReferences(c *collector, path string, ids []string) {
	if len(ids) > maxReferencesPerSet {
		c.add(path, "must contain at most %d reference ids", maxReferencesPerSet)
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if !referenceIDPattern.MatchString(id) {
			c.add(fmt.Sprintf("%s[%d]", path, i), "must be a reference id of the form LEVEL#id, for example N4#1311125")
			continue
		}
		if seen[id] {
			c.add(fmt.Sprintf("%s[%d]", path, i), "duplicates reference id %q", id)
		}
		seen[id] = true
	}
}

func validateCloze(c *collector, path string, p *Cloze) {
	c.text(path+".text", p.Text, maxClozeTextLen, true)
	if len(p.Blanks) == 0 {
		c.add(path+".blanks", "must contain at least one blank")
	}
	if len(p.Blanks) > maxBlanks {
		c.add(path+".blanks", "must contain at most %d blanks", maxBlanks)
	}

	if len(p.WordBank) > maxWordBankEntries {
		c.add(path+".wordBank", "must contain at most %d entries", maxWordBankEntries)
	}
	bank := map[string]bool{}
	for i := range p.WordBank {
		p.WordBank[i] = strings.TrimSpace(p.WordBank[i])
		entryPath := fmt.Sprintf("%s.wordBank[%d]", path, i)
		c.text(entryPath, p.WordBank[i], maxAnswerLen, true)
		if bank[p.WordBank[i]] {
			c.add(entryPath, "duplicates word bank entry %q", p.WordBank[i])
		}
		bank[p.WordBank[i]] = true
	}

	markers := map[int]int{}
	for _, match := range blankMarkerPattern.FindAllStringSubmatch(p.Text, -1) {
		index, err := strconv.Atoi(match[1])
		if err == nil {
			markers[index]++
		}
	}

	declared := map[int]bool{}
	for i := range p.Blanks {
		blankPath := fmt.Sprintf("%s.blanks[%d]", path, i)
		b := &p.Blanks[i]
		if b.Index < 1 {
			c.add(blankPath+".index", "must be 1 or higher and match a {{n}} marker in the text")
			continue
		}
		if declared[b.Index] {
			c.add(blankPath+".index", "duplicates blank index %d", b.Index)
		}
		declared[b.Index] = true
		b.Answer = strings.TrimSpace(b.Answer)
		c.text(blankPath+".answer", b.Answer, maxAnswerLen, true)
		if len(p.WordBank) > 0 && b.Answer != "" && !bank[b.Answer] {
			c.add(blankPath+".answer", "must appear in the wordBank so the learner can select it")
		}
		if len(b.Alternates) > maxAlternateAnswers {
			c.add(blankPath+".alternates", "must contain at most %d alternate answers", maxAlternateAnswers)
		}
		for j, alternate := range b.Alternates {
			c.text(fmt.Sprintf("%s.alternates[%d]", blankPath, j), alternate, maxAnswerLen, true)
		}
		c.text(blankPath+".hint", b.Hint, maxHintLen, false)
		switch markers[b.Index] {
		case 0:
			c.add(blankPath+".index", "has no matching {{%d}} marker in the text", b.Index)
		case 1:
		default:
			c.add(blankPath+".index", "marker {{%d}} appears more than once in the text", b.Index)
		}
	}
	for index := range markers {
		if !declared[index] {
			c.add(path+".blanks", "marker {{%d}} in the text has no matching blank entry", index)
		}
	}
}

func validateTranslation(c *collector, path string, p *Translation) {
	p.Source = strings.TrimSpace(p.Source)
	c.text(path+".source", p.Source, maxSentenceLen, true)
	c.text(path+".reference", p.Reference, maxSentenceLen, false)
}

func validateOrdering(c *collector, path string, p *Ordering) {
	if len(p.Items) < 2 {
		c.add(path+".items", "must contain at least two items to order")
	}
	if len(p.Items) > maxOrderingItems {
		c.add(path+".items", "must contain at most %d items", maxOrderingItems)
	}
	for i := range p.Items {
		p.Items[i] = strings.TrimSpace(p.Items[i])
		c.text(fmt.Sprintf("%s.items[%d]", path, i), p.Items[i], maxOrderingItemLen, true)
	}
	c.text(path+".translation", p.Translation, maxPromptLen, false)
}

func validateMatching(c *collector, path string, p *Matching) {
	if len(p.Pairs) < 2 {
		c.add(path+".pairs", "must contain at least two pairs")
	}
	if len(p.Pairs) > maxPairs {
		c.add(path+".pairs", "must contain at most %d pairs", maxPairs)
	}
	for i := range p.Pairs {
		pair := &p.Pairs[i]
		pair.Left = strings.TrimSpace(pair.Left)
		pair.Right = strings.TrimSpace(pair.Right)
		c.text(fmt.Sprintf("%s.pairs[%d].left", path, i), pair.Left, maxPairSideLen, true)
		c.text(fmt.Sprintf("%s.pairs[%d].right", path, i), pair.Right, maxPairSideLen, true)
	}
}

func validateMultipleChoice(c *collector, path string, p *MultipleChoice) {
	if len(p.Questions) == 0 {
		c.add(path+".questions", "must contain at least one question")
	}
	if len(p.Questions) > maxQuestions {
		c.add(path+".questions", "must contain at most %d questions", maxQuestions)
	}
	for i := range p.Questions {
		q := &p.Questions[i]
		questionPath := fmt.Sprintf("%s.questions[%d]", path, i)
		q.Question = strings.TrimSpace(q.Question)
		c.text(questionPath+".question", q.Question, maxQuestionLen, true)
		if len(q.Options) < 2 || len(q.Options) > maxOptions {
			c.add(questionPath+".options", "must contain between 2 and %d options", maxOptions)
		}
		for j := range q.Options {
			q.Options[j] = strings.TrimSpace(q.Options[j])
			c.text(fmt.Sprintf("%s.options[%d]", questionPath, j), q.Options[j], maxOptionLen, true)
		}
		q.Answer = strings.TrimSpace(q.Answer)
		if q.Answer == "" {
			c.add(questionPath+".answer", "must not be empty")
		} else if !slices.Contains(q.Options, q.Answer) {
			c.add(questionPath+".answer", "must exactly match one of the options")
		}
	}
}

func validateReading(c *collector, path string, p *Reading) {
	if p.Genre != GenreShortStory {
		c.add(path+".genre", "must be %q", GenreShortStory)
	}
	p.Title = strings.TrimSpace(p.Title)
	c.text(path+".title", p.Title, maxStoryTitleLen, true)
	p.Passage = strings.TrimSpace(p.Passage)
	c.text(path+".passage", p.Passage, maxPassageLen, true)

	if len(p.Annotations) > maxAnnotations {
		c.add(path+".annotations", "must contain at most %d annotations", maxAnnotations)
	}
	for i := range p.Annotations {
		a := &p.Annotations[i]
		annotationPath := fmt.Sprintf("%s.annotations[%d]", path, i)
		c.text(annotationPath+".surface", a.Surface, maxSurfaceLen, true)
		c.text(annotationPath+".reading", a.Reading, maxSurfaceLen, false)
		c.text(annotationPath+".gloss", a.Gloss, maxGlossLen, false)
	}

	if len(p.Questions) == 0 {
		c.add(path+".questions", "must contain at least one comprehension question")
	}
	if len(p.Questions) > maxQuestions {
		c.add(path+".questions", "must contain at most %d questions", maxQuestions)
	}
	for i := range p.Questions {
		validateQuestion(c, fmt.Sprintf("%s.questions[%d]", path, i), &p.Questions[i])
	}
}

func validateQuestion(c *collector, path string, q *Question) {
	q.Question = strings.TrimSpace(q.Question)
	c.text(path+".question", q.Question, maxQuestionLen, true)
	switch q.Kind {
	case KindMultipleChoice:
		if len(q.Options) < 2 || len(q.Options) > maxOptions {
			c.add(path+".options", "must contain between 2 and %d options", maxOptions)
		}
		for i := range q.Options {
			q.Options[i] = strings.TrimSpace(q.Options[i])
			c.text(fmt.Sprintf("%s.options[%d]", path, i), q.Options[i], maxOptionLen, true)
		}
		q.Answer = strings.TrimSpace(q.Answer)
		if q.Answer == "" {
			c.add(path+".answer", "must not be empty for a %q question", KindMultipleChoice)
		} else if !slices.Contains(q.Options, q.Answer) {
			c.add(path+".answer", "must exactly match one of the options")
		}
		if len(q.Alternates) > 0 {
			c.add(path+".alternates", "must be omitted for a %q question", KindMultipleChoice)
		}
	case KindShortAnswer:
		if len(q.Options) > 0 {
			c.add(path+".options", "must be omitted for a %q question", KindShortAnswer)
		}
		c.text(path+".answer", q.Answer, maxShortAnswerLen, false)
		if len(q.Alternates) > maxAlternateAnswers {
			c.add(path+".alternates", "must contain at most %d alternate answers", maxAlternateAnswers)
		}
		if len(q.Alternates) > 0 && strings.TrimSpace(q.Answer) == "" {
			c.add(path+".alternates", "must be omitted when the question has no answer")
		}
		for i := range q.Alternates {
			q.Alternates[i] = strings.TrimSpace(q.Alternates[i])
			c.text(fmt.Sprintf("%s.alternates[%d]", path, i), q.Alternates[i], maxShortAnswerLen, true)
		}
	default:
		c.add(path+".kind", "must be %q or %q", KindMultipleChoice, KindShortAnswer)
	}
}

func validateWritingPrompt(c *collector, path string, p *WritingPrompt) {
	c.text(path+".guidance", p.Guidance, maxGuidanceLen, false)
	c.text(path+".modelAnswer", p.ModelAnswer, maxModelAnswerLen, false)
}

func validateScriptPractice(c *collector, path string, p *ScriptPractice) {
	if len(p.Items) == 0 {
		c.add(path+".items", "must contain at least one glyph item")
	}
	if len(p.Items) > maxScriptItems {
		c.add(path+".items", "must contain at most %d items", maxScriptItems)
	}
	for i := range p.Items {
		item := &p.Items[i]
		itemPath := fmt.Sprintf("%s.items[%d]", path, i)
		item.Glyph = strings.TrimSpace(item.Glyph)
		c.text(itemPath+".glyph", item.Glyph, maxGlyphLen, true)
		c.text(itemPath+".reading", item.Reading, maxAnswerLen, false)
		c.text(itemPath+".meaning", item.Meaning, maxAnswerLen, false)
	}
}
