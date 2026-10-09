package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/justin06lee/caveira/core/llm"
)

// The name is asked of the small model, without reasoning or tools, and
// comes back without the quotes and full stop it was given with.
func TestNameChatAsksTheSmallModel(t *testing.T) {
	var got struct {
		Model    string        `json:"model"`
		Effort   string        `json:"reasoning_effort"`
		Tools    []any         `json:"tools"`
		Messages []llm.Message `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sse(`"Reading a.txt."`, "", "", ""))
	}))
	defer srv.Close()

	name, err := nameChat(context.Background(), llm.New(srv.URL, "k"), "what is in a.txt?")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Reading a.txt" {
		t.Fatalf("name %q", name)
	}
	if got.Model != "abliterated-model" || got.Effort != "none" || len(got.Tools) != 0 {
		t.Fatalf("request: model %q, effort %q, %d tools", got.Model, got.Effort, len(got.Tools))
	}
	if len(got.Messages) != 2 || got.Messages[1].Content != "what is in a.txt?" {
		t.Fatalf("messages: %+v", got.Messages)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"Fix the login bug":                       "Fix the login bug",
		"  **Title: Fix the login bug.**\n":       "Fix the login bug",
		"<think>a bug, then</think>\n\nLogin bug": "Login bug",
		"“Poppins everywhere”":                    "Poppins everywhere",
		"":                                        "",
	} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A new chat takes the name the model gives it, whether the name comes
// while the first turn runs or after it ends; it is kept on disk, told to
// the window, and asked for only once.
func TestChatIsNamedByTheModel(t *testing.T) {
	for _, during := range []bool{true, false} {
		t.Run(map[bool]string{true: "during the turn", false: "after the turn"}[during], func(t *testing.T) {
			var a *App
			var id string
			named := func() bool {
				a.mu.Lock()
				defer a.mu.Unlock()
				return a.chats[id].named
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.ReadAll(r.Body)
				// Held until the name is in, so it lands mid-turn.
				for during && !named() {
					time.Sleep(5 * time.Millisecond)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, sse("Hi.", "", "", ""))
			}))
			defer srv.Close()
			a, dir := testApp(t, srv)

			var mu sync.Mutex
			var titles []string
			a.sink = func(name string, data any) {
				if e, ok := data.(ChatEvent); ok && e.Type == "title" {
					mu.Lock()
					titles = append(titles, e.Title)
					mu.Unlock()
				}
			}
			gate := make(chan struct{})
			asked := make(chan string, 4)
			a.namer = func(ctx context.Context, client *llm.Client, first string) (string, error) {
				asked <- first
				if !during {
					<-gate
				}
				return "Saying hi", nil
			}

			v, err := a.NewChat(dir)
			if err != nil {
				t.Fatal(err)
			}
			id = v.ID
			if _, err := a.Send(id, "say hi\nplease"); err != nil {
				t.Fatal(err)
			}
			v = waitIdle(t, a, id, func(v ChatView) bool { return !v.Running })
			if !during {
				if v.Title != "say hi" {
					t.Fatalf("before the name, title %q", v.Title)
				}
				close(gate)
			}
			v = waitIdle(t, a, id, func(v ChatView) bool { return v.Title == "Saying hi" })
			if first := <-asked; first != "say hi\nplease" {
				t.Fatalf("named from %q", first)
			}

			headers, err := a.Chats(dir)
			if err != nil || len(headers) != 1 || headers[0].Title != "Saying hi" {
				t.Fatalf("on disk: %+v, %v", headers, err)
			}
			mu.Lock()
			if len(titles) != 1 || titles[0] != "Saying hi" {
				t.Fatalf("title events %q", titles)
			}
			mu.Unlock()

			// A second turn keeps the name and does not ask again.
			if _, err := a.Send(id, "again"); err != nil {
				t.Fatal(err)
			}
			v = waitIdle(t, a, id, func(v ChatView) bool { return !v.Running })
			if v.Title != "Saying hi" || len(asked) != 0 {
				t.Fatalf("second turn: title %q, asked %d more", v.Title, len(asked))
			}
		})
	}
}
