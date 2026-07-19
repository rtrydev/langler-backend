package burmese_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rtrydev/langler-backend/internal/domain/burmese"
)

func TestValidateOrthography(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		text  string
		valid bool
	}{
		{name: "ordinary sentence", text: "မနက်ဖြန် ကျောင်းကို သွားမယ်။", valid: true},
		{name: "kinzi and tall aa", text: "မင်္ဂလာပါ", valid: true},
		{name: "zawgyi storage order", text: "ေက်ာင္း", valid: false},
		{name: "orphan vowel", text: "ိက", valid: false},
		{name: "illegal medial ha", text: "ကှ", valid: false},
		{name: "repeated tone", text: "က့့", valid: false},
		{name: "virama before vowel", text: "က္ာ", valid: false},
		{name: "wrong aa form", text: "ဂာ", valid: false},
		{name: "join control", text: "က\u200cာ", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			issues := burmese.ValidateOrthography(test.text)
			if test.valid && len(issues) > 0 {
				t.Errorf("ValidateOrthography(%q) = %v", test.text, issues)
			}
			if !test.valid && len(issues) == 0 {
				t.Errorf("ValidateOrthography(%q) accepted illegal text", test.text)
			}
		})
	}
}

func TestGrammarInventoryExamplesConform(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../etl/langler_etl/data/grammar_my.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var topics []struct {
		TopicID string `json:"topicId"`
		Example struct {
			Text string `json:"text"`
		} `json:"example"`
	}
	if err := json.Unmarshal(data, &topics); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, topic := range topics {
		t.Run(topic.TopicID, func(t *testing.T) {
			t.Parallel()
			if issues := burmese.ValidateOrthography(topic.Example.Text); len(issues) > 0 {
				t.Errorf("ValidateOrthography(%q) = %v", topic.Example.Text, issues)
			}
		})
	}
}
