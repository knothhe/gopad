package appsearch

import (
	"slices"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		text, query string
		want        bool
	}{
		{"终端", "zhongduan", true},
		{"终端", " ZD ", true},
		{"终端", "zhong duan", true},
		{"终端", "ＺＤ", true},
		{"终端", "hongdu", true},
		{"终端", "zhong端", true},
		{"终端", "终d", true},
		{"终端", "终端", true},
		{"终端", "zhongdian", false},
		{"微信 WeChat", "weixinwechat", true},
		{"微信 WeChat", "wxwechat", true},
		{"微信 WeChat", "微xin", true},
		{"微信 WeChat", "wech", true},
		{"重庆音乐", "chongqingyinyue", true},
		{"重庆音乐", "zhongqingyinle", true},
		{"重庆音乐", "cqyy", true},
		{"音乐", "yy", true},
		{"音乐", "yl", true},
		{"音🙂乐", "yinyue", false},
		{"音🙂乐", "yin🙂yue", true},
		{"com.apple.Terminal /Applications/Terminal.app", "APPLE.TERM", true},
		{"终端", "  ", true},
		{"", "zd", false},
		{strings.Repeat("重行乐", 100), strings.Repeat("z", 30) + "q", false},
	} {
		t.Run(tc.text+"/"+tc.query, func(t *testing.T) {
			if got := Match(tc.text, tc.query); got != tc.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tc.text, tc.query, got, tc.want)
			}
		})
	}
}

func TestReadingsIncludesOnlyUsedCharactersAndAllPronunciations(t *testing.T) {
	dictionary := Readings("重庆音乐", "微信 WeChat", "重")
	if !slices.Contains(dictionary["重"], "chong") || !slices.Contains(dictionary["重"], "zhong") {
		t.Fatalf("missing alternate readings: %v", dictionary["重"])
	}
	if !slices.Contains(dictionary["乐"], "yue") || !slices.Contains(dictionary["乐"], "le") {
		t.Fatalf("missing alternate readings: %v", dictionary["乐"])
	}
	if len(dictionary) != 6 {
		t.Fatalf("dictionary contains unused characters: %v", dictionary)
	}
	// Callers cannot mutate the cache through an API response dictionary.
	dictionary["重"][0] = "changed"
	if slices.Contains(Readings("重")["重"], "changed") {
		t.Fatal("response dictionary changed the reading cache")
	}
}
