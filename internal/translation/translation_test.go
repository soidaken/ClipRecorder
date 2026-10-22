package translation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type capturedBody struct {
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	ResponseFormat map[string]string `json:"response_format"`
}

func TestTranslateImageSendsImageAsData(t *testing.T) {
	image := []byte{0xff, 0xd8, 0xff, 0xd8, 0x2c}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing API authentication")
		}
		var body capturedBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
			t.Fatal("instructions and image must be separate messages")
		}
		var system string
		if err := json.Unmarshal(body.Messages[0].Content, &system); err != nil || system != TranslationPrompt {
			t.Error("system prompt changed")
		}
		var parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if err := json.Unmarshal(body.Messages[1].Content, &parts); err != nil {
			t.Fatal(err)
		}
		if len(parts) != 2 || parts[0].Type != "text" || parts[1].Type != "image_url" {
			t.Fatalf("unexpected content parts: %+v", parts)
		}
		want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
		if parts[1].ImageURL.URL != want {
			t.Error("image must be sent inline as a JPEG data URL")
		}
		if body.ResponseFormat["type"] != "json_object" {
			t.Error("JSON output required")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{
			"content": `{"lines":[{"text":"你好，世界","x":0.1,"y":0.2,"w":0.5,"h":0.1,"color":"abc"}]}`,
		}}}})
	}))
	defer server.Close()
	lines, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", image)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Text != "你好，世界" || lines[0].Color != "#aabbcc" {
		t.Fatalf("unexpected result: %+v", lines)
	}
}

func TestTranslateImageClampsAndDropsBadLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{
			"content": `{"lines":[{"text":"越界","x":0.9,"y":0.95,"w":0.8,"h":0.8,"color":"#123456"},{"text":"  ","x":0.1,"y":0.1,"w":0.2,"h":0.2},{"text":"无效","x":0.1,"y":0.1,"w":0,"h":0.2}]}`,
		}}}})
	}))
	defer server.Close()
	lines, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("unusable lines must be dropped: %+v", lines)
	}
	l := lines[0]
	if l.X != 0.9 || l.Y != 0.95 || l.W < 0.099 || l.H < 0.049 {
		t.Fatalf("coordinates not clamped: %+v", l)
	}
}

func TestTranslateImageRejectsEmptyResults(t *testing.T) {
	for _, content := range []string{`{"lines":[]}`, `{}`, `not JSON`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
		}))
		_, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", []byte{1})
		server.Close()
		if err == nil {
			t.Fatalf("accepted invalid content: %s", content)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	if _, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", []byte{1}); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("invalid key: %v", err)
	}
}

func TestNormalizeColor(t *testing.T) {
	for value, want := range map[string]string{
		"#224499": "#224499", "224499": "#224499", "#ABC": "#aabbcc",
		"": "#000000", "nope": "#000000", "#1234567": "#000000",
	} {
		if got := NormalizeColor(value); got != want {
			t.Fatalf("NormalizeColor(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestContrastOnWhite(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"white", "#ffffff"},
		{"light grey", "#d0d0d0"},
		{"yellow", "#ffe680"},
		{"normal blue", "#224499"},
		{"black", "#000000"},
	}
	for _, tc := range cases {
		col := ContrastOnWhite(ParseColor(tc.input))
		if l := Luminance(col); l > 0.2 {
			t.Fatalf("%s stays unreadable on white: %v luminance %.3f", tc.name, col, l)
		}
	}
	// A color that is already dark enough must keep its hue.
	kept := ContrastOnWhite(ParseColor("#224499"))
	if kept.R != 0x22 || kept.G != 0x44 || kept.B != 0x99 {
		t.Fatalf("dark color changed: %v", kept)
	}
	// Pure white text must end up clearly visible, not a faint grey.
	white := ContrastOnWhite(ParseColor("#ffffff"))
	if Luminance(white) > 0.2 {
		t.Fatalf("white text not darkened: %v", white)
	}
}

func TestTranslateImageRetriesLinesLeftInEnglish(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		content := `{"lines":[{"text":"Hello world","x":0.1,"y":0.2,"w":0.5,"h":0.1,"color":"#111111"}]}`
		if calls == 2 {
			var body capturedBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			var system, source string
			json.Unmarshal(body.Messages[0].Content, &system)
			json.Unmarshal(body.Messages[1].Content, &source)
			if system != RepairPrompt {
				t.Error("repair prompt missing")
			}
			if !strings.Contains(source, "Hello world") || strings.Contains(source, "image_url") {
				t.Fatalf("repair must send the text only, not the image: %s", source)
			}
			content = `{"lines":[{"id":0,"text":"你好，世界"}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
	}))
	defer server.Close()
	lines, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected one repair request, got %d calls", calls)
	}
	if lines[0].Text != "你好，世界" || lines[0].X != 0.1 {
		t.Fatalf("repair did not replace the text: %+v", lines[0])
	}
}

func TestTranslationStaysSingleRequestForChinese(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{
			"content": `{"lines":[{"text":"你好","x":0.1,"y":0.1,"w":0.2,"h":0.1,"color":"#000000"},{"text":"main.go","x":0.1,"y":0.3,"w":0.2,"h":0.1}]}`,
		}}}})
	}))
	defer server.Close()
	if _, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", []byte{1}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("common case must not retry, got %d calls", calls)
	}
}

func TestNeedsTranslation(t *testing.T) {
	for text, want := range map[string]bool{
		"Hello world":      true,
		"Continue":         true,
		"你好，世界":            false,
		"保存 Save":          false,
		"https://a.com/x":  false,
		"main.go":          false,
		"src/app_test.go":  false,
		"DeepSeek":         true,
		"第 2 章 Chapter II": false,
	} {
		if got := NeedsTranslation(text); got != want {
			t.Fatalf("NeedsTranslation(%q) = %v, want %v", text, got, want)
		}
	}
}
