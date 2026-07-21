package burmese_test

import (
	"testing"

	"github.com/rtrydev/langler-backend/internal/domain/burmese"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "vowel before medial ha", text: "လူှ", want: "လှူ"},
		{name: "lower vowel before upper vowel", text: "ကုိ", want: "ကို"},
		{name: "vowel e before medial ra", text: "ကေြ", want: "ကြေ"},
		{name: "medials out of order", text: "မှြ", want: "မြှ"},
		{name: "anusvara before lower vowel", text: "သံုး", want: "သုံး"},
		{name: "visarga before dot below", text: "နဲး့", want: "နဲ့း"},
		{name: "already canonical", text: "မနက်ဖြန် ကျောင်းကို သွားမယ်။", want: "မနက်ဖြန် ကျောင်းကို သွားမယ်။"},
		{name: "kinzi untouched", text: "မင်္ဂလာပါ", want: "မင်္ဂလာပါ"},
		{name: "contracted spelling untouched", text: "ကျွန်ုပ်", want: "ကျွန်ုပ်"},
		{name: "non-burmese untouched", text: "hello, świecie", want: "hello, świecie"},
		{name: "decomposed great u", text: "\u1025\u102e\u1038", want: "\u1026\u1038"},
		{name: "decomposed au", text: "\u101e\u103c\u1002\u102f\u1010\u103a", want: "\u1029\u1002\u102f\u1010\u103a"},
		{name: "decomposed au with asat", text: "\u101e\u103c\u1031\u102c\u103a", want: "\u102a"},
		{name: "atomic letters untouched", text: "\u1026\u1038\u1029", want: "\u1026\u1038\u1029"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := burmese.Normalize(test.text); got != test.want {
				t.Errorf("Normalize(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}
