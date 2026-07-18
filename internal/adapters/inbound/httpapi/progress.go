package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type reviewItemDTO struct {
	ItemID         string  `json:"itemId"`
	Kind           string  `json:"kind"`
	Headword       string  `json:"headword"`
	Reading        string  `json:"reading,omitempty"`
	Gloss          string  `json:"gloss"`
	Example        string  `json:"example,omitempty"`
	ExampleMeaning string  `json:"exampleMeaning,omitempty"`
	EaseFactor     float64 `json:"easeFactor"`
	IntervalDays   int     `json:"intervalDays"`
	DueDate        string  `json:"dueDate"`
}

type dueLanguageDTO struct {
	Language string          `json:"language"`
	Items    []reviewItemDTO `json:"items"`
}

func (h *Handler) handleDueReviews(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	dueOn, errResponse := parseStudyDate(req.QueryStringParameters["date"], "date")
	if errResponse != nil {
		return *errResponse
	}
	result, err := h.progress.Due(ctx, inbound.DueReviewQuery{Owner: owner, Language: req.QueryStringParameters["language"], DueOn: dueOn})
	if err != nil {
		return progressError(ctx, err)
	}
	languages := make([]dueLanguageDTO, 0, len(result.Languages))
	for _, group := range result.Languages {
		items := make([]reviewItemDTO, 0, len(group.Items))
		for _, item := range group.Items {
			items = append(items, toReviewItem(item))
		}
		languages = append(languages, dueLanguageDTO{Language: group.Language, Items: items})
	}
	return respondJSON(http.StatusOK, map[string]any{"languages": languages})
}

func (h *Handler) handleReviewGrade(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	body, errResponse := requestBody(req)
	if errResponse != nil {
		return *errResponse
	}
	var document struct {
		Language   string `json:"language"`
		Kind       string `json:"kind"`
		ItemID     string `json:"itemId"`
		Grade      string `json:"grade"`
		ReviewedOn string `json:"reviewedOn"`
	}
	if issue := decodeStrict(body, &document, "$"); issue != nil {
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "review validation failed", Issues: []issueDTO{*issue}})
	}
	reviewedOn, errResponse := parseStudyDate(document.ReviewedOn, "reviewedOn")
	if errResponse != nil {
		return *errResponse
	}
	item, err := h.progress.Grade(ctx, inbound.ReviewGradeCommand{
		Owner: owner, Language: document.Language, Kind: progress.ItemKind(document.Kind),
		ItemID: document.ItemID, Grade: progress.Grade(document.Grade), ReviewedOn: reviewedOn,
	})
	if err != nil {
		return progressError(ctx, err)
	}
	return respondJSON(http.StatusOK, toReviewItem(item))
}

type recentLessonDTO struct {
	LessonID    string `json:"lessonId"`
	Title       string `json:"title"`
	Score       int    `json:"score"`
	MaxScore    int    `json:"maxScore"`
	CompletedAt string `json:"completedAt"`
}

type languageProgressDTO struct {
	Language            string              `json:"language"`
	LessonsCompleted    int                 `json:"lessonsCompleted"`
	ItemsTracked        int                 `json:"itemsTracked"`
	DueToday            int                 `json:"dueToday"`
	CurrentReviewStreak int                 `json:"currentReviewStreak"`
	ReviewHistory       []inbound.ReviewDay `json:"reviewHistory"`
	RecentLessons       []recentLessonDTO   `json:"recentLessons"`
}

func (h *Handler) handleProgressSummary(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	dueOn, errResponse := parseStudyDate(req.QueryStringParameters["date"], "date")
	if errResponse != nil {
		return *errResponse
	}
	result, err := h.progress.Summary(ctx, inbound.ProgressSummaryQuery{Owner: owner, DueOn: dueOn})
	if err != nil {
		return progressError(ctx, err)
	}
	languages := make([]languageProgressDTO, 0, len(result.Languages))
	for _, language := range result.Languages {
		recent := make([]recentLessonDTO, 0, len(language.RecentLessons))
		for _, activity := range language.RecentLessons {
			recent = append(recent, recentLessonDTO{
				LessonID: activity.LessonID, Title: activity.Title, Score: activity.Score,
				MaxScore: activity.MaxScore, CompletedAt: activity.CompletedAt.UTC().Format(time.RFC3339Nano),
			})
		}
		languages = append(languages, languageProgressDTO{
			Language: language.Language, LessonsCompleted: language.LessonsCompleted,
			ItemsTracked: language.ItemsTracked, DueToday: language.DueToday,
			CurrentReviewStreak: language.CurrentReviewStreak,
			ReviewHistory:       language.ReviewHistory, RecentLessons: recent,
		})
	}
	return respondJSON(http.StatusOK, map[string]any{"languages": languages})
}

func parseStudyDate(value, field string) (time.Time, *events.APIGatewayV2HTTPResponse) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		response := errorJSON(http.StatusBadRequest, field+" must be a YYYY-MM-DD date")
		return time.Time{}, &response
	}
	return parsed, nil
}

func toReviewItem(item progress.Item) reviewItemDTO {
	return reviewItemDTO{
		ItemID: item.ID, Kind: string(item.Kind), Headword: item.Headword, Reading: item.Reading,
		Gloss: item.Gloss, Example: item.Example, ExampleMeaning: item.ExampleMeaning,
		EaseFactor: item.EaseFactor, IntervalDays: item.IntervalDays,
		DueDate: item.DueDate.UTC().Format(time.DateOnly),
	}
}

func progressError(ctx context.Context, err error) events.APIGatewayV2HTTPResponse {
	switch {
	case errors.Is(err, lesson.ErrInvalidOwner):
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	case errors.Is(err, progress.ErrInvalidItem), errors.Is(err, progress.ErrInvalidGrade):
		return errorJSON(http.StatusBadRequest, err.Error())
	case errors.Is(err, progress.ErrNotFound):
		return errorJSON(http.StatusNotFound, err.Error())
	}
	slog.ErrorContext(ctx, "progress request failed", "error", err)
	return errorJSON(http.StatusInternalServerError, "internal error")
}
