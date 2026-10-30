package i18n

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestDictionaryParityAndPlaceholders(t *testing.T) {
	placeholder := regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9]*)\}`)
	keys := func(values map[string]string) []string {
		out := []string{}
		for key := range values {
			out = append(out, key)
		}
		slices.Sort(out)
		return out
	}
	if !reflect.DeepEqual(keys(catalogs["en"]), keys(catalogs["zh-CN"])) {
		t.Fatal("English and Chinese message IDs must match")
	}
	for key, en := range catalogs["en"] {
		zh := catalogs["zh-CN"][key]
		if strings.TrimSpace(en) == "" || strings.TrimSpace(zh) == "" {
			t.Errorf("empty message %s", key)
		}
		a, b := placeholder.FindAllString(en, -1), placeholder.FindAllString(zh, -1)
		slices.Sort(a)
		slices.Sort(b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("placeholder mismatch %s: %v vs %v", key, a, b)
		}
	}
}

func TestLanguageFallbackAndPlainTextInterpolation(t *testing.T) {
	for _, test := range []struct{ pref, system, want string }{
		{"system", "zh-Hans-CN", "zh-CN"}, {"system", "en-GB", "en"}, {"zh-CN", "en", "zh-CN"}, {"en", "zh-CN", "en"}, {"future-language", "zh-CN", "en"}, {"system", "zh-Hant-TW", "en"},
	} {
		if got := Resolve(test.pref, test.system); got != test.want {
			t.Errorf("%+v: %s", test, got)
		}
	}
	value := T("en", "app.managerTitle", "name", "<img onerror=bad>")
	if value != "<img onerror=bad> · Apps" {
		t.Fatal(value)
	}
	copy := Messages("en")
	copy["category.all"] = "changed"
	if T("en", "category.all") != "All" {
		t.Fatal("dictionary was mutated")
	}
}
