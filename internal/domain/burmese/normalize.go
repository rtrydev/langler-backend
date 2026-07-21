package burmese

// markRank orders the marks that may follow a base consonant within one
// syllable cluster, per canonical Unicode storage order (UTN 11): medials
// ya < ra < wa < ha, then vowel e, upper vowels, lower vowels, aa, anusvara,
// dot below, visarga. Marks sharing a rank keep their original relative order.
var markRank = map[rune]int{
	0x103b: 1, 0x103c: 2, 0x103d: 3, 0x103e: 4,
	0x1031: 5,
	0x102d: 6, 0x102e: 6, 0x1032: 6,
	0x102f: 7, 0x1030: 7,
	0x102b: 8, 0x102c: 8,
	0x1036: 9,
	0x1037: 10,
	0x1038: 11,
}

// Normalize reorders medial, vowel, and tone marks into canonical Unicode
// storage order, repairing text generated in visual or typing order (for
// example vowel signs written before medials). Asat (U+103A) and virama
// (U+1039) act as boundaries so kinzi, stacks, and contracted spellings such
// as ကျွန်ုပ် are never rearranged. Non-Myanmar text passes through unchanged.
func Normalize(text string) string {
	runes := []rune(text)
	for start := 0; start < len(runes); {
		if _, sortable := markRank[runes[start]]; !sortable {
			start++
			continue
		}
		end := start + 1
		for end < len(runes) {
			if _, sortable := markRank[runes[end]]; !sortable {
				break
			}
			end++
		}
		sortMarks(runes[start:end])
		start = end
	}
	return string(compose(runes))
}

// compose replaces decomposed spellings of atomic letters with their canonical
// code points: ဥ+ီ → ဦ, သ+ြ → ဩ, and သ+ြ+ော် → ဪ (per UTN 11). LLMs emit
// the decomposed forms, which the validator would otherwise reject.
func compose(runes []rune) []rune {
	out := runes[:0]
	for i := 0; i < len(runes); {
		switch {
		case runes[i] == 0x1025 && i+1 < len(runes) && runes[i+1] == 0x102e:
			out = append(out, 0x1026)
			i += 2
		case runes[i] == 'သ' && i+1 < len(runes) && runes[i+1] == 0x103c:
			if i+4 < len(runes) && runes[i+2] == 0x1031 && runes[i+3] == 0x102c && runes[i+4] == 0x103a {
				out = append(out, 0x102a)
				i += 5
			} else {
				out = append(out, 0x1029)
				i += 2
			}
		default:
			out = append(out, runes[i])
			i++
		}
	}
	return out
}

func sortMarks(marks []rune) {
	for i := 1; i < len(marks); i++ {
		for j := i; j > 0 && markRank[marks[j]] < markRank[marks[j-1]]; j-- {
			marks[j], marks[j-1] = marks[j-1], marks[j]
		}
	}
}
