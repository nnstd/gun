package y18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Y18N provides internationalization support.
type Y18N struct {
	directory string
	locale    string
	updateFiles bool
	fallbackToLanguage bool
	cache     map[string]map[string]string
}

// Default creates a new Y18N instance. Accepts an optional map with
// "directory", "locale", "updateFiles", "fallbackToLanguage" keys.
func Default(opts ...map[string]interface{}) *Y18N {
	y := &Y18N{
		directory:          "./locales",
		locale:             "en",
		updateFiles:        true,
		fallbackToLanguage: true,
		cache:              make(map[string]map[string]string),
	}
	if len(opts) > 0 {
		if v, ok := opts[0]["directory"].(string); ok {
			y.directory = v
		}
		if v, ok := opts[0]["locale"].(string); ok {
			y.locale = v
		}
		if v, ok := opts[0]["updateFiles"].(bool); ok {
			y.updateFiles = v
		}
		if v, ok := opts[0]["fallbackToLanguage"].(bool); ok {
			y.fallbackToLanguage = v
		}
	}
	return y
}

// Translate looks up a string in the current locale's translations.
func (y *Y18N) Translate(str string, args ...interface{}) string {
	table := y.loadLocale(y.locale)
	if val, ok := table[str]; ok {
		if len(args) > 0 {
			return fmt.Sprintf(val, args...)
		}
		return val
	}
	// Fallback to language prefix (e.g. "en" from "en_US")
	if y.fallbackToLanguage {
		if idx := strings.IndexByte(y.locale, '_'); idx > 0 {
			lang := y.locale[:idx]
			table = y.loadLocale(lang)
			if val, ok := table[str]; ok {
				if len(args) > 0 {
					return fmt.Sprintf(val, args...)
				}
				return val
			}
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(str, args...)
	}
	return str
}

// SetLocale sets the active locale.
func (y *Y18N) SetLocale(locale string) {
	y.locale = locale
}

// GetLocale returns the active locale.
func (y *Y18N) GetLocale() string {
	return y.locale
}

// UpdateLocale merges translations into the current locale.
func (y *Y18N) UpdateLocale(obj map[string]string) {
	table := y.loadLocale(y.locale)
	for k, v := range obj {
		table[k] = v
	}
	y.cache[y.locale] = table
}

func (y *Y18N) loadLocale(locale string) map[string]string {
	if table, ok := y.cache[locale]; ok {
		return table
	}
	table := make(map[string]string)
	path := filepath.Join(y.directory, locale+".json")
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &table)
	}
	y.cache[locale] = table
	return table
}
