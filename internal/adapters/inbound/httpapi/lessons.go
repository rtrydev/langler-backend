package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

const maxLessonBodyBytes = 256 * 1024

type issueDTO struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type validationResponse struct {
	Error  string     `json:"error"`
	Issues []issueDTO `json:"issues"`
}

type lessonDocument struct {
	SchemaVersion    string             `json:"schemaVersion"`
	LessonID         string             `json:"lessonId"`
	Language         string             `json:"language"`
	Level            string             `json:"level"`
	Title            string             `json:"title"`
	Description      string             `json:"description,omitempty"`
	Topic            string             `json:"topic,omitempty"`
	Tags             []string           `json:"tags,omitempty"`
	ReadingStage     string             `json:"readingStage"`
	SourceModel      string             `json:"sourceModel,omitempty"`
	EstimatedMinutes int                `json:"estimatedMinutes,omitempty"`
	Exercises        []exerciseDocument `json:"exercises"`
	Metadata         json.RawMessage    `json:"metadata,omitempty"`
}

type exerciseDocument struct {
	ExerciseID        string          `json:"exerciseId"`
	Type              string          `json:"type"`
	Prompt            string          `json:"prompt,omitempty"`
	Points            int             `json:"points,omitempty"`
	ReferencedVocab   []string        `json:"referencedVocab,omitempty"`
	ReferencedGrammar []string        `json:"referencedGrammar,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

type clozePayload struct {
	Text     string         `json:"text"`
	Blanks   []blankPayload `json:"blanks"`
	WordBank []string       `json:"wordBank,omitempty"`
}

type blankPayload struct {
	Index      int      `json:"index"`
	Answer     string   `json:"answer"`
	Alternates []string `json:"alternates,omitempty"`
	Hint       string   `json:"hint,omitempty"`
}

type resultDocument struct {
	AttemptID   string                   `json:"attemptId"`
	StartedAt   string                   `json:"startedAt"`
	CompletedAt string                   `json:"completedAt"`
	CompletedOn string                   `json:"completedOn"`
	Score       int                      `json:"score"`
	MaxScore    int                      `json:"maxScore"`
	AutoScore   int                      `json:"autoScore"`
	AutoMax     int                      `json:"autoMax"`
	SelfScore   int                      `json:"selfScore"`
	SelfMax     int                      `json:"selfMax"`
	Exercises   []exerciseResultDocument `json:"exercises"`
}

type exerciseResultDocument struct {
	ExerciseID string `json:"exerciseId"`
	Type       string `json:"type"`
	Grading    string `json:"grading"`
	Score      int    `json:"score"`
	MaxScore   int    `json:"maxScore"`
	Correct    int    `json:"correct"`
	Total      int    `json:"total"`
}

type translationPayload struct {
	Source    string `json:"source"`
	Reference string `json:"reference,omitempty"`
}

type orderingPayload struct {
	Items       []string `json:"items"`
	Translation string   `json:"translation,omitempty"`
}

type matchingPayload struct {
	Pairs []pairPayload `json:"pairs"`
}

type pairPayload struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}

type multipleChoicePayload struct {
	Questions []mcQuestionPayload `json:"questions"`
}

type mcQuestionPayload struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Answer   string   `json:"answer"`
}

type readingPayload struct {
	Genre       string              `json:"genre"`
	Title       string              `json:"title"`
	Passage     string              `json:"passage"`
	Annotations []annotationPayload `json:"annotations,omitempty"`
	Questions   []questionPayload   `json:"questions"`
}

type annotationPayload struct {
	Surface string `json:"surface"`
	Reading string `json:"reading,omitempty"`
	Gloss   string `json:"gloss,omitempty"`
}

type questionPayload struct {
	Question   string   `json:"question"`
	Kind       string   `json:"kind"`
	Options    []string `json:"options,omitempty"`
	Answer     string   `json:"answer,omitempty"`
	Alternates []string `json:"alternates,omitempty"`
}

type writingPromptPayload struct {
	Guidance    string `json:"guidance,omitempty"`
	ModelAnswer string `json:"modelAnswer,omitempty"`
}

type scriptPracticePayload struct {
	Items []scriptItemPayload `json:"items"`
}

type scriptItemPayload struct {
	Glyph   string `json:"glyph"`
	Reading string `json:"reading,omitempty"`
	Meaning string `json:"meaning,omitempty"`
}

func ownerFrom(req events.APIGatewayV2HTTPRequest) string {
	authorizer := req.RequestContext.Authorizer
	if authorizer == nil {
		return ""
	}
	if authorizer.JWT != nil {
		return authorizer.JWT.Claims["sub"]
	}
	if owner, ok := authorizer.Lambda["owner"].(string); ok {
		return owner
	}
	return ""
}

func header(req events.APIGatewayV2HTTPRequest, name string) string {
	for key, value := range req.Headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func requestBody(req events.APIGatewayV2HTTPRequest) ([]byte, *events.APIGatewayV2HTTPResponse) {
	body := []byte(req.Body)
	if req.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(req.Body)
		if err != nil {
			resp := errorJSON(http.StatusBadRequest, "request body is not valid base64")
			return nil, &resp
		}
		body = decoded
	}
	if len(body) > maxLessonBodyBytes {
		resp := errorJSON(http.StatusRequestEntityTooLarge, fmt.Sprintf("request body must be at most %d bytes", maxLessonBodyBytes))
		return nil, &resp
	}
	return body, nil
}

func (h *Handler) handleLessonImport(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	idempotencyKey := header(req, "Idempotency-Key")
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		return errorJSON(http.StatusBadRequest, "Idempotency-Key must contain 8-200 characters")
	}
	body, errResp := requestBody(req)
	if errResp != nil {
		return *errResp
	}

	candidate, issues := decodeLessonDocument(body)
	if len(issues) > 0 {
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "lesson validation failed", Issues: issues})
	}

	hash := sha256.Sum256(body)
	result, err := h.importer.Import(ctx, inbound.LessonImportCommand{
		Owner:          owner,
		ContentHash:    hex.EncodeToString(hash[:]),
		IdempotencyKey: idempotencyKey,
		Lesson:         candidate,
	})
	if err != nil {
		return lessonError(ctx, err)
	}

	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return respondJSON(status, toImportResponse(result))
}

func (h *Handler) handleLessonPrompt(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	body, errResp := requestBody(req)
	if errResp != nil {
		return *errResp
	}

	var dto struct {
		Language         string   `json:"language"`
		Level            string   `json:"level"`
		Topic            string   `json:"topic"`
		TopicSlug        string   `json:"topicSlug"`
		ExerciseTypes    []string `json:"exerciseTypes"`
		ReadingStage     string   `json:"readingStage"`
		Length           string   `json:"length"`
		IncludeReference *bool    `json:"includeReference"`
	}
	if err := json.Unmarshal(body, &dto); err != nil {
		return errorJSON(http.StatusBadRequest, "request body must be a JSON object with the prompt parameters")
	}

	includeReference := true
	if dto.IncludeReference != nil {
		includeReference = *dto.IncludeReference
	}
	result, err := h.prompts.Build(ctx, inbound.LessonPromptQuery{
		Owner:            ownerFrom(req),
		Language:         dto.Language,
		Level:            dto.Level,
		Topic:            dto.Topic,
		TopicSlug:        dto.TopicSlug,
		ExerciseTypes:    dto.ExerciseTypes,
		ReadingStage:     dto.ReadingStage,
		Length:           dto.Length,
		IncludeReference: includeReference,
	})
	if err != nil {
		return lessonError(ctx, err)
	}
	return respondJSON(http.StatusOK, map[string]string{"prompt": result.Prompt})
}

type lessonTopicDTO struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	WordCount    int    `json:"wordCount"`
	CoveredCount int    `json:"coveredCount"`
}

func (h *Handler) handleLessonTopics(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	result, err := h.topics.Topics(ctx, inbound.LessonTopicsQuery{
		Owner:    owner,
		Language: req.QueryStringParameters["lang"],
		Level:    req.QueryStringParameters["level"],
	})
	if err != nil {
		return lessonError(ctx, err)
	}
	items := make([]lessonTopicDTO, 0, len(result.Topics))
	for _, topic := range result.Topics {
		items = append(items, lessonTopicDTO{
			Slug:         topic.Slug,
			Name:         topic.Name,
			Description:  topic.Description,
			WordCount:    topic.WordCount,
			CoveredCount: topic.CoveredCount,
		})
	}
	return respondJSON(http.StatusOK, map[string][]lessonTopicDTO{"topics": items})
}

func (h *Handler) handleLessonList(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	limit, ok := parseLimit(req.QueryStringParameters["limit"])
	if !ok {
		return errorJSON(http.StatusBadRequest, "limit must be a positive integer")
	}
	result, err := h.library.List(ctx, inbound.LessonListQuery{
		Owner:  owner,
		Limit:  limit,
		Cursor: req.QueryStringParameters["cursor"],
	})
	if err != nil {
		return lessonError(ctx, err)
	}

	items := make([]lessonSummaryDTO, 0, len(result.Lessons))
	for _, stored := range result.Lessons {
		items = append(items, toLessonSummary(stored))
	}
	return respondJSON(http.StatusOK, pageResponse[lessonSummaryDTO]{Items: items, NextCursor: result.NextCursor})
}

func (h *Handler) handleLessonGet(ctx context.Context, req events.APIGatewayV2HTTPRequest, id string) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	stored, err := h.library.Get(ctx, inbound.LessonQuery{Owner: owner, ID: id})
	if err != nil {
		return lessonError(ctx, err)
	}
	return respondJSON(http.StatusOK, toLessonDetail(stored))
}

func (h *Handler) handleLessonDelete(ctx context.Context, req events.APIGatewayV2HTTPRequest, id string) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	if err := h.library.Delete(ctx, inbound.LessonQuery{Owner: owner, ID: id}); err != nil {
		return lessonError(ctx, err)
	}
	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNoContent}
}

func (h *Handler) handleLessonResult(ctx context.Context, req events.APIGatewayV2HTTPRequest, id string) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	body, errResp := requestBody(req)
	if errResp != nil {
		return *errResp
	}
	var doc resultDocument
	if issue := decodeStrict(body, &doc, "$"); issue != nil {
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "lesson result validation failed", Issues: []issueDTO{*issue}})
	}
	startedAt, err := time.Parse(time.RFC3339Nano, doc.StartedAt)
	if err != nil {
		return errorJSON(http.StatusBadRequest, "startedAt must be an RFC 3339 timestamp")
	}
	completedAt, err := time.Parse(time.RFC3339Nano, doc.CompletedAt)
	if err != nil {
		return errorJSON(http.StatusBadRequest, "completedAt must be an RFC 3339 timestamp")
	}
	completedOn, errResponse := parseStudyDate(doc.CompletedOn, "completedOn")
	if errResponse != nil {
		return *errResponse
	}
	exercises := make([]lesson.ExerciseResult, 0, len(doc.Exercises))
	for _, exercise := range doc.Exercises {
		exercises = append(exercises, lesson.ExerciseResult{
			ExerciseID: exercise.ExerciseID,
			Type:       lesson.ExerciseType(exercise.Type),
			Grading:    exercise.Grading,
			Score:      exercise.Score,
			MaxScore:   exercise.MaxScore,
			Correct:    exercise.Correct,
			Total:      exercise.Total,
		})
	}
	result, err := h.results.Record(ctx, inbound.LessonResultCommand{
		Owner: owner, CompletedOn: completedOn,
		Result: lesson.Result{
			AttemptID:   doc.AttemptID,
			LessonID:    id,
			StartedAt:   startedAt,
			CompletedAt: completedAt,
			Score:       doc.Score,
			MaxScore:    doc.MaxScore,
			AutoScore:   doc.AutoScore,
			AutoMax:     doc.AutoMax,
			SelfScore:   doc.SelfScore,
			SelfMax:     doc.SelfMax,
			Exercises:   exercises,
		},
	})
	if err != nil {
		return lessonError(ctx, err)
	}
	return respondJSON(http.StatusCreated, map[string]any{
		"attemptId":   result.AttemptID,
		"lessonId":    result.LessonID,
		"completedAt": result.CompletedAt,
		"score":       result.Score,
		"maxScore":    result.MaxScore,
	})
}

func lessonError(ctx context.Context, err error) events.APIGatewayV2HTTPResponse {
	var validation *lesson.ValidationError
	if errors.As(err, &validation) {
		issues := make([]issueDTO, 0, len(validation.Issues))
		for _, issue := range validation.Issues {
			issues = append(issues, issueDTO{Path: issue.Path, Message: issue.Message})
		}
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "lesson validation failed", Issues: issues})
	}
	switch {
	case errors.Is(err, lesson.ErrNotFound):
		return errorJSON(http.StatusNotFound, lesson.ErrNotFound.Error())
	case errors.Is(err, lesson.ErrInvalidLessonID), errors.Is(err, lesson.ErrInvalidCursor):
		return errorJSON(http.StatusBadRequest, err.Error())
	case errors.Is(err, lesson.ErrInvalidResult):
		return errorJSON(http.StatusBadRequest, err.Error())
	case errors.Is(err, lesson.ErrAlreadyExists):
		return errorJSON(http.StatusConflict, "lesson result already exists")
	case errors.Is(err, lesson.ErrIdempotencyConflict):
		return errorJSON(http.StatusConflict, lesson.ErrIdempotencyConflict.Error())
	case errors.Is(err, lesson.ErrInvalidOwner):
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	slog.ErrorContext(ctx, "lesson request failed", "error", err)
	return errorJSON(http.StatusInternalServerError, "internal error")
}

func decodeLessonDocument(body []byte) (lesson.Lesson, []issueDTO) {
	var doc lessonDocument
	if issue := decodeStrict(body, &doc, "$"); issue != nil {
		return lesson.Lesson{}, []issueDTO{*issue}
	}

	candidate := lesson.Lesson{
		SchemaVersion:    doc.SchemaVersion,
		ID:               doc.LessonID,
		Language:         lesson.Language(doc.Language),
		Level:            lesson.Level(strings.ToUpper(doc.Level)),
		Title:            doc.Title,
		Description:      doc.Description,
		Topic:            doc.Topic,
		Tags:             doc.Tags,
		ReadingStage:     lesson.ReadingStage(doc.ReadingStage),
		SourceModel:      doc.SourceModel,
		EstimatedMinutes: doc.EstimatedMinutes,
	}

	var issues []issueDTO
	for i, exercise := range doc.Exercises {
		mapped, exerciseIssues := decodeExercise(exercise, fmt.Sprintf("exercises[%d]", i))
		issues = append(issues, exerciseIssues...)
		candidate.Exercises = append(candidate.Exercises, mapped)
	}
	return candidate, issues
}

func decodeExercise(doc exerciseDocument, path string) (lesson.Exercise, []issueDTO) {
	exercise := lesson.Exercise{
		ID:                doc.ExerciseID,
		Type:              lesson.ExerciseType(doc.Type),
		Prompt:            doc.Prompt,
		Points:            doc.Points,
		ReferencedVocab:   doc.ReferencedVocab,
		ReferencedGrammar: doc.ReferencedGrammar,
	}
	if len(doc.Payload) == 0 {
		return exercise, nil
	}

	payloadPath := path + ".payload"
	var issue *issueDTO
	switch exercise.Type {
	case lesson.TypeCloze:
		payload := &clozePayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.Cloze = toClozeDomain(payload)
		}
	case lesson.TypeTranslation:
		payload := &translationPayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.Translation = &lesson.Translation{Source: payload.Source, Reference: payload.Reference}
		}
	case lesson.TypeOrdering:
		payload := &orderingPayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.Ordering = &lesson.Ordering{Items: payload.Items, Translation: payload.Translation}
		}
	case lesson.TypeMatching:
		payload := &matchingPayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.Matching = toMatchingDomain(payload)
		}
	case lesson.TypeMultipleChoice:
		payload := &multipleChoicePayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.MultipleChoice = toMultipleChoiceDomain(payload)
		}
	case lesson.TypeReading:
		payload := &readingPayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.Reading = toReadingDomain(payload)
		}
	case lesson.TypeWritingPrompt:
		payload := &writingPromptPayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.WritingPrompt = &lesson.WritingPrompt{Guidance: payload.Guidance, ModelAnswer: payload.ModelAnswer}
		}
	case lesson.TypeScriptPractice:
		payload := &scriptPracticePayload{}
		if issue = decodeStrict(doc.Payload, payload, payloadPath); issue == nil {
			exercise.ScriptPractice = toScriptPracticeDomain(payload)
		}
	}
	if issue != nil {
		return exercise, []issueDTO{*issue}
	}
	return exercise, nil
}

func decodeStrict(data []byte, target any, path string) *issueDTO {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &issueDTO{Path: path, Message: decodeMessage(err)}
	}
	if decoder.More() {
		return &issueDTO{Path: path, Message: "must contain a single JSON object with no trailing content"}
	}
	return nil
}

func decodeMessage(err error) string {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return "must contain the lesson JSON"
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("is not valid JSON near offset %d: %s", syntaxErr.Offset, syntaxErr.Error())
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "value"
		}
		return fmt.Sprintf("field %q must be of type %s", field, typeErr.Type)
	case strings.Contains(err.Error(), "unknown field"):
		return strings.TrimPrefix(err.Error(), "json: ") + "; remove it or check the field name against the schema"
	default:
		return "could not be parsed: " + err.Error()
	}
}

func toClozeDomain(payload *clozePayload) *lesson.Cloze {
	blanks := make([]lesson.Blank, 0, len(payload.Blanks))
	for _, blank := range payload.Blanks {
		blanks = append(blanks, lesson.Blank{Index: blank.Index, Answer: blank.Answer, Alternates: blank.Alternates, Hint: blank.Hint})
	}
	return &lesson.Cloze{Text: payload.Text, Blanks: blanks, WordBank: payload.WordBank}
}

func toMultipleChoiceDomain(payload *multipleChoicePayload) *lesson.MultipleChoice {
	questions := make([]lesson.MCQuestion, 0, len(payload.Questions))
	for _, question := range payload.Questions {
		questions = append(questions, lesson.MCQuestion{Question: question.Question, Options: question.Options, Answer: question.Answer})
	}
	return &lesson.MultipleChoice{Questions: questions}
}

func toMatchingDomain(payload *matchingPayload) *lesson.Matching {
	pairs := make([]lesson.Pair, 0, len(payload.Pairs))
	for _, pair := range payload.Pairs {
		pairs = append(pairs, lesson.Pair{Left: pair.Left, Right: pair.Right})
	}
	return &lesson.Matching{Pairs: pairs}
}

func toReadingDomain(payload *readingPayload) *lesson.Reading {
	annotations := make([]lesson.Annotation, 0, len(payload.Annotations))
	for _, annotation := range payload.Annotations {
		annotations = append(annotations, lesson.Annotation{
			Surface: annotation.Surface,
			Reading: annotation.Reading,
			Gloss:   annotation.Gloss,
		})
	}
	questions := make([]lesson.Question, 0, len(payload.Questions))
	for _, question := range payload.Questions {
		questions = append(questions, lesson.Question{
			Question:   question.Question,
			Kind:       question.Kind,
			Options:    question.Options,
			Answer:     question.Answer,
			Alternates: question.Alternates,
		})
	}
	return &lesson.Reading{
		Genre:       payload.Genre,
		Title:       payload.Title,
		Passage:     payload.Passage,
		Annotations: annotations,
		Questions:   questions,
	}
}

func toScriptPracticeDomain(payload *scriptPracticePayload) *lesson.ScriptPractice {
	items := make([]lesson.ScriptItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, lesson.ScriptItem{Glyph: item.Glyph, Reading: item.Reading, Meaning: item.Meaning})
	}
	return &lesson.ScriptPractice{Items: items}
}

type importResponseDTO struct {
	LessonID      string `json:"lessonId"`
	Title         string `json:"title"`
	Language      string `json:"language"`
	Level         string `json:"level"`
	ReadingStage  string `json:"readingStage"`
	ExerciseCount int    `json:"exerciseCount"`
	TotalPoints   int    `json:"totalPoints"`
	VocabRefCount int    `json:"vocabRefCount"`
	CreatedAt     string `json:"createdAt"`
	Created       bool   `json:"created"`
}

func toImportResponse(result inbound.LessonImportResult) importResponseDTO {
	l := result.Stored.Lesson
	points := 0
	vocabRefs := map[string]bool{}
	for _, exercise := range l.Exercises {
		points += exercise.Points
		for _, id := range exercise.ReferencedVocab {
			vocabRefs[id] = true
		}
	}
	return importResponseDTO{
		LessonID:      l.ID,
		Title:         l.Title,
		Language:      string(l.Language),
		Level:         string(l.Level),
		ReadingStage:  string(l.ReadingStage),
		ExerciseCount: len(l.Exercises),
		TotalPoints:   points,
		VocabRefCount: len(vocabRefs),
		CreatedAt:     result.Stored.CreatedAt.Format(time.RFC3339),
		Created:       result.Created,
	}
}

type lessonSummaryDTO struct {
	LessonID         string   `json:"lessonId"`
	Language         string   `json:"language"`
	Level            string   `json:"level"`
	Title            string   `json:"title"`
	Description      string   `json:"description,omitempty"`
	Topic            string   `json:"topic,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	ReadingStage     string   `json:"readingStage"`
	SourceModel      string   `json:"sourceModel,omitempty"`
	EstimatedMinutes int      `json:"estimatedMinutes,omitempty"`
	ExerciseTypes    []string `json:"exerciseTypes"`
	ExerciseCount    int      `json:"exerciseCount"`
	TotalPoints      int      `json:"totalPoints"`
	HasStory         bool     `json:"hasStory"`
	CreatedAt        string   `json:"createdAt"`
}

