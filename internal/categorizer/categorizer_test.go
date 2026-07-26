package categorizer

import "testing"

func TestCategorize(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := map[string]string{
		"youtube.com":         "streaming",
		"m.youtube.com":       "streaming",
		"i.ytimg.com":         "streaming",
		"tiktok.com":          "social",
		"www.tiktok.com":      "social",
		"pornhub.com":         "adult",
		"steamcommunity.com":  "gaming",
		"discord.com":         "messaging",
		"wikipedia.org":       "education",
		"amazon.fr":           "shopping",
		"doubleclick.net":     "ads_tracking",
		"unknown-domain.zzz":  CategoryUnknown,
		"":                    CategoryUnknown,
		"a.b.c.google.com":    "productivity",
	}

	for input, want := range cases {
		got := c.Categorize(input)
		if got != want {
			t.Errorf("Categorize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestColor(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.Color("adult"); got != "#ff3e3e" {
		t.Errorf("Color(adult) = %q, want #ff3e3e", got)
	}
	if got := c.Color("nonexistent"); got == "" {
		t.Error("Color(nonexistent) should return fallback color")
	}
}
