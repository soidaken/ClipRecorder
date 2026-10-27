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
			"content": `{"blocks":[{"src":"Hello, world","text":"你好，世界","bold":true}]}`,
		}}}})
	}))
	defer server.Close()
	blocks, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", image)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0] != (Block{Src: "Hello, world", Text: "你好，世界", Bold: true}) {
		t.Fatalf("unexpected result: %+v", blocks)
	}
}

func TestTranslateImageDropsBlocksItCannotPlace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{
			"content": `{"blocks":[{"src":" Price ","text":" 价格 "},{"src":"","text":"无原文"},{"src":"Empty","text":"  "}]}`,
		}}}})
	}))
	defer server.Close()
	blocks, err := (Client{Endpoint: server.URL}).TranslateImage(context.Background(), "test-key", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Src != "Price" || blocks[0].Text != "价格" {
		t.Fatalf("blocks without source or translation must be dropped, the rest trimmed: %+v", blocks)
	}
}

func TestTranslateImageRejectsEmptyResults(t *testing.T) {
	for _, content := range []string{`{"blocks":[]}`, `{}`, `not JSON`} {
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

func TestTranslateImageRetriesLinesLeftInEnglish(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		content := `{"blocks":[{"src":"Hello world","text":"Hello world"}]}`
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
	if lines[0].Text != "你好，世界" || lines[0].Src != "Hello world" {
		t.Fatalf("repair did not replace the text: %+v", lines[0])
	}
}

func TestTranslationStaysSingleRequestForChinese(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{
			"content": `{"blocks":[{"src":"Hello","text":"你好"},{"src":"main.go","text":"main.go"}]}`,
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