func toLessonSummary(stored inbound.StoredLesson) lessonSummaryDTO {
	l := stored.Lesson
	points := 0
	var types []string
	seen := map[string]bool{}
	hasStory := false
	for _, exercise := range l.Exercises {
		points += exercise.Points
		if !seen[string(exercise.Type)] {
			seen[string(exercise.Type)] = true
			types = append(types, string(exercise.Type))
		}
		if exercise.Type == lesson.TypeReading && exercise.Reading != nil && exercise.Reading.Genre == lesson.GenreShortStory {
			hasStory = true
		}
	}
	return lessonSummaryDTO{
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
		ExerciseTypes:    types,
		ExerciseCount:    len(l.Exercises),
		TotalPoints:      points,
		HasStory:         hasStory,
		CreatedAt:        stored.CreatedAt.Format(time.RFC3339),
	}
}

type lessonDetailDTO struct {
	lessonDocument
	CreatedAt string `json:"createdAt"`
}

func toLessonDetail(stored inbound.StoredLesson) lessonDetailDTO {
	l := stored.Lesson
	doc := lessonDocument{
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
		doc.Exercises = append(doc.Exercises, toExerciseDocument(exercise))
	}
	return lessonDetailDTO{lessonDocument: doc, CreatedAt: stored.CreatedAt.Format(time.RFC3339)}
}

