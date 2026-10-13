package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

// site is a stand-in for the caveira site: the login is approved on the
// third poll.
func site(t *testing.T, deny bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var polls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["client"] != "tui" || in["label"] != "box" {
			t.Errorf("start got %v", in)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dc", "user_code": "ABCD-EFGH",
			"verification_uri_complete": "http://site/device?code=ABCD-EFGH",
			"interval":                  0, "expires_in": 600,
		})
	})
	mux.HandleFunc("POST /api/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		if polls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		if deny {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"access_denied"}`))
			return
		}
		w.Write([]byte(`{"token":"tok","email":"a@b.c"}`))
	})
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Sign in again","type":"caveira"}}`))
			return
		}
		w.Write([]byte(`{"email":"a@b.c","plan":{"id":"lightweight","name":"Lightweight","large":true,"status":"active"},"usage":{"used":0.25,"resets_at":"2026-11-01T00:00:00Z"}}`))
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &polls
}

func TestLoginKeepsTheAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv, polls := site(t, false)
	ctx := context.Background()

	c, err := Start(ctx, srv.URL, "tui", "box")
	if err != nil {
		t.Fatal(err)
	}
	if c.UserCode != "ABCD-EFGH" || c.Interval != 2 {
		t.Fatalf("code %+v", c)
	}
	c.Interval = 0 // no waiting in the test
	a, err := Wait(ctx, srv.URL, Code{DeviceCode: c.DeviceCode})
	if err != nil {
		t.Fatal(err)
	}
	if polls.Load() != 3 || a.Token != "tok" || a.Email != "a@b.c" {
		t.Fatalf("after %d polls: %+v", polls.Load(), a)
	}
	got, ok := Load()
	if !ok || got != a {
		t.Fatalf("kept %+v %v", got, ok)
	}
	if st, _ := os.Stat(Path()); st.Mode().Perm() != 0o600 {
		t.Fatalf("auth.json is %v", st.Mode().Perm())
	}
	if a.APIBase() != srv.URL+"/api/v1" {
		t.Fatalf("api base %q", a.APIBase())
	}

	acct, err := Me(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if acct.Plan.ID != "lightweight" || !acct.Plan.Large || acct.Usage.Used != 0.25 {
		t.Fatalf("account %+v", acct)
	}
	_, err = Me(ctx, Auth{URL: srv.URL, Token: "old"})
	if !Unauthorized(err) || err.Error() != "Sign in again" {
		t.Fatalf("stale token: %v", err)
	}

	if err := Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(); ok {
		t.Fatal("still signed in after logout")
	}
}

func TestLoginCancelledInTheBrowser(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv, _ := site(t, true)
	_, err := Wait(context.Background(), srv.URL, Code{DeviceCode: "dc"})
	if err != ErrDenied {
		t.Fatalf("got %v", err)
	}
	if _, ok := Load(); ok {
		t.Fatal("signed in after a denied login")
	}
}

func TestURLFromEnvironment(t *testing.T) {
	t.Setenv("CAVEIRA_CLOUD_URL", "http://localhost:3000/")
	if URL() != "http://localhost:3000" {
		t.Fatal(URL())
	}
	t.Setenv("CAVEIRA_CLOUD_URL", "")
	if URL() != DefaultURL {
		t.Fatal(URL())
	}
}
