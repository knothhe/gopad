// Package appsearch matches application names in their original spelling,
// Mandarin pinyin, or pinyin initials.
package appsearch

import (
	"strings"
	"sync"
	"unicode"

	"github.com/mozillazg/go-pinyin"
	"golang.org/x/text/unicode/norm"
)

var readingCache sync.Map // rune -> immutable []string

// Normalize makes spaces, case and compatibility forms insignificant.
func Normalize(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(norm.NFKC.String(text)), ""))
}

func readings(r rune) []string {
	if !unicode.Is(unicode.Han, r) {
		return nil
	}
	if cached, ok := readingCache.Load(r); ok {
		return cached.([]string)
	}
	args := pinyin.NewArgs()
	args.Heteronym = true
	value := pinyin.SinglePinyin(r, args)
	readingCache.Store(r, value)
	return value
}

// Readings returns only the characters used by these texts. The management UI
// uses this same dictionary locally, without another transliteration library.
func Readings(texts ...string) map[string][]string {
	result := make(map[string][]string)
	for _, text := range texts {
		for _, r := range Normalize(text) {
			if _, exists := result[string(r)]; exists {
				continue
			}
			if values := readings(r); len(values) > 0 {
				result[string(r)] = append([]string(nil), values...)
			}
		}
	}
	return result
}

// Match preserves literal substring search and also accepts full pinyin and
// initials, including mixed Chinese/Latin names and alternate pronunciations.
func Match(text, query string) bool {
	text, query = Normalize(text), Normalize(query)
	if strings.Contains(text, query) {
		return true
	}
	hasReadings := false
	for _, r := range text {
		if len(readings(r)) > 0 {
			hasReadings = true
			break
		}
	}
	if !hasReadings {
		return false
	}
	return matchReadings(text, query, false) || matchReadings(text, query, true)
}

func matchReadings(text, query string, initials bool) bool {
	// Offsets record matched query prefixes, rather than enumerating the
	// exponentially many complete strings of a name with multiple heteronyms.
	active, next := make([]bool, len(query)+1), make([]bool, len(query)+1)
	active[0] = true
	for _, r := range text {
		options := []string{string(r)}
		for _, value := range readings(r) {
			if initials {
				value = value[:1]
			}
			options = append(options, value)
		}
		clear(next)
		next[0] = true
		for matched, valid := range active {
			if !valid {
				continue
			}
			for _, option := range options {
				for start := range option {
					if matched > 0 && start > 0 {
						break
					}
					remaining, suffix := query[matched:], option[start:]
					if strings.HasPrefix(suffix, remaining) {
						return true
					}
					if strings.HasPrefix(remaining, suffix) {
						next[matched+len(suffix)] = true
					}
				}
			}
		}
		active, next = next, active
	}
	return false
}

// Score ranks literal application names above phonetic matches. Zero means no
// match. Metadata callers should use a lower score than a name match, regardless
// of whether a bundle identifier happens to match exactly.
func Score(text, query string) int {
	text, query = Normalize(text), Normalize(query)
	if query == "" {
		return 0
	}
	if text == query {
		return 500
	}
	if strings.HasPrefix(text, query) {
		return 400
	}
	if strings.Contains(text, query) {
		return 300
	}
	if Match(text, query) {
		return 200
	}
	return 0
}
