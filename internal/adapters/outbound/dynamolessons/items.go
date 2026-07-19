package dynamolessons

import (
	"time"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
)

type lessonItem struct {
	PK               string         `dynamodbav:"PK"`
	SK               string         `dynamodbav:"SK"`
	ContentHash      string         `dynamodbav:"contentHash"`
	CreatedAt        string         `dynamodbav:"createdAt"`
	SchemaVersion    string         `dynamodbav:"schemaVersion"`
	LessonID         string         `dynamodbav:"lessonId"`
	Language         string         `dynamodbav:"language"`
	Level            string         `dynamodbav:"level"`
	Title            string         `dynamodbav:"title"`
	Description      string         `dynamodbav:"description,omitempty"`
	Topic            string         `dynamodbav:"topic,omitempty"`
	Tags             []string       `dynamodbav:"tags,omitempty"`
	ReadingStage     string         `dynamodbav:"readingStage"`
	SourceModel      string         `dynamodbav:"sourceModel,omitempty"`
	EstimatedMinutes int            `dynamodbav:"estimatedMinutes,omitempty"`
	Exercises        []exerciseItem `dynamodbav:"exercises"`
}

type exerciseItem struct {
	ExerciseID        string              `dynamodbav:"exerciseId"`
	Type              string              `dynamodbav:"type"`
	Prompt            string              `dynamodbav:"prompt,omitempty"`
	Points            int                 `dynamodbav:"points,omitempty"`
	ReferencedVocab   []string            `dynamodbav:"referencedVocab,omitempty"`
	ReferencedGrammar []string            `dynamodbav:"referencedGrammar,omitempty"`
	Cloze             *clozeItem          `dynamodbav:"cloze,omitempty"`
	Translation       *translationItem    `dynamodbav:"translation,omitempty"`
	Ordering          *orderingItem       `dynamodbav:"ordering,omitempty"`
	Matching          *matchingItem       `dynamodbav:"matching,omitempty"`
	MultipleChoice    *multipleChoiceItem `dynamodbav:"multipleChoice,omitempty"`
	Reading           *readingItem        `dynamodbav:"reading,omitempty"`
	WritingPrompt     *writingPromptItem  `dynamodbav:"writingPrompt,omitempty"`
	ScriptPractice    *scriptPracticeItem `dynamodbav:"scriptPractice,omitempty"`
}

type clozeItem struct {
	Text     string      `dynamodbav:"text"`
	Blanks   []blankItem `dynamodbav:"blanks"`
	WordBank []string    `dynamodbav:"wordBank,omitempty"`
}

type blankItem struct {
	Index      int      `dynamodbav:"index"`
	Answer     string   `dynamodbav:"answer"`
	Alternates []string `dynamodbav:"alternates,omitempty"`
	Hint       string   `dynamodbav:"hint,omitempty"`
}

type resultItem struct {
	PK          string               `dynamodbav:"PK"`
	SK          string               `dynamodbav:"SK"`
	AttemptID   string               `dynamodbav:"attemptId"`
	LessonID    string               `dynamodbav:"lessonId"`
	StartedAt   string               `dynamodbav:"startedAt"`
	CompletedAt string               `dynamodbav:"completedAt"`
	Score       int                  `dynamodbav:"score"`
	MaxScore    int                  `dynamodbav:"maxScore"`
	AutoScore   int                  `dynamodbav:"autoScore"`
	AutoMax     int                  `dynamodbav:"autoMax"`
	SelfScore   int                  `dynamodbav:"selfScore"`
	SelfMax     int                  `dynamodbav:"selfMax"`
	Exercises   []exerciseResultItem `dynamodbav:"exercises"`
}

type exerciseResultItem struct {
	ExerciseID string `dynamodbav:"exerciseId"`
	Type       string `dynamodbav:"type"`
	Grading    string `dynamodbav:"grading"`
	Score      int    `dynamodbav:"score"`
	MaxScore   int    `dynamodbav:"maxScore"`
	Correct    int    `dynamodbav:"correct"`
	Total      int    `dynamodbav:"total"`
}

type translationItem struct {
	Source    string `dynamodbav:"source"`
	Reference string `dynamodbav:"reference,omitempty"`
}

type orderingItem struct {
	Items       []string `dynamodbav:"items"`
	Translation string   `dynamodbav:"translation,omitempty"`
}

type matchingItem struct {
	Pairs []pairItem `dynamodbav:"pairs"`
}

type pairItem struct {
	Left  string `dynamodbav:"left"`
	Right string `dynamodbav:"right"`
}

type multipleChoiceItem struct {
	Questions []mcQuestionItem `dynamodbav:"questions"`
}

