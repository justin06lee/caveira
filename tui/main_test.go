package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/justin06lee/caveira/core/cloud"
)

// isolate gives the test a home and an environment of its own, signing in
// through site: nothing touches the real ~/.caveira or uses the
// developer's keys.
func isolate(t *testing.T, site string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CAVEIRA_CLOUD_URL", site)
	for _, k := range []string{"CAVEIRA_API_KEY", "ABLITERATION_API_KEY", "ABLIT_KEY", "CAVEIRA_BASE_URL", "ABLITERATION_BASE_URL"} {
		t.Setenv(k, "")
	}
}

// site stands in for the caveira site. /api/me takes only the token the
// login hands out.
func site(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var logouts atomic.Int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("POST /api/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dc", "user_code": "WXYZ-1234",
			"verification_uri_complete": srv.URL + "/device?code=WXYZ-1234",
			"interval":                  1, "expires_in": 600,
		})
	})
	mux.HandleFunc("POST /api/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok","email":"rebel@caveira.dev"}`))
	})
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Sign in again","type":"caveira","code":"unauthorized"}}`))
			return
		}
		w.Write([]byte(`{"email":"rebel@caveira.dev","plan":{"id":"free","name":"Free"}}`))
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		logouts.Add(1)
		w.Write([]byte(`{}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &logouts
}

// caveira login prints the page and the code, opens the browser, and says
// who it signed in as; run again it says so, unless the site has stopped
// taking the token. caveira logout tells the site and forgets it here.
func TestLoginAndLogoutCommands(t *testing.T) {
	srv, logouts := site(t)
	isolate(t, srv.URL)

	var out bytes.Buffer
	var opened []string
	open := func(u string) error { opened = append(opened, u); return nil }
	if err := login(&out, open); err != nil {
		t.Fatal(err)
	}
	page := srv.URL + "/device?code=WXYZ-1234"
	for _, want := range []string{"Opening your browser", page, "WXYZ-1234", "Signed in as rebel@caveira.dev"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("login printed\n%s\nmissing %q", out.String(), want)
		}
	}
	if len(opened) != 1 || opened[0] != page {
		t.Fatalf("browser opened at %v, want %s", opened, page)
	}
	if a, ok := cloud.Load(); !ok || a.Token != "tok" {
		t.Fatal("login kept no account")
	}

	out.Reset()
	if err := login(&out, open); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "Already signed in as rebel@caveira.dev") || len(opened) != 1 {
		t.Fatalf("login while signed in printed %q", out.String())
	}

	// A token the site no longer takes is replaced, and where the browser
	// cannot open, the page is still there to visit.
	cloud.Save(cloud.Auth{URL: srv.URL, Token: "stale", Email: "rebel@caveira.dev"})
	out.Reset()
	if err := login(&out, func(string) error { return os.ErrNotExist }); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "To sign in to caveira, visit\n\n  "+page) || !strings.Contains(out.String(), "Signed in as") {
		t.Fatalf("login over a rejected token printed\n%s", out.String())
	}

	out.Reset()
	if err := logout(&out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Signed out of rebel@caveira.dev\n" || logouts.Load() != 1 {
		t.Fatalf("logout printed %q, site told %d times", out.String(), logouts.Load())
	}
	if _, ok := cloud.Load(); ok {
		t.Fatal("logout kept the account")
	}
	out.Reset()
	logout(&out)
	if out.String() != "Not signed in.\n" {
		t.Fatalf("logout signed out printed %q", out.String())
	}
}

// Signed in, an endpoint given with --base-url gets the request as given,
// never the account's token.
func TestBaseURLFlagNeverGetsTheAccountToken(t *testing.T) {
	srv, _ := site(t)
	isolate(t, srv.URL)
	cloud.Save(cloud.Auth{URL: srv.URL, Token: "tok", Email: "rebel@caveira.dev"})
	var sent atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(other.Close)

	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"caveira", "-C", t.TempDir(), "--base-url", "http://localhost:" + other.URL[len("http://127.0.0.1:"):], "-p", "hi"}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if got, _ := sent.Load().(string); got != "" {
		t.Fatalf("--base-url got Authorization %q", got)
	}
}

// -p with no account and no key fails at once and says to sign in.
func TestPrintModeSignedOutSaysToLogin(t *testing.T) {
	srv, _ := site(t)
	isolate(t, srv.URL)
	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"caveira", "-C", t.TempDir(), "-p", "explain main.go"}
	err := run()
	if err == nil || !strings.Contains(err.Error(), `Run "caveira login"`) {
		t.Fatalf("print mode signed out: %v", err)
	}
}
