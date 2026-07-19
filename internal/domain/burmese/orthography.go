package burmese

import (
	"fmt"
	"unicode/utf8"
)

type OrthographyIssue struct {
	Offset  int
	Cluster string
	Message string
}

func (i OrthographyIssue) Error() string {
	return fmt.Sprintf("%s at character %d near %q", i.Message, i.Offset+1, i.Cluster)
}

var medialBases = map[rune]map[rune]bool{
	0x103b: runeSet("ကခဂဃစဆဇတထဒဓနပဖဗဘမယလ"),
	0x103c: runeSet("ကခဂဃစဆဇတထဒဓနပဖဗဘမဟလ"),
	0x103d: runeSet("ကခဂဃငစဆဇဉညဋဌဍဎဏတထဒဓနပဖဗဘမယရလဝသဟ"),
	0x103e: runeSet("ငဉညနမယရလဝ"),
}

var tallAaBases = runeSet("ခဂငဒပဝ")
var stackClasses = []map[rune]bool{
	runeSet("ကခဂဃင"), runeSet("စဆဇဈညဉ"), runeSet("ဋဌဍဎဏ"), runeSet("တထဒဓန"), runeSet("ပဖဗဘမ"),
}

func ValidateOrthography(text string) []OrthographyIssue {
	runes := []rune(text)
	var issues []OrthographyIssue
	for start := 0; start < len(runes); {
		if !isMyanmar(runes[start]) {
			start++
			continue
		}
		end := start + 1
		for end < len(runes) && isMyanmar(runes[end]) {
			end++
		}
		issues = append(issues, validateRun(runes[start:end], start)...)
		start = end
	}
	return issues
}

func validateRun(runes []rune, offset int) []OrthographyIssue {
	var issues []OrthographyIssue
	base := rune(0)
	independent := false
	lastMedial := rune(0)
	marks := map[rune]bool{}
	for index, current := range runes {
		add := func(message string) {
			left := max(0, index-1)
			right := min(len(runes), index+2)
			issues = append(issues, OrthographyIssue{Offset: offset + index, Cluster: string(runes[left:right]), Message: message})
		}
		switch {
		case isConsonant(current):
			if index > 0 && runes[index-1] == 0x1039 {
				if !validStackLower(current) {
					add("consonant cannot be used as the lower member of a virama stack")
				}
				base = current
				independent = false
				lastMedial = 0
				marks = map[rune]bool{}
				continue
			}
			if base != 0 && index+1 < len(runes) && runes[index+1] == 0x103a {
				continue
			}
			base = current
			independent = false
			lastMedial = 0
			marks = map[rune]bool{}
		case isIndependentVowel(current):
			base = current
			independent = true
			lastMedial = 0
			marks = map[rune]bool{}
		case isMedial(current):
			if base == 0 || independent {
				add("medial mark must follow a Burmese consonant")
			} else if !medialBases[current][base] {
				add("medial mark is not legal on this consonant")
			}
			if lastMedial >= current {
				add("medial marks must be unique and in canonical Unicode order")
			}
			lastMedial = current
		case isDependentVowel(current):
			if base == 0 || independent {
				add("dependent vowel sign must follow a Burmese consonant")
			}
			if marks[current] {
				add("vowel and tone marks cannot be repeated")
			}
			marks[current] = true
			if current == 0x102b && !tallAaBases[base] {
				add("tall aa is not legal after this consonant; use ာ")
			}
			if current == 0x102c && tallAaBases[base] && lastMedial == 0 {
				add("this consonant requires tall aa ါ")
			}
			if lastMedial == 0x103e && (current == 0x102e || current == 0x1030) {
				add("medial ha cannot combine with long i or long u")
			}
		case current == 0x103a:
			if base == 0 || index == 0 || (!isConsonant(runes[index-1]) && !isDependentVowel(runes[index-1]) && runes[index-1] != 0x1037 && runes[index-1] != 0x1038) {
				add("asat must close a consonant or vowel-bearing syllable")
			}
			if marks[current] {
				add("asat cannot be repeated")
			}
			marks[current] = true
		case current == 0x1039:
			if index+1 >= len(runes) || !isConsonant(runes[index+1]) {
				add("virama must be followed by a stackable consonant")
				continue
			}
			upper := rune(0)
			if index > 0 && isConsonant(runes[index-1]) {
				upper = runes[index-1]
			} else if index > 1 && runes[index-1] == 0x103a && runes[index-2] == 'င' {
				upper = 'င'
			} else {
				add("virama must follow a consonant, or nga plus asat for kinzi")
			}
			if upper != 0 && !validStackUpper(upper) {
				add("consonant cannot be used as the upper member of a virama stack")
			}
		case current == 0x1037 || current == 0x1038 || current == 0x1036:
			if base == 0 {
				add("tone mark must follow a Burmese syllable")
			}
			if marks[current] {
				add("vowel and tone marks cannot be repeated")
			}
			marks[current] = true
		case current == 0x200c || current == 0x200d:
			add("join-control characters are not legal inside Burmese text")
		case current >= 0x1060 && current <= 0x1097:
			add("Zawgyi or non-Burmese extended code point must be converted to Unicode")
		}
	}
	return issues
}

func ContainsMyanmar(text string) bool {
	for _, current := range text {
		if isMyanmar(current) && current != '၊' && current != '။' {
			return true
		}
	}
	return false
}

func RuneCount(text string) int {
	return utf8.RuneCountInString(text)
}

func isMyanmar(current rune) bool {
	return current >= 0x1000 && current <= 0x109f || current >= 0xaa60 && current <= 0xaa7f || current >= 0xa9e0 && current <= 0xa9ff || current == 0x200c || current == 0x200d
}

func isConsonant(current rune) bool {
	return current >= 0x1000 && current <= 0x1021
}

func isIndependentVowel(current rune) bool {
	return current >= 0x1023 && current <= 0x102a
}

func isMedial(current rune) bool {
	return current >= 0x103b && current <= 0x103e
}

func isDependentVowel(current rune) bool {
	return current >= 0x102b && current <= 0x1032 || current == 0x1036
}

func validStackUpper(current rune) bool {
	return validStackLower(current) || current == 'ဟ'
}

func validStackLower(current rune) bool {
	if current == 'ဟ' {
		return true
	}
	for _, class := range stackClasses {
		if class[current] {
			return true
		}
	}
	return false
}

func runeSet(value string) map[rune]bool {
	result := make(map[rune]bool)
	for _, current := range value {
		result[current] = true
	}
	return result
}