func toExerciseDocument(exercise lesson.Exercise) exerciseDocument {
	doc := exerciseDocument{
		ExerciseID:        exercise.ID,
		Type:              string(exercise.Type),
		Prompt:            exercise.Prompt,
		Points:            exercise.Points,
		ReferencedVocab:   exercise.ReferencedVocab,
		ReferencedGrammar: exercise.ReferencedGrammar,
	}
	var payload any
	switch {
	case exercise.Cloze != nil:
		blanks := make([]blankPayload, 0, len(exercise.Cloze.Blanks))
		for _, blank := range exercise.Cloze.Blanks {
			blanks = append(blanks, blankPayload{Index: blank.Index, Answer: blank.Answer, Alternates: blank.Alternates, Hint: blank.Hint})
		}
		payload = clozePayload{Text: exercise.Cloze.Text, Blanks: blanks, WordBank: exercise.Cloze.WordBank}
	case exercise.Translation != nil:
		payload = translationPayload{Source: exercise.Translation.Source, Reference: exercise.Translation.Reference}
	case exercise.Ordering != nil:
		payload = orderingPayload{Items: exercise.Ordering.Items, Translation: exercise.Ordering.Translation}
	case exercise.Matching != nil:
		pairs := make([]pairPayload, 0, len(exercise.Matching.Pairs))
		for _, pair := range exercise.Matching.Pairs {
			pairs = append(pairs, pairPayload{Left: pair.Left, Right: pair.Right})
		}
		payload = matchingPayload{Pairs: pairs}
	case exercise.MultipleChoice != nil:
		questions := make([]mcQuestionPayload, 0, len(exercise.MultipleChoice.Questions))
		for _, question := range exercise.MultipleChoice.Questions {
			questions = append(questions, mcQuestionPayload{Question: question.Question, Options: question.Options, Answer: question.Answer})
		}
		payload = multipleChoicePayload{Questions: questions}
	case exercise.Reading != nil:
		annotations := make([]annotationPayload, 0, len(exercise.Reading.Annotations))
		for _, annotation := range exercise.Reading.Annotations {
			annotations = append(annotations, annotationPayload{
				Surface: annotation.Surface,
				Reading: annotation.Reading,
				Gloss:   annotation.Gloss,
			})
		}
		questions := make([]questionPayload, 0, len(exercise.Reading.Questions))
		for _, question := range exercise.Reading.Questions {
			questions = append(questions, questionPayload{
				Question:   question.Question,
				Kind:       question.Kind,
				Options:    question.Options,
				Answer:     question.Answer,
				Alternates: question.Alternates,
			})
		}
		payload = readingPayload{
			Genre:       exercise.Reading.Genre,
			Title:       exercise.Reading.Title,
			Passage:     exercise.Reading.Passage,
			Annotations: annotations,
			Questions:   questions,
		}
	case exercise.WritingPrompt != nil:
		payload = writingPromptPayload{
			Guidance:    exercise.WritingPrompt.Guidance,
			ModelAnswer: exercise.WritingPrompt.ModelAnswer,
		}
	case exercise.ScriptPractice != nil:
		items := make([]scriptItemPayload, 0, len(exercise.ScriptPractice.Items))
		for _, item := range exercise.ScriptPractice.Items {
			items = append(items, scriptItemPayload{Glyph: item.Glyph, Reading: item.Reading, Meaning: item.Meaning})
		}
		payload = scriptPracticePayload{Items: items}
	}
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err == nil {
			doc.Payload = encoded
		}
	}
	return doc
}
