package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justin06lee/caveira/core/cloud"
	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// site is a stand-in for the caveira site: a device login that waits
// until it is approved or denied, the account and its plan, and the
// account's model requests, which go to model.
type site struct {
	*httptest.Server
	token, email string

	mu       sync.Mutex
	plan     string
	approved bool
	denied   bool
	revoked  bool
	logouts  int
	// bearers is what each model request was signed with.
	bearers []string
}

func newSite(t *testing.T, model http.Handler) *site {
	t.Helper()
	s := &site{token: "tok_rebel", email: "rebel@caveira.ai", plan: "free"}
	siteError := func(w http.ResponseWriter, status int, code, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg, "type": "caveira", "code": code}})
	}
	// authed says the request carries the token the site still takes.
	authed := func(r *http.Request) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return r.Header.Get("Authorization") == "Bearer "+s.token && !s.revoked
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dc", "user_code": "ABCD-EFGH",
			"verification_uri_complete": s.URL + "/device?code=ABCD-EFGH",
			"interval":                  1, "expires_in": 600,
		})
	})
	mux.HandleFunc("POST /api/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		approved, denied := s.approved, s.denied
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case denied:
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"access_denied"}`)
		case !approved:
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"authorization_pending"}`)
		default:
			json.NewEncoder(w).Encode(map[string]string{"token": s.token, "email": s.email})
		}
	})
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			siteError(w, http.StatusUnauthorized, "unauthorized", "Sign in again.")
			return
		}
		s.mu.Lock()
		p, _ := planByID(s.plan)
		s.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{
			"email": s.email,
			"plan":  map[string]any{"id": p.ID, "name": p.Name, "large": p.Large, "status": "active"},
			"usage": map[string]any{"used": 0.25, "resets_at": "2026-11-01T00:00:00Z"},
		})
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.logouts++
		s.mu.Unlock()
		io.WriteString(w, `{}`)
	})
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.bearers = append(s.bearers, r.Header.Get("Authorization"))
		s.mu.Unlock()
		if !authed(r) {
			siteError(w, http.StatusUnauthorized, "unauthorized", "This sign-in is no longer valid.")
			return
		}
		if model == nil {
			siteError(w, http.StatusInternalServerError, "", "no model here")
			return
		}
		model.ServeHTTP(w, r)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *site) set(f func(s *site)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// events keeps what the app tells the window, by name.
type events struct {
	mu  sync.Mutex
	got map[string][]any
}

func record(a *App) *events {
	e := &events{got: map[string][]any{}}
	a.sink = func(name string, data any) {
		e.mu.Lock()
		e.got[name] = append(e.got[name], data)
		e.mu.Unlock()
	}
	return e
}

// account waits for an "account" event that until takes.
func (e *events) account(t *testing.T, until func(AccountView) bool) AccountView {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		for _, d := range e.got["account"] {
			if v := d.(AccountView); until(v) {
				e.mu.Unlock()
				return v
			}
		}
		e.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the account")
	return AccountView{}
}

// watchQuickly has the app ask after the account every few milliseconds.
func watchQuickly(t *testing.T) {
	every := watchEvery
	watchEvery = 20 * time.Millisecond
	t.Cleanup(func() { watchEvery = every })
}

// Signed out, a message waits under a sign-in. Signing in opens the
// site's page with a code the window shows too, and once it is approved
// there, the chat is built again on the account and what it held goes.
func TestSigningInSendsWhatWasHeld(t *testing.T) {
	a, dir, st := appOn(t, "", scripted(sse("Hello.", "", "", "")))
	if _, err := a.SignOut(); err != nil {
		t.Fatal(err)
	}
	if _, ok := cloud.Load(); ok || st.logouts != 1 {
		t.Fatalf("still signed in after signing out; the site heard %d times", st.logouts)
	}
	ev := record(a)
	opened := make(chan string, 4)
	a.browse = func(url string) error { opened <- url; return nil }

	v, err := a.NewChat(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"hi", "and this"} {
		if started, err := a.Send(v.ID, text); err != nil || started {
			t.Fatalf("send %q signed out: started %v, %v", text, started, err)
		}
	}
	v, _ = a.OpenChat(v.ID)
	if got := kinds(v.Items); got != "user user paywall" || v.Items[2].Text != "" {
		t.Fatalf("held: %q, paywall %q", got, v.Items[2].Text)
	}

	acct, err := a.SignIn()
	if err != nil {
		t.Fatal(err)
	}
	if !acct.Waiting || acct.Code != "ABCD-EFGH" || acct.SignedIn {
		t.Fatalf("waiting: %+v", acct)
	}
	if url := <-opened; url != st.URL+"/device?code=ABCD-EFGH" {
		t.Fatalf("opened %q", url)
	}
	// Asked again while waiting, the same page opens again.
	if again, err := a.SignIn(); err != nil || again.Code != "ABCD-EFGH" || <-opened != st.URL+"/device?code=ABCD-EFGH" {
		t.Fatalf("again: %+v, %v", again, err)
	}
	st.set(func(s *site) { s.approved = true })

	acct = ev.account(t, func(v AccountView) bool { return v.SignedIn })
	if acct.Email != "rebel@caveira.ai" || acct.Plan != "free" || acct.PlanName != "Free" || acct.Used != 0.25 || acct.Waiting {
		t.Fatalf("signed in: %+v", acct)
	}
	v = waitIdle(t, a, v.ID, func(v ChatView) bool { return !v.Running && kinds(v.Items) != "user user paywall" })
	if got := kinds(v.Items); got != "user user assistant" || v.NeedsKey {
		t.Fatalf("after signing in: %q, needs key %v", got, v.NeedsKey)
	}
	st.mu.Lock()
	bearers := st.bearers
	st.mu.Unlock()
	if len(bearers) != 1 || bearers[0] != "Bearer tok_rebel" {
		t.Fatalf("the model was asked with %q", bearers)
	}
	a.mu.Lock()
	var users []string
	for _, m := range a.chats[v.ID].ag.Messages {
		if m.Role == llm.RoleUser {
			users = append(users, m.Content)
		}
	}
	a.mu.Unlock()
	if len(users) != 2 || users[0] != "hi" || users[1] != "and this" {
		t.Fatalf("the model was sent %q", users)
	}
}

// A sign-in cancelled in the window stops waiting; one cancelled in the
// browser says so.
func TestSignInCancelled(t *testing.T) {
	a, _, st := appOn(t, "", nil)
	if _, err := a.SignOut(); err != nil {
		t.Fatal(err)
	}
	ev := record(a)

	if v, err := a.SignIn(); err != nil || !v.Waiting {
		t.Fatalf("%+v, %v", v, err)
	}
	if v := a.CancelSignIn(); v.Waiting || v.Code != "" || v.SignedIn {
		t.Fatalf("after cancelling: %+v", v)
	}

	if _, err := a.SignIn(); err != nil {
		t.Fatal(err)
	}
	st.set(func(s *site) { s.denied = true })
	v := ev.account(t, func(v AccountView) bool { return v.Error != "" })
	if v.SignedIn || v.Waiting || !strings.Contains(v.Error, "cancelled in the browser") {
		t.Fatalf("denied: %+v", v)
	}
	if _, ok := cloud.Load(); ok {
		t.Fatal("signed in after a denied sign-in")
	}
}

// Signed in, chats go through the account with its token, the models are
// the catalog's, and a token the site stops taking asks for a sign-in
// again, there and for what is sent after.
func TestRevokedSignInAsksAgain(t *testing.T) {
	a, dir, st := appOn(t, "", scripted(sse("Hi.", "", "", "")))
	cfg, _ := config.Load(dir)
	if !cfg.OnAccount() || cfg.BaseURL != st.URL+"/api/v1" || cfg.APIKey != st.token {
		t.Fatalf("not on the account: %+v", cfg)
	}
	models, err := a.Models()
	if err != nil || len(models) != len(catalog) || models[0].Name != "Qwen3.5" {
		t.Fatalf("models on the account: %+v, %v", models, err)
	}
	ev := record(a)

	v, err := a.NewChat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Send(v.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, a, v.ID, func(v ChatView) bool { return !v.Running && len(v.Items) == 2 })

	st.set(func(s *site) { s.revoked = true })
	if started, err := a.Send(v.ID, "again"); err != nil || !started {
		t.Fatalf("send: started %v, %v", started, err)
	}
	v = waitIdle(t, a, v.ID, func(v ChatView) bool { return !v.Running && len(v.Items) == 4 })
	last := v.Items[3]
	if last.Kind != "notice" || last.Action != "sign-in" || !strings.Contains(last.Text, "Sign in again") {
		t.Fatalf("after the site turned it away: %+v", last)
	}
	// The turn asks the site after the account once it is over.
	acct := ev.account(t, func(v AccountView) bool { return v.Expired })
	if acct.SignedIn || acct.Email != "rebel@caveira.ai" {
		t.Fatalf("account after a 401: %+v", acct)
	}
	if started, err := a.Send(v.ID, "and now?"); err != nil || started {
		t.Fatalf("send on a revoked sign-in: started %v, %v", started, err)
	}
	v, _ = a.OpenChat(v.ID)
	if n := len(v.Items); v.Items[n-1].Kind != "paywall" || v.Items[n-1].Text != "" {
		t.Fatalf("want it held for a sign-in: %q", kinds(v.Items))
	}
}

// Signing in or out in the terminal reaches the window the next time it
// asks after the account, and chats are built again for it.
func TestAccountChangedElsewhere(t *testing.T) {
	a, _, _ := appOn(t, "", nil)
	a.mu.Lock()
	gen := a.gen
	a.mu.Unlock()
	if v := a.Account(); !v.SignedIn {
		t.Fatalf("%+v", v)
	}
	if err := os.Remove(cloud.Path()); err != nil {
		t.Fatal(err)
	}
	if v := a.Account(); v.SignedIn || v.Expired || v.Email != "" {
		t.Fatalf("signed out in the terminal: %+v", v)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.gen != gen+1 {
		t.Fatalf("gen %d, was %d", a.gen, gen)
	}
}

// A desktop.json from when the plan was picked in the app still reads,
// and the plan in it is let go.
func TestOldPrefsStillRead(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"projects":["/tmp"],"theme":"dark","onboarded":true,"plan":"lightweight"}`
	if err := os.WriteFile(prefsPath(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	p := loadPrefs()
	if p.Theme != "dark" || !p.Onboarded || len(p.Projects) != 1 {
		t.Fatalf("prefs %+v", p)
	}
	if err := p.save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(config.Dir(), "desktop.json"))
	if strings.Contains(string(b), "plan") {
		t.Fatalf("desktop.json: %s", b)
	}
}
