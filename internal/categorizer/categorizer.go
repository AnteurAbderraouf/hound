// Package categorizer maps a DNS domain to a human-friendly category
// (social, streaming, gaming, adult, ...). It ships an embedded YAML file
// with ~150 curated second-level domains and matches by exact-then-suffix,
// so `m.youtube.com` and `googlevideo.com` both resolve to `streaming`.
package categorizer

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed categories.yaml
var categoriesYAML []byte

// CategoryUnknown is returned for domains not found in any category list.
const CategoryUnknown = "other"

const colorUnknown = "#6b7280"

type rawCategory struct {
	Color   string   `yaml:"color"`
	Domains []string `yaml:"domains"`
}

type rawFile struct {
	Categories map[string]rawCategory `yaml:"categories"`
}

// Categorizer maps domains to categories.
type Categorizer struct {
	domainToCat map[string]string
	catColor    map[string]string
}

// New parses the embedded categories.yaml and returns a ready-to-use matcher.
func New() (*Categorizer, error) {
	var raw rawFile
	if err := yaml.Unmarshal(categoriesYAML, &raw); err != nil {
		return nil, fmt.Errorf("parse embedded categories: %w", err)
	}

	c := &Categorizer{
		domainToCat: make(map[string]string, 512),
		catColor:    make(map[string]string, len(raw.Categories)+1),
	}
	c.catColor[CategoryUnknown] = colorUnknown

	for name, cat := range raw.Categories {
		c.catColor[name] = cat.Color
		for _, d := range cat.Domains {
			c.domainToCat[strings.ToLower(strings.TrimSpace(d))] = name
		}
	}
	return c, nil
}

// Categorize returns the category name for a given domain, or CategoryUnknown
// if no match. Matching is done in two passes:
//  1. exact domain match (google.com -> productivity)
//  2. progressive suffix match (mail.google.com -> productivity)
func (c *Categorizer) Categorize(domain string) string {
	d := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if d == "" {
		return CategoryUnknown
	}
	if cat, ok := c.domainToCat[d]; ok {
		return cat
	}
	// walk parent suffixes: a.b.c -> b.c, then c
	parts := strings.Split(d, ".")
	for i := 1; i < len(parts)-1; i++ {
		if cat, ok := c.domainToCat[strings.Join(parts[i:], ".")]; ok {
			return cat
		}
	}
	return CategoryUnknown
}

// Color returns the hex color associated with a category name.
func (c *Categorizer) Color(category string) string {
	if col, ok := c.catColor[category]; ok {
		return col
	}
	return colorUnknown
}

// Categories returns the full name->color map (used by the API to hydrate
// the UI legend without hardcoding colors on the frontend).
func (c *Categorizer) Categories() map[string]string {
	out := make(map[string]string, len(c.catColor))
	for k, v := range c.catColor {
		out[k] = v
	}
	return out
}
