package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/nnstd/gun/test262/parser"
)

// SkipList defines criteria for skipping test262 tests.
type SkipList struct {
	SkipFeatures      []string `yaml:"skip_features"`
	SkipFlags         []string `yaml:"skip_flags"`
	SkipNegativePhases []string `yaml:"skip_negative_phases"`
	SkipPatterns      []string `yaml:"skip_patterns"`
	SkipIncludes      []string `yaml:"skip_includes"`

	// compiled sets for fast lookup
	features      map[string]bool
	flags         map[string]bool
	negativePhases map[string]bool
	includes      map[string]bool
}

// LoadSkipList loads a skip list from a YAML file.
func LoadSkipList(path string) (*SkipList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skip list: %w", err)
	}
	var sl SkipList
	if err := yaml.Unmarshal(data, &sl); err != nil {
		return nil, fmt.Errorf("parse skip list YAML: %w", err)
	}
	sl.compile()
	return &sl, nil
}

// DefaultSkipList returns the built-in skip list embedded in the binary.
func DefaultSkipList() *SkipList {
	sl := SkipList{
		SkipFeatures: []string{
			"SharedArrayBuffer", "Atomics", "generators", "async-iteration",
			"regexp-unicode-property-escapes", "regexp-named-groups",
			"tail-call-optimization", "Symbol.species", "cross-realm", "realm",
		},
		SkipFlags: []string{"module", "onlyStrict", "raw"},
		SkipNegativePhases: []string{"parse", "early", "resolution"},
		SkipPatterns: []string{
			"test/intl402/**", "test/annexB/**", "test/staging/**",
		},
		SkipIncludes: []string{
			"propertyHelper.js", "deepEqual.js", "compareArray.js", "compareIterator.js",
			"isConstructor.js", "proxyTrapsHelper.js", "regExpUtils.js",
			"resizableArrayBufferUtils.js", "temporalHelpers.js", "testIntl.js",
			"testTypedArray.js", "atomicsHelper.js", "byteConversionValues.js",
			"dateConstants.js", "decimalToHexString.js", "detachArrayBuffer.js",
			"fnGlobalObject.js", "nativeFunctionMatcher.js", "nans.js",
			"promiseHelper.js", "tcoHelper.js", "typeCoercion.js",
			"wellKnownIntrinsicObjects.js",
		},
	}
	sl.compile()
	return &sl
}

func (sl *SkipList) compile() {
	sl.features = make(map[string]bool, len(sl.SkipFeatures))
	for _, f := range sl.SkipFeatures {
		sl.features[f] = true
	}
	sl.flags = make(map[string]bool, len(sl.SkipFlags))
	for _, f := range sl.SkipFlags {
		sl.flags[f] = true
	}
	sl.negativePhases = make(map[string]bool, len(sl.SkipNegativePhases))
	for _, p := range sl.SkipNegativePhases {
		sl.negativePhases[p] = true
	}
	sl.includes = make(map[string]bool, len(sl.SkipIncludes))
	for _, inc := range sl.SkipIncludes {
		sl.includes[inc] = true
	}
}

// ShouldSkip checks whether a test should be skipped.
// Returns (true, reason) if the test should be skipped, (false, "") otherwise.
func (sl *SkipList) ShouldSkip(info *parser.TestInfo, filePath string) (bool, string) {
	// Check features intersection
	for _, f := range info.Features {
		if sl.features[f] {
			return true, fmt.Sprintf("unsupported feature: %s", f)
		}
	}

	// Check flags
	for _, f := range info.Flags {
		if sl.flags[f] {
			return true, fmt.Sprintf("unsupported flag: %s", f)
		}
	}

	// Check negative phases
	if info.IsNegative() && sl.negativePhases[info.Negative.Phase] {
		return true, fmt.Sprintf("unsupported negative phase: %s", info.Negative.Phase)
	}

	// Check glob patterns
	for _, pattern := range sl.SkipPatterns {
		if matchGlob(pattern, filePath) {
			return true, fmt.Sprintf("matched pattern: %s", pattern)
		}
	}

	// Check includes
	for _, inc := range info.Includes {
		if sl.includes[inc] {
			return true, fmt.Sprintf("unsupported include: %s", inc)
		}
	}

	return false, ""
}

// matchGlob checks if filePath matches a simple glob pattern.
// Supports ** for recursive directory matching and * for single segment.
func matchGlob(pattern, filePath string) bool {
	// Normalize paths
	filePath = filepath.ToSlash(filePath)

	// Handle ** patterns
	if strings.Contains(pattern, "**") {
		prefix := strings.SplitN(pattern, "**", 2)[0]
		if prefix != "" && strings.HasPrefix(filePath, prefix) {
			return true
		}
		return false
	}

	// Use filepath.Match for simple globs
	matched, err := filepath.Match(pattern, filepath.Base(filePath))
	if err == nil && matched {
		return true
	}

	// Try matching the full path
	matched, err = filepath.Match(pattern, filePath)
	if err == nil && matched {
		return true
	}

	return false
}
