package reference_test

import (
	"errors"
	"testing"

	"github.com/rtrydev/langler-backend/internal/domain/reference"
)

func TestNewLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		want    reference.Language
		wantErr error
	}{
		{name: "two letter", code: "ja", want: "ja"},
		{name: "three letter", code: "mya", want: "mya"},
		{name: "empty", code: "", wantErr: reference.ErrInvalidLanguage},
		{name: "uppercase", code: "JA", wantErr: reference.ErrInvalidLanguage},
		{name: "too long", code: "japanese", wantErr: reference.ErrInvalidLanguage},
		{name: "injection", code: "ja#x", wantErr: reference.ErrInvalidLanguage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := reference.NewLanguage(tt.code)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewLanguage(%q) error = %v, want %v", tt.code, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NewLanguage(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestNewLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		band    string
		want    reference.Level
		wantErr error
	}{
		{name: "jlpt", band: "N5", want: "N5"},
		{name: "lowercase normalized", band: "n5", want: "N5"},
		{name: "cefr", band: "A1", want: "A1"},
		{name: "empty", band: "", wantErr: reference.ErrInvalidLevel},
		{name: "injection", band: "N5#", wantErr: reference.ErrInvalidLevel},
		{name: "too long", band: "VERYLONGLEVEL", wantErr: reference.ErrInvalidLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := reference.NewLevel(tt.band)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewLevel(%q) error = %v, want %v", tt.band, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NewLevel(%q) = %q, want %q", tt.band, got, tt.want)
			}
		})
	}
}

func TestNewScriptType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    reference.ScriptType
		wantErr error
	}{
		{name: "kana", value: "kana", want: "kana"},
		{name: "kanji", value: "kanji", want: "kanji"},
		{name: "empty", value: "", wantErr: reference.ErrInvalidScriptType},
		{name: "uppercase", value: "Kana", wantErr: reference.ErrInvalidScriptType},
		{name: "injection", value: "kana#", wantErr: reference.ErrInvalidScriptType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := reference.NewScriptType(tt.value)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewScriptType(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NewScriptType(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestNewTopicTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    reference.TopicTag
		wantErr error
	}{
		{name: "simple", value: "food", want: "food"},
		{name: "kebab", value: "daily-life", want: "daily-life"},
		{name: "empty", value: "", wantErr: reference.ErrInvalidTopic},
		{name: "leading hyphen", value: "-food", wantErr: reference.ErrInvalidTopic},
		{name: "uppercase", value: "Food", wantErr: reference.ErrInvalidTopic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := reference.NewTopicTag(tt.value)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewTopicTag(%q) error = %v, want %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NewTopicTag(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
