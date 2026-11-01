// Package i18n holds the interface translations. Keys are the Simplified
// Chinese strings used in the code, so a missing entry simply falls back to
// Chinese instead of showing an empty label.
package i18n

import (
	"fmt"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

var getUserDefaultUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// Lang is one interface language.
type Lang string

const (
	ZHHans Lang = "zh-Hans"
	ZHHant Lang = "zh-Hant"
	EN     Lang = "en"
	JA     Lang = "ja"
)

// Languages lists the selectable languages in menu order.
var Languages = []Lang{ZHHans, ZHHant, EN, JA}

// Label is the language's own name, as a language menu should show it.
func (l Lang) Label() string {
	switch l {
	case ZHHant:
		return "中文（繁體）"
	case EN:
		return "English"
	case JA:
		return "日本語"
	default:
		return "中文（简体）"
	}
}

func (l Lang) column() int {
	switch l {
	case ZHHant:
		return 1
	case EN:
		return 2
	case JA:
		return 3
	default:
		return 0
	}
}

var current atomic.Value // Lang

func init() { current.Store(ZHHans) }

// Current is the language in use.
func Current() Lang { return current.Load().(Lang) }

// Use switches the language for this run only.
func Use(l Lang) {
	switch l {
	case ZHHans, ZHHant, EN, JA:
	default:
		l = ZHHans
	}
	current.Store(l)
}

// Set switches the language and stores the choice for the next start.
func Set(l Lang) error {
	Use(l)
	return save(Current())
}

// T translates a Simplified Chinese source string.
func T(source string) string {
	lang := Current()
	if lang == ZHHans {
		return source
	}
	entry, ok := catalog[source]
	if !ok {
		return source
	}
	if text := entry[lang.column()]; text != "" {
		return text
	}
	return source
}

// Tf translates a format string and formats it, keeping the arguments in the
// order the source string uses.
func Tf(source string, args ...any) string {
	return fmt.Sprintf(T(source), args...)
}

const languageValue = "Language"

// Load uses the stored preference, and follows the Windows display language
// on a first start.
func Load() Lang {
	lang := System()
	if key, err := registry.OpenKey(registry.CURRENT_USER, `Software\ClipRecorder`, registry.QUERY_VALUE); err == nil {
		value, _, readErr := key.GetStringValue(languageValue)
		key.Close()
		if readErr == nil {
			for _, l := range Languages {
				if string(l) == value {
					lang = l
				}
			}
		}
	}
	current.Store(lang)
	return lang
}

// System is the language of Windows itself.
func System() Lang {
	id, _, _ := getUserDefaultUILanguage.Call()
	switch uint16(id) {
	case 0x0804, 0x1004: // PRC, Singapore
		return ZHHans
	case 0x0404, 0x0c04, 0x1404: // Taiwan, Hong Kong, Macau
		return ZHHant
	case 0x0411:
		return JA
	}
	switch uint16(id) & 0xff {
	case 0x04:
		return ZHHans
	case 0x11:
		return JA
	}
	return EN
}

// saveIMEIdlePref stores the language for the next start.
func save(l Lang) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\ClipRecorder`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(languageValue, string(l))
}
