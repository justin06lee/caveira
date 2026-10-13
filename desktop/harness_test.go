package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// TestHarness serves the built frontend with a stand-in for the Wails
// bridge, so the window can be opened and driven in a browser against the
// real backend. WebKit does not paint a window that is behind others, so
// this is how to look at the UI without bringing the app to the front:
//
//	cd frontend && bun run build && cd ..
//	CAVEIRA_HARNESS=localhost:34999 go test -run TestHarness -timeout 0 .
//
// Methods are called by name through POST /call/<Method>; events stream
// from /events. Menu actions can be sent with POST /menu/<action>. Pages
// of the caveira site the app would open in the browser are only logged.
//
// With CAVEIRA_HARNESS_SITE=1 the app runs in a home of its own, signed
// out, against the stand-in caveira site from account_test.go, which is
// driven with POST /site/approve (which also takes the token back),
// /site/deny, /site/wait (a sign-in waits again), /site/revoke, and
// /site/plan/<id>; the real ~/.caveira is not touched.
func TestHarness(t *testing.T) {
	addr := os.Getenv("CAVEIRA_HARNESS")
	if addr == "" {
		t.Skip("set CAVEIRA_HARNESS=host:port to serve the harness")
	}
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		t.Fatal(err)
	}
	var st *site
	if os.Getenv("CAVEIRA_HARNESS_SITE") != "" {
		t.Setenv("HOME", t.TempDir())
		st = newSite(t, nil)
		t.Setenv("CAVEIRA_CLOUD_URL", st.URL)
	}

	var mu sync.Mutex
	subs := map[chan []byte]bool{}
	publish := func(name string, data any) {
		b, _ := json.Marshal(map[string]any{"name": name, "data": data})
		mu.Lock()
		defer mu.Unlock()
		for ch := range subs {
			select {
			case ch <- b:
			default:
			}
		}
	}
	app := NewApp("harness")
	app.sink = publish
	app.browse = func(url string) error {
		t.Logf("open %s", url)
		return nil
	}

	mux := http.NewServeMux()
	if st != nil {
		mux.HandleFunc("POST /site/{do}", func(w http.ResponseWriter, r *http.Request) {
			st.set(func(s *site) {
				switch r.PathValue("do") {
				case "approve":
					s.approved, s.denied, s.revoked = true, false, false
				case "deny":
					s.denied = true
				case "wait":
					s.approved, s.denied = false, false
				case "revoke":
					s.revoked = true
				}
			})
		})
		mux.HandleFunc("POST /site/plan/{id}", func(w http.ResponseWriter, r *http.Request) {
			st.set(func(s *site) { s.plan = r.PathValue("id") })
		})
	}
	mux.HandleFunc("/bridge.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		fmt.Fprint(w, harnessShim)
	})
	mux.HandleFunc("/call/", func(w http.ResponseWriter, r *http.Request) {
		out, err := callByName(app, strings.TrimPrefix(r.URL.Path, "/call/"), r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	})
	mux.HandleFunc("/menu/", func(w http.ResponseWriter, r *http.Request) {
		app.emit("menu", strings.TrimPrefix(r.URL.Path, "/menu/"))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		ch := make(chan []byte, 4096)
		mu.Lock()
		subs[ch] = true
		mu.Unlock()
		defer func() {
			mu.Lock()
			delete(subs, ch)
			mu.Unlock()
		}()
		flusher := w.(http.Flusher)
		fmt.Fprint(w, ": ok\n\n")
		flusher.Flush()
		for {
			select {
			case b := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	files := http.FileServerFS(dist)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			files.ServeHTTP(w, r)
			return
		}
		b, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "build the frontend first", http.StatusNotFound)
			return
		}
		page := strings.Replace(string(b), "<script", `<script src="/bridge.js"></script><script`, 1)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})
	t.Logf("harness on http://%s", addr)
	t.Fatal(http.ListenAndServe(addr, mux))
}

// callByName calls one of App's exported methods with the JSON array in
// the request body as its arguments, the way the Wails bridge does.
func callByName(app *App, name string, r *http.Request) ([]byte, error) {
	m := reflect.ValueOf(app).MethodByName(name)
	if !m.IsValid() {
		return nil, fmt.Errorf("no method %s", name)
	}
	var raw []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, err
	}
	mt := m.Type()
	if len(raw) != mt.NumIn() {
		return nil, fmt.Errorf("%s takes %d arguments, got %d", name, mt.NumIn(), len(raw))
	}
	args := make([]reflect.Value, len(raw))
	for i, rm := range raw {
		v := reflect.New(mt.In(i))
		if err := json.Unmarshal(rm, v.Interface()); err != nil {
			return nil, err
		}
		args[i] = v.Elem()
	}
	res := m.Call(args)
	if n := len(res); n > 0 && mt.Out(n-1) == reflect.TypeFor[error]() {
		if err, _ := res[n-1].Interface().(error); err != nil {
			return nil, err
		}
		res = res[:n-1]
	}
	if len(res) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(res[0].Interface())
}

const harnessShim = `
(() => {
  const call = (m) => (...args) =>
    fetch("/call/" + m, { method: "POST", body: JSON.stringify(args) }).then(async (r) => {
      const t = await r.text();
      if (!r.ok) throw t.trim();
      return JSON.parse(t);
    });
  window.go = { main: { App: new Proxy({}, { get: (_, m) => call(m) }) } };
  const handlers = {};
  new EventSource("/events").onmessage = (e) => {
    const { name, data } = JSON.parse(e.data);
    (handlers[name] || []).forEach((f) => f(data));
  };
  window.runtime = {
    EventsOn(n, f) {
      (handlers[n] ||= []).push(f);
      return () => (handlers[n] = handlers[n].filter((x) => x !== f));
    },
    ClipboardSetText: async (t) => { try { await navigator.clipboard.writeText(t); } catch {} return true; },
    BrowserOpenURL: (u) => console.log("open", u),
  };
})();
`
