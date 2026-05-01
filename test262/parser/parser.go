package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
)

// Negative describes an expected error from a test262 negative test.
type Negative struct {
	Type  string `yaml:"type"`
	Phase string `yaml:"phase"`
}

// TestInfo holds the parsed frontmatter from a test262 test file.
type TestInfo struct {
	Esid        string    `yaml:"esid"`
	Es6id       string    `yaml:"es6id"`
	Es5id       string    `yaml:"es5id"`
	Description string    `yaml:"description"`
	Info        string    `yaml:"info"`
	Features    []string  `yaml:"features"`
	Flags       []string  `yaml:"flags"`
	Includes    []string  `yaml:"includes"`
	Negative    *Negative `yaml:"negative"`
	Author      string    `yaml:"author"`
	Locale      []string  `yaml:"locale"`
}

// ParseFrontmatter extracts and parses YAML frontmatter from a test262 .js file.
// Frontmatter is delimited by /*--- and ---*/.
// Returns empty TestInfo (no error) if no frontmatter found.
func ParseFrontmatter(source []byte) (*TestInfo, error) {
	start := bytes.Index(source, []byte("/*---"))
	if start < 0 {
		return &TestInfo{}, nil
	}

	end := bytes.Index(source[start+5:], []byte("---*/"))
	if end < 0 {
		return &TestInfo{}, nil
	}

	yamlBytes := source[start+5 : start+5+end]
	yamlBytes = bytes.TrimSpace(yamlBytes)

	if len(yamlBytes) == 0 {
		return &TestInfo{}, nil
	}

	var info TestInfo
	if err := yaml.Unmarshal(yamlBytes, &info); err != nil {
		return nil, fmt.Errorf("parse frontmatter YAML: %w", err)
	}

	return &info, nil
}

// StripFrontmatter removes the frontmatter block from source, returning
// only the test body.
func StripFrontmatter(source []byte) []byte {
	start := bytes.Index(source, []byte("/*---"))
	if start < 0 {
		return source
	}

	end := bytes.Index(source[start:], []byte("---*/"))
	if end < 0 {
		return source
	}

	return append(source[:start], source[start+end+4:]...)
}

// HasFlag checks if the test has a specific flag.
func (t *TestInfo) HasFlag(flag string) bool {
	for _, f := range t.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// HasFeature checks if the test requires a specific feature.
func (t *TestInfo) HasFeature(feature string) bool {
	for _, f := range t.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// HasInclude checks if the test requires a specific include file.
func (t *TestInfo) HasInclude(include string) bool {
	for _, inc := range t.Includes {
		if inc == include {
			return true
		}
	}
	return false
}

// IsNegative returns true if the test expects an error.
func (t *TestInfo) IsNegative() bool {
	return t.Negative != nil && t.Negative.Type != ""
}

// Key returns a unique identifier for the test based on its path.
func (t *TestInfo) Key(filePath string) string {
	return strings.TrimPrefix(filePath, "test/")
}