type mcQuestionItem struct {
	Question string   `dynamodbav:"question"`
	Options  []string `dynamodbav:"options"`
	Answer   string   `dynamodbav:"answer"`
}

type readingItem struct {
	Genre       string           `dynamodbav:"genre"`
	Title       string           `dynamodbav:"title"`
	Passage     string           `dynamodbav:"passage"`
	Annotations []annotationItem `dynamodbav:"annotations,omitempty"`
	Questions   []questionItem   `dynamodbav:"questions"`
}

type annotationItem struct {
	Surface string `dynamodbav:"surface"`
	Reading string `dynamodbav:"reading,omitempty"`
	Gloss   string `dynamodbav:"gloss,omitempty"`
}

type questionItem struct {
	Question   string   `dynamodbav:"question"`
	Kind       string   `dynamodbav:"kind"`
	Options    []string `dynamodbav:"options,omitempty"`
	Answer     string   `dynamodbav:"answer,omitempty"`
	Alternates []string `dynamodbav:"alternates,omitempty"`
}

type writingPromptItem struct {
	Guidance    string `dynamodbav:"guidance,omitempty"`
	ModelAnswer string `dynamodbav:"modelAnswer,omitempty"`
}

type scriptPracticeItem struct {
	Items []scriptGlyphItem `dynamodbav:"items"`
}

type scriptGlyphItem struct {
	Glyph   string   `dynamodbav:"glyph,omitempty"`
	Reading string   `dynamodbav:"reading,omitempty"`
	Meaning string   `dynamodbav:"meaning,omitempty"`
	Kind    string   `dynamodbav:"kind,omitempty"`
	Answer  string   `dynamodbav:"answer,omitempty"`
	Options []string `dynamodbav:"options,omitempty"`
}

func toItem(owner string, contentHash string, createdAt time.Time, l domain.Lesson) lessonItem {
	item := lessonItem{
		PK:               "USER#" + owner,
		SK:               "LESSON#" + l.ID,
		ContentHash:      contentHash,
		CreatedAt:        createdAt.UTC().Format(time.RFC3339),
		SchemaVersion:    l.SchemaVersion,
		LessonID:         l.ID,
		Language:         string(l.Language),
		Level:            string(l.Level),
		Title:            l.Title,
		Description:      l.Description,
		Topic:            l.Topic,
		Tags:             l.Tags,
		ReadingStage:     string(l.ReadingStage),
		SourceModel:      l.SourceModel,
		EstimatedMinutes: l.EstimatedMinutes,
	}
	for _, exercise := range l.Exercises {
		item.Exercises = append(item.Exercises, toExerciseItem(exercise))
	}
	return item
}

func toExerciseItem(e domain.Exercise) exerciseItem {
	item := exerciseItem{
		ExerciseID:        e.ID,
		Type:              string(e.Type),
		Prompt:            e.Prompt,
		Points:            e.Points,
		ReferencedVocab:   e.ReferencedVocab,
		ReferencedGrammar: e.ReferencedGrammar,
	}
	if e.Cloze != nil {
		blanks := make([]blankItem, 0, len(e.Cloze.Blanks))
		for _, blank := range e.Cloze.Blanks {
			blanks = append(blanks, blankItem(blank))
		}
		item.Cloze = &clozeItem{Text: e.Cloze.Text, Blanks: blanks, WordBank: e.Cloze.WordBank}
	}
	if e.Translation != nil {
		item.Translation = &translationItem{Source: e.Translation.Source, Reference: e.Translation.Reference}
	}
	if e.Ordering != nil {
		item.Ordering = &orderingItem{Items: e.Ordering.Items, Translation: e.Ordering.Translation}
	}
	if e.Matching != nil {
		pairs := make([]pairItem, 0, len(e.Matching.Pairs))
		for _, pair := range e.Matching.Pairs {
			pairs = append(pairs, pairItem(pair))
		}
		item.Matching = &matchingItem{Pairs: pairs}
	}
	if e.MultipleChoice != nil {
		questions := make([]mcQuestionItem, 0, len(e.MultipleChoice.Questions))
		for _, question := range e.MultipleChoice.Questions {
			questions = append(questions, mcQuestionItem(question))
		}
		item.MultipleChoice = &multipleChoiceItem{Questions: questions}
	}
	if e.Reading != nil {
		annotations := make([]annotationItem, 0, len(e.Reading.Annotations))
		for _, annotation := range e.Reading.Annotations {
			annotations = append(annotations, annotationItem(annotation))
		}
		questions := make([]questionItem, 0, len(e.Reading.Questions))
		for _, question := range e.Reading.Questions {
			questions = append(questions, questionItem(question))
		}
		item.Reading = &readingItem{
			Genre:       e.Reading.Genre,
			Title:       e.Reading.Title,
			Passage:     e.Reading.Passage,
			Annotations: annotations,
			Questions:   questions,
		}
	}
	if e.WritingPrompt != nil {
		item.WritingPrompt = &writingPromptItem{Guidance: e.WritingPrompt.Guidance, ModelAnswer: e.WritingPrompt.ModelAnswer}
	}
	if e.ScriptPractice != nil {
		items := make([]scriptGlyphItem, 0, len(e.ScriptPractice.Items))
		for _, glyph := range e.ScriptPractice.Items {
			items = append(items, scriptGlyphItem{
				Glyph: glyph.Glyph, Reading: glyph.Reading, Meaning: glyph.Meaning,
				Kind: glyph.Kind, Answer: glyph.Answer, Options: glyph.Options,
			})
		}
		item.ScriptPractice = &scriptPracticeItem{Items: items}
	}
	return item
}

