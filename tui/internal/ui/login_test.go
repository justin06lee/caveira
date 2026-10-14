package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/cloud"
	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// fakeSite stands in for the caveira site: the device login, logout, and
// the account's OpenAI-compatible endpoint, which takes only the token
// the login handed out.
type fakeSite struct {
	*httptest.Server
	deny bool
	// unauthorized is what the endpoint says to any other token.
	unauthorized string
	logouts      atomic.Int32

	mu      sync.Mutex
	bearers []string // what each model request was sent with
}

func newSite(t *testing.T) *fakeSite {
	t.Helper()
	s := &fakeSite{unauthorized: "Your caveira session has ended. Run /login to sign in again."}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["client"] != "tui" {
			t.Errorf("login started as client %q, want tui", in["client"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dc", "user_code": "WXYZ-1234",
			"verification_uri_complete": s.URL + "/device?code=WXYZ-1234",
			"interval":                  1, "expires_in": 600,
		})
	})
	mux.HandleFunc("POST /api/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		if s.deny {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"access_denied"}`))
			return
		}
		w.Write([]byte(`{"token":"tok","email":"rebel@caveira.ai"}`))
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s.logouts.Add(1)
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("POST /api/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		bearer := r.Header.Get("Authorization")
		s.mu.Lock()
		s.bearers = append(s.bearers, bearer)
		s.mu.Unlock()
		if bearer != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"message":%q,"type":"caveira","code":"unauthorized"}}`, s.unauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Ready when you are.\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *fakeSite) sentWith() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bearers...)
}

// signedOutSession is a session with no account and no key, signing in
// through site, in a home and an environment of its own: nothing here
// touches the real ~/.caveira or picks up the developer's keys.
func signedOutSession(t *testing.T, site *fakeSite) (*Model, *[]string) {
	t.Helper()
	m := session(t, 100, 40)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CAVEIRA_CLOUD_URL", site.URL)
	for _, k := range []string{"CAVEIRA_API_KEY", "ABLITERATION_API_KEY", "ABLIT_KEY", "CAVEIRA_BASE_URL", "ABLITERATION_BASE_URL"} {
		t.Setenv(k, "")
	}
	m.workDir = t.TempDir()
	m.cfg.BaseURL, m.cfg.APIKey = config.DefaultBaseURL, ""
	m.agent.Client = llm.New(config.DefaultBaseURL, "")

	var opened []string
	openBrowser = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openBrowser = cloud.OpenBrowser })
	return m, &opened
}

// results runs cmd the way the program would and returns the messages it
// produces, leaving out the animation clock's.
func results(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, results(c)...)
		}
		return out
	case tickMsg, clearNoticeMsg:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// settle feeds what cmd produces back into the model, and what that
// produces, until nothing is left: a whole turn, say.
func settle(m *Model, cmd tea.Cmd) {
	for _, msg := range results(cmd) {
		_, next := m.Update(msg)
		settle(m, next)
	}
}

// codeFor runs /login up to the site handing out its code, and returns
// that message with the poll interval taken out so the test does not wait.
func codeFor(t *testing.T, m *Model) loginCodeMsg {
	t.Helper()
	msgs := results(m.slash("/login"))
	if len(msgs) != 1 {
		t.Fatalf("/login produced %d messages, want the code", len(msgs))
	}
	msg, ok := msgs[0].(loginCodeMsg)
	if !ok {
		t.Fatalf("/login produced %T, want loginCodeMsg", msgs[0])
	}
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	msg.code.Interval = 0
	return msg
}

func last(m *Model) *item { return m.items[len(m.items)-1] }

// /login opens the site's page with the code, says where it is and what
// code to look for, waits without holding up the input, and then the same
// session, transcript and all, sends its requests through the account.
func TestLoginSwitchesTheSessionToTheAccount(t *testing.T) {
	site := newSite(t)
	m, opened := signedOutSession(t, site)
	id := m.agent.NewSession().ID
	m.agent.Client.Headers["x-abliteration-session-id"] = id
	before := len(m.items)

	m.submit("tighten the greeting")
	if it := last(m); it.kind != itemError || !strings.Contains(it.text, "/login") {
		t.Fatalf("a message sent signed out says %q, want it to point to /login", it.text)
	}

	code := codeFor(t, m)
	if m.login == nil || !strings.Contains(plain(m.renderStatus(m.inner())), "Asking caveira for a login code") {
		t.Fatal("no login under way while the site hands out the code")
	}
	_, wait := m.Update(code)
	page := site.URL + "/device?code=WXYZ-1234"
	if len(*opened) != 1 || (*opened)[0] != page {
		t.Fatalf("browser opened at %v, want %s", *opened, page)
	}
	frame := plain(m.View().Content)
	dump(t, "login-waiting", m.View().Content)
	for _, want := range []string{"Opening your browser", page, "WXYZ-1234", "Waiting for you to sign in in the browser", "esc to cancel"} {
		if !strings.Contains(frame, want) {
			t.Errorf("waiting frame missing %q", want)
		}
	}
	// The wait is in the background: typing still goes into the box.
	m.handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.input.Value() != "x" {
		t.Fatal("the input is held up while the login waits")
	}
	m.input.Reset()

	settle(m, wait)
	if it := last(m); it.kind != itemNotice || !strings.HasPrefix(it.text, "Signed in as rebel@caveira.ai") {
		t.Fatalf("after the browser approved: %+v", it)
	}
	if m.login != nil || m.signedOut() {
		t.Fatal("still waiting, or still signed out, after signing in")
	}
	c := m.agent.Client
	if c.BaseURL != site.URL+"/api/v1" || c.APIKey != "tok" || c.Headers["x-abliteration-session-id"] != id {
		t.Fatalf("client after sign-in: %s with %q, headers %v", c.BaseURL, c.APIKey, c.Headers)
	}
	card := m.rend.render(m.items[0])
	dump(t, "login-card", card)
	if !m.cfg.OnAccount() || !strings.Contains(plain(card), "account  rebel@caveira.ai") {
		t.Fatal("the session card does not say who is signed in")
	}
	if m.items[1].kind != itemUser || len(m.items) <= before {
		t.Fatal("the transcript did not survive signing in")
	}

	settle(m, m.submit("are you there?"))
	if got := site.sentWith(); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("model requests went with %v, want the account's token", got)
	}
	if !strings.Contains(plain(m.View().Content), "Ready when you are.") {
		t.Fatal("the account's reply is not in the transcript")
	}
	if m.agent.Session.ID != id {
		t.Fatalf("session %s after signing in, want %s kept", m.agent.Session.ID, id)
	}
}

// Esc, or /login a second time, calls a waiting login off, and what the
// site sends after that changes nothing.
func TestLoginCanBeCalledOff(t *testing.T) {
	site := newSite(t)
	m, opened := signedOutSession(t, site)

	code := codeFor(t, m)
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.login != nil || last(m).text != "login cancelled" {
		t.Fatalf("esc did not call the login off: %+v", last(m))
	}
	n := len(m.items)
	if _, cmd := m.Update(code); cmd != nil || len(m.items) != n || len(*opened) != 0 {
		t.Fatal("a code that came after esc still opened the browser")
	}

	code = codeFor(t, m)
	_, wait := m.Update(code)
	m.slash("/login")
	if m.login != nil || last(m).text != "login cancelled" {
		t.Fatalf("a second /login did not call the first off: %+v", last(m))
	}
	n = len(m.items)
	settle(m, wait)
	if len(m.items) != n || !m.signedOut() {
		t.Fatal("a called-off login still signed in")
	}
	if _, ok := cloud.Load(); ok {
		t.Fatal("a called-off login kept a token")
	}
}

// A login the browser turns down, or a site that cannot be reached, ends
// in a plain line saying so.
func TestLoginSaysWhyItFailed(t *testing.T) {
	site := newSite(t)
	site.deny = true
	m, _ := signedOutSession(t, site)
	_, wait := m.Update(codeFor(t, m))
	settle(m, wait)
	if it := last(m); it.kind != itemError || it.text != "the login was cancelled in the browser · /login to try again" {
		t.Fatalf("denied login: %+v", it)
	}
	if m.login != nil || !m.signedOut() {
		t.Fatal("a denied login left the session waiting or signed in")
	}

	site.Close()
	settle(m, m.slash("/login"))
	if it := last(m); it.kind != itemError || !strings.HasPrefix(it.text, "cannot start the login: cannot reach") {
		t.Fatalf("unreachable site: %+v", it)
	}
	if m.login != nil {
		t.Fatal("still waiting after the site could not be reached")
	}
}

// Signed in already, /login says as whom and changes nothing.
func TestLoginWhenSignedInSaysWho(t *testing.T) {
	site := newSite(t)
	m, opened := signedOutSession(t, site)
	if err := cloud.Save(cloud.Auth{URL: site.URL, Token: "tok", Email: "rebel@caveira.ai"}); err != nil {
		t.Fatal(err)
	}
	m.useSettings()
	if cmd := m.slash("/login"); cmd != nil || m.login != nil || len(*opened) != 0 {
		t.Fatal("/login started another login while signed in")
	}
	if it := last(m); it.kind != itemNotice || it.text != "Signed in as rebel@caveira.ai\n/logout first to use another account" {
		t.Fatalf("/login signed in: %+v", it)
	}
}

// /logout ends the session on the site and here. A key of one's own for
// abliteration.ai does not stand in for the account, so the session then
// says to /login, and so does any message sent.
func TestLogoutSaysToLoginAgain(t *testing.T) {
	site := newSite(t)
	m, _ := signedOutSession(t, site)
	t.Setenv("CAVEIRA_API_KEY", "dev-key")
	cloud.Save(cloud.Auth{URL: site.URL, Token: "tok", Email: "rebel@caveira.ai"})
	m.useSettings()
	if m.agent.Client.APIKey != "tok" {
		t.Fatal("signed in, the account should win over the environment's key")
	}

	settle(m, m.slash("/logout"))
	if it := last(m); it.kind != itemNotice || it.text != "Signed out\n/login to sign in again" {
		t.Fatalf("after /logout: %+v", it)
	}
	if site.logouts.Load() != 1 {
		t.Fatal("the site was not told")
	}
	if m.items[0].header.account != "" {
		t.Fatal("the session card still names the account")
	}
	m.submit("hello?")
	if it := last(m); it.kind != itemError || it.text != signInHint {
		t.Fatalf("a message signed out, with a key of its own: %+v", it)
	}

	m.slash("/logout")
	if it := last(m); it.text != "not signed in" {
		t.Fatalf("/logout signed out: %+v", it)
	}
}

// Only the account reaches abliteration.ai; a server set on purpose needs
// its own key, or none on this machine.
func TestKeyMissing(t *testing.T) {
	for _, c := range []struct {
		name    string
		s       config.Settings
		missing bool
	}{
		{"own key for abliteration.ai", config.Settings{BaseURL: config.DefaultBaseURL, APIKey: "own"}, true},
		{"nothing", config.Settings{BaseURL: config.DefaultBaseURL}, true},
		{"account", config.Settings{BaseURL: "https://caveira.ai/api/v1", APIKey: "tok", KeySource: config.AccountSource}, false},
		{"another server with its key", config.Settings{BaseURL: "https://models.example/v1", APIKey: "own"}, false},
		{"another server without one", config.Settings{BaseURL: "https://models.example/v1"}, true},
		{"a server on this machine", config.Settings{BaseURL: "http://localhost:11434/v1"}, false},
	} {
		if got := KeyMissing(c.s); got != c.missing {
			t.Errorf("%s: KeyMissing = %v, want %v", c.name, got, c.missing)
		}
	}
}

// A token the site no longer takes ends the turn with the site's own
// word, which points to /login, and /login then signs in again.
func TestRejectedTokenPointsToLogin(t *testing.T) {
	site := newSite(t)
	m, _ := signedOutSession(t, site)
	cloud.Save(cloud.Auth{URL: site.URL, Token: "stale", Email: "rebel@caveira.ai"})
	m.useSettings()

	settle(m, m.submit("hello"))
	it := last(m)
	for i := len(m.items) - 1; i >= 0 && it.kind != itemError; i-- {
		it = m.items[i]
	}
	if it.text != site.unauthorized {
		t.Fatalf("401 from the account shows %q, want the site's message as is", it.text)
	}
	if !strings.Contains(plain(m.View().Content), "Run /login to sign in again") {
		t.Fatal("the site's message is not on screen")
	}

	site.unauthorized = "Unauthorized"
	settle(m, m.submit("hello again"))
	found := false
	for _, it := range m.items {
		found = found || it.kind == itemError && it.text == "Unauthorized · /login to sign in again"
	}
	if !found {
		t.Fatal("a 401 that does not mention /login gets no pointer to it")
	}

	if _, wait := m.Update(codeFor(t, m)); wait == nil {
		t.Fatal("/login with a rejected token did not sign in again")
	}
}

// Started with no account and no key, the home screen says to /login,
// the palette still opens over it, and a resumed session says it too.
func TestSignedOutStartSaysToLogin(t *testing.T) {
	site := newSite(t)
	m, _ := signedOutSession(t, site)
	m.items = nil
	m.phase = phaseHero
	m.layout()
	frame, _ := m.renderHome(0)
	dump(t, "home-signed-out", frame)
	if p := plain(frame); !strings.Contains(p, "not signed in · /login to sign in to your caveira account") {
		t.Fatal("the home screen does not say to /login")
	}
	m.input.SetValue("/lo")
	frame, _ = m.renderHome(0)
	if p := plain(frame); !strings.Contains(p, "/login") || !strings.Contains(p, "/logout") || !strings.Contains(p, "sign in to your caveira account in the browser") {
		t.Fatal("the palette does not open on the signed-out home screen")
	}

	r := New("v0.4.0", nil)
	ag := agent.New(llm.New(config.DefaultBaseURL, ""), config.Settings{Model: "abliterated-model"}, m.workDir, "sys")
	s := ag.NewSession()
	r.Apply(Options{Agent: ag, Settings: config.Settings{Model: "abliterated-model", BaseURL: config.DefaultBaseURL}, WorkDir: m.workDir, Resumed: s})
	if it := last(r); it.kind != itemNotice || it.text != signInHint {
		t.Fatalf("a resumed session with no key ends with %+v", it)
	}
}
