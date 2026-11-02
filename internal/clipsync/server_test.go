package clipsync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStoreBumpsRevisionOnlyOnChange(t *testing.T) {
	store := NewStore()
	if rev := store.Set("a"); rev != 1 {
		t.Fatalf("first revision: %d", rev)
	}
	if rev := store.Set("a"); rev != 1 {
		t.Fatalf("identical text must not bump: %d", rev)
	}
	if rev := store.Set("b"); rev != 2 {
		t.Fatalf("changed text: %d", rev)
	}
	// Text coming back from the phone changes the revision even when equal, so
	// the phone learns the PC accepted it.
	if rev := store.SetFromRemote("b"); rev != 3 {
		t.Fatalf("remote text: %d", rev)
	}
}

func TestStoreWaitWakesOnChange(t *testing.T) {
	store := NewStore()
	store.Set("first")

	done := make(chan struct{})
	var gotRev uint64
	var gotText string
	go func() {
		gotRev, gotText = store.Wait(context.Background(), 1, 2*time.Second)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	store.Set("second")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("wait did not wake up")
	}
	if gotRev != 2 || gotText != "second" {
		t.Fatalf("wait returned %d %q", gotRev, gotText)
	}
}

func TestStoreWaitTimesOut(t *testing.T) {
	store := NewStore()
	rev, text := store.Wait(context.Background(), 0, 40*time.Millisecond)
	if rev != 0 || text != "" {
		t.Fatalf("timeout returned %d %q", rev, text)
	}
}

func newTestServer(token string, remote *string) *httptest.Server {
	store := NewStore()
	server := NewServer(token, store, func(text string) {
		if remote != nil {
			*remote = text
		}
	})
	return httptest.NewServer(server.http.Handler)
}

func TestHandlersRequireTheToken(t *testing.T) {
	ts := newTestServer("secret", nil)
	defer ts.Close()
	for _, path := range []string{"/api/clip", "/api/ping"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without token: %d", path, resp.StatusCode)
		}
	}
}

func TestClipRoundTrip(t *testing.T) {
	remote := ""
	ts := newTestServer("secret", &remote)
	defer ts.Close()

	// The phone pushes text; the PC side receives it through the callback.
	resp, err := http.Post(ts.URL+"/api/clip?t=secret", "application/json", strings.NewReader(`{"text":"from phone"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post: %d %s", resp.StatusCode, body)
	}
	if remote != "from phone" {
		t.Fatalf("remote text: %q", remote)
	}

	// The phone then reads the PC clipboard revision back.
	resp, err = http.Get(ts.URL + "/api/clip?t=secret&since=0")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Rev  uint64 `json:"rev"`
		Text string `json:"text"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if payload.Rev == 0 || payload.Text != "from phone" {
		t.Fatalf("get: %+v", payload)
	}
}

func TestPostRejectsHugeText(t *testing.T) {
	ts := newTestServer("secret", nil)
	defer ts.Close()
	huge := `{"text":"` + strings.Repeat("x", maxText+10) + `"}`
	resp, err := http.Post(ts.URL+"/api/clip?t=secret", "application/json", strings.NewReader(huge))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestPairingLine(t *testing.T) {
	if got := Pairing("192.168.1.5:47820", "abc"); got != "192.168.1.5:47820#abc" {
		t.Fatalf("pairing: %q", got)
	}
}