func (item lessonItem) toDomain() (domain.Lesson, time.Time, string) {
	l := domain.Lesson{
		SchemaVersion:    item.SchemaVersion,
		ID:               item.LessonID,
		Language:         domain.Language(item.Language),
		Level:            domain.Level(item.Level),
		Title:            item.Title,
		Description:      item.Description,
		Topic:            item.Topic,
		Tags:             item.Tags,
		ReadingStage:     domain.ReadingStage(item.ReadingStage),
		SourceModel:      item.SourceModel,
		EstimatedMinutes: item.EstimatedMinutes,
	}
	for _, exercise := range item.Exercises {
		l.Exercises = append(l.Exercises, exercise.toDomain())
	}
	createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
	return l, createdAt, item.ContentHash
}

func (item exerciseItem) toDomain() domain.Exercise {
	e := domain.Exercise{
		ID:                item.ExerciseID,
		Type:              domain.ExerciseType(item.Type),
		Prompt:            item.Prompt,
		Points:            item.Points,
		ReferencedVocab:   item.ReferencedVocab,
		ReferencedGrammar: item.ReferencedGrammar,
	}
	if item.Cloze != nil {
		blanks := make([]domain.Blank, 0, len(item.Cloze.Blanks))
		for _, blank := range item.Cloze.Blanks {
			blanks = append(blanks, domain.Blank(blank))
		}
		e.Cloze = &domain.Cloze{Text: item.Cloze.Text, Blanks: blanks, WordBank: item.Cloze.WordBank}
	}
	if item.Translation != nil {
		e.Translation = &domain.Translation{Source: item.Translation.Source, Reference: item.Translation.Reference}
	}
	if item.Ordering != nil {
		e.Ordering = &domain.Ordering{Items: item.Ordering.Items, Translation: item.Ordering.Translation}
	}
	if item.Matching != nil {
		pairs := make([]domain.Pair, 0, len(item.Matching.Pairs))
		for _, pair := range item.Matching.Pairs {
			pairs = append(pairs, domain.Pair(pair))
		}
		e.Matching = &domain.Matching{Pairs: pairs}
	}
	if item.MultipleChoice != nil {
		questions := make([]domain.MCQuestion, 0, len(item.MultipleChoice.Questions))
		for _, question := range item.MultipleChoice.Questions {
			questions = append(questions, domain.MCQuestion(question))
		}
		e.MultipleChoice = &domain.MultipleChoice{Questions: questions}
	}
	if item.Reading != nil {
		annotations := make([]domain.Annotation, 0, len(item.Reading.Annotations))
		for _, annotation := range item.Reading.Annotations {
			annotations = append(annotations, domain.Annotation(annotation))
		}
		questions := make([]domain.Question, 0, len(item.Reading.Questions))
		for _, question := range item.Reading.Questions {
			questions = append(questions, domain.Question(question))
		}
		e.Reading = &domain.Reading{
			Genre:       item.Reading.Genre,
			Title:       item.Reading.Title,
			Passage:     item.Reading.Passage,
			Annotations: annotations,
			Questions:   questions,
		}
	}
	if item.WritingPrompt != nil {
		e.WritingPrompt = &domain.WritingPrompt{Guidance: item.WritingPrompt.Guidance, ModelAnswer: item.WritingPrompt.ModelAnswer}
	}
	if item.ScriptPractice != nil {
		items := make([]domain.ScriptItem, 0, len(item.ScriptPractice.Items))
		for _, glyph := range item.ScriptPractice.Items {
			items = append(items, domain.ScriptItem{
				Glyph: glyph.Glyph, Reading: glyph.Reading, Meaning: glyph.Meaning,
				Kind: glyph.Kind, Answer: glyph.Answer, Options: glyph.Options,
			})
		}
		e.ScriptPractice = &domain.ScriptPractice{Items: items}
	}
	return e
}
