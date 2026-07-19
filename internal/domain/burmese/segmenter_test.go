package burmese_test

import (
	"slices"
	"testing"

	"github.com/rtrydev/langler-backend/internal/domain/burmese"
)

func TestSegmenterUsesBigramAndUnigramProbabilities(t *testing.T) {
	t.Parallel()
	segmenter, err := burmese.NewSegmenter(burmese.NgramModel{
		Total: 1000,
		Unigram: map[string]int{
			"မြန်မာ": 100, "စာ": 90, "မြန်မာစာ": 2,
		},
		Bigram: map[string]map[string]int{"မြန်မာ": {"စာ": 80}},
	})
	if err != nil {
		t.Fatalf("NewSegmenter: %v", err)
	}
	if got := segmenter.Segment("မြန်မာစာ"); !slices.Equal(got, []string{"မြန်မာ", "စာ"}) {
		t.Errorf("Segment = %v, want [မြန်မာ စာ]", got)
	}
}

func TestNewSegmenterRejectsEmptyModel(t *testing.T) {
	t.Parallel()
	if _, err := burmese.NewSegmenter(burmese.NgramModel{}); err == nil {
		t.Fatal("NewSegmenter error = nil")
	}
}
