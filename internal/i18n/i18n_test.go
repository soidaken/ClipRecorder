package i18n

import "testing"

func TestCatalogCoversEveryLanguage(t *testing.T) {
	for source, entry := range catalog {
		for i, text := range entry {
			if text == "" {
				t.Fatalf("%q is missing language column %d", source, i)
			}
		}
		if entry[0] != source {
			t.Fatalf("Simplified Chinese must be the source string: %q vs %q", entry[0], source)
		}
	}
}

func TestTranslate(t *testing.T) {
	previous := Current()
	defer Use(previous)

	Use(ZHHans)
	if got := T("退出"); got != "退出" {
		t.Fatalf("zh-Hans: %q", got)
	}
	Use(ZHHant)
	if got := T("退出"); got != "結束" {
		t.Fatalf("zh-Hant: %q", got)
	}
	Use(EN)
	if got := T("退出"); got != "Quit" {
		t.Fatalf("en: %q", got)
	}
	Use(JA)
	if got := T("退出"); got != "終了" {
		t.Fatalf("ja: %q", got)
	}
	// Unknown strings fall back to the source instead of disappearing.
	if got := T("not translated"); got != "not translated" {
		t.Fatalf("fallback: %q", got)
	}
}

func TestTranslateFormat(t *testing.T) {
	previous := Current()
	defer Use(previous)
	Use(EN)
	if got := Tf("文件已保存至 %s：%v", "/tmp/a.mp4", 3); got != "File saved to /tmp/a.mp4: 3" {
		t.Fatalf("formatted: %q", got)
	}
}

func TestLanguageLabels(t *testing.T) {
	want := map[Lang]string{ZHHans: "中文（简体）", ZHHant: "中文（繁體）", EN: "English", JA: "日本語"}
	for lang, label := range want {
		if got := lang.Label(); got != label {
			t.Fatalf("%s label: %q", lang, got)
		}
	}
	if System() == "" {
		t.Fatal("system language must map to one of the supported languages")
	}
}
