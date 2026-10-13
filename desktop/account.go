package main

import (
	"context"
	"errors"
	"time"

	"github.com/justin06lee/caveira/core/cloud"
	"github.com/justin06lee/caveira/core/llm"
)

// account is the caveira account as the app last knew it. Signed in, model
// requests go through the site with the token (core/config sees to that),
// and the plan the site reports is what allows goes by.
type account struct {
	auth cloud.Auth
	// me is what the site last said about the account; known says it has
	// said anything yet.
	me    cloud.Account
	known bool
	// expired says the site no longer takes auth's token.
	expired bool
}

// signedIn says there is an account the site still takes.
func (ac account) signedIn() bool { return ac.auth.Token != "" && !ac.expired }

// signIn is a sign-in waiting on the browser.
type signIn struct {
	// code is what the browser shows too, and url the page that shows it;
	// both are "" while the site is still being asked for them.
	code, url string
	cancel    context.CancelFunc
}

// AccountView is the caveira account as the window shows it.
type AccountView struct {
	SignedIn bool   `json:"signedIn"`
	Email    string `json:"email,omitempty"`
	// Plan is the plan's id, "" until the site has said, and PlanName what
	// it is called.
	Plan     string `json:"plan,omitempty"`
	PlanName string `json:"planName,omitempty"`
	// Used is the share of this period's usage spent, 0 to 1, until
	// ResetsAt (RFC 3339).
	Used     float64 `json:"used"`
	ResetsAt string  `json:"resetsAt,omitempty"`
	// Expired says the site stopped taking this machine's sign-in, which
	// Email was: sign in again.
	Expired bool `json:"expired,omitempty"`
	// Code is a sign-in waiting on the browser, as both screens show it.
	Code string `json:"code,omitempty"`
	// Waiting says a sign-in has started, Code or not yet.
	Waiting bool `json:"waiting,omitempty"`
	// Site is the caveira website, for its account page.
	Site string `json:"site"`
	// Error, on the event that ends a sign-in, is why it did not go
	// through.
	Error string `json:"error,omitempty"`
}

// accountView is the account for the window. Caller holds App.mu.
func (a *App) accountView() AccountView {
	ac := a.acct
	v := AccountView{Site: a.siteLocked(), Expired: ac.expired && ac.auth.Token != ""}
	if ac.auth.Token != "" {
		v.Email = ac.auth.Email
	}
	if ac.signedIn() {
		v.SignedIn = true
		if ac.known {
			if ac.me.Email != "" {
				v.Email = ac.me.Email
			}
			v.Plan, v.PlanName = ac.me.Plan.ID, ac.me.Plan.Name
			v.Used, v.ResetsAt = ac.me.Usage.Used, ac.me.Usage.ResetsAt
		}
	}
	if a.signing != nil {
		v.Waiting, v.Code = true, a.signing.code
	}
	return v
}

// siteLocked is the site the account is on, or the one to sign in
// through. Caller holds App.mu.
func (a *App) siteLocked() string {
	if a.acct.auth.URL != "" {
		return a.acct.auth.URL
	}
	return cloud.URL()
}

// Account asks the site what the account is on now, and sends what chats
// were holding if it runs them. The window asks when it comes to the
// front.
func (a *App) Account() AccountView { return a.refreshAccount() }

// refreshAccount reads the account this machine is signed in to and asks
// the site about it, then tells the window. Signing in or out, here or in
// the terminal, changes where chats send their requests, so it bumps gen
// the way a settings change does.
func (a *App) refreshAccount() AccountView {
	auth, ok := cloud.Load()
	var me cloud.Account
	var err error
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		me, err = cloud.Me(ctx, auth)
		cancel()
	}

	a.mu.Lock()
	prev := a.acct
	switch {
	case !ok:
		a.acct = account{}
	case err == nil:
		a.acct = account{auth: auth, me: me, known: true}
	case cloud.Unauthorized(err):
		a.acct = account{auth: auth, expired: true}
	case prev.auth.Token != auth.Token:
		a.acct = account{auth: auth}
	default:
		// The site could not be asked: what it said last still holds.
	}
	if a.acct.auth.Token != prev.auth.Token {
		a.gen++
	}
	v := a.accountView()
	a.mu.Unlock()

	a.emit("account", v)
	a.releaseAll()
	return v
}

// SignIn signs this machine in to a caveira account. The site's page for
// it opens in the browser with a code, which the window shows too until
// it is used; it returns once the page is open, and "account" events say
// how it went. Asked again while waiting, it opens the page again.
func (a *App) SignIn() (AccountView, error) {
	a.mu.Lock()
	if s := a.signing; s != nil {
		url, v := s.url, a.accountView()
		a.mu.Unlock()
		if url == "" {
			return v, nil
		}
		return v, a.browse(url)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &signIn{cancel: cancel}
	a.signing = s
	a.mu.Unlock()

	site := cloud.URL()
	start, stop := context.WithTimeout(ctx, 30*time.Second)
	code, err := cloud.Start(start, site, "desktop", cloud.Label())
	stop()
	if err == nil {
		err = a.browse(code.VerifyURL)
	}
	a.mu.Lock()
	if err != nil || a.signing != s {
		// It failed, or was cancelled while the site was being asked.
		cancel()
		if a.signing == s {
			a.signing = nil
		}
		v := a.accountView()
		a.mu.Unlock()
		if err != nil {
			err = signInError(err)
		}
		return v, err
	}
	s.code, s.url = code.UserCode, code.VerifyURL
	v := a.accountView()
	a.mu.Unlock()
	a.emit("account", v)

	wait := 15 * time.Minute
	if code.ExpiresIn > 0 {
		wait = time.Duration(code.ExpiresIn)*time.Second + time.Minute
	}
	go func() {
		ctx, done := context.WithTimeout(ctx, wait)
		_, err := cloud.Wait(ctx, site, code)
		done()
		cancel()
		a.mu.Lock()
		mine := a.signing == s
		if mine {
			a.signing = nil
		}
		a.mu.Unlock()
		if !mine {
			return // cancelled, or another sign-in took its place
		}
		if err != nil {
			a.mu.Lock()
			v := a.accountView()
			a.mu.Unlock()
			v.Error = signInError(err).Error()
			a.emit("account", v)
			return
		}
		a.refreshAccount()
	}()
	return v, nil
}

// signInError says why signing in did not go through, for the window.
func signInError(err error) error {
	switch {
	case errors.Is(err, cloud.ErrDenied):
		return errors.New("signing in was cancelled in the browser")
	case errors.Is(err, cloud.ErrExpired), errors.Is(err, context.DeadlineExceeded):
		return errors.New("the sign-in code ran out before it was used; sign in again")
	}
	return err
}

// siteError is the caveira site's own answer to a model request, nil when
// err is not one: signed out (401), out of usage (402), or on a plan
// without the model (403).
func siteError(err error) *llm.APIError {
	var e *llm.APIError
	if errors.As(err, &e) && e.Type == "caveira" {
		return e
	}
	return nil
}

// CancelSignIn stops waiting on a sign-in started in the browser.
func (a *App) CancelSignIn() AccountView {
	a.mu.Lock()
	s := a.signing
	a.signing = nil
	v := a.accountView()
	a.mu.Unlock()
	if s != nil {
		s.cancel()
	}
	return v
}

// SignOut signs this machine out of the caveira account, on the site as
// well when it can be reached.
func (a *App) SignOut() (AccountView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	err := cloud.Logout(ctx)
	cancel()
	if _, still := cloud.Load(); still {
		a.mu.Lock()
		v := a.accountView()
		a.mu.Unlock()
		return v, err
	}
	// The token is gone from here; the site forgets it on its own if it
	// did not hear.
	return a.refreshAccount(), nil
}

// watchEvery and watchFor pace asking the site about the account while a
// chat holds a message, so a plan bought in the browser, or a sign-in
// finished in the terminal, sends it without the window being touched.
var (
	watchEvery = 5 * time.Second
	watchFor   = 10 * time.Minute
)

// watch asks the site about the account every watchEvery until no chat
// holds a message, or for watchFor since the last call.
func (a *App) watch() {
	a.mu.Lock()
	a.watchUntil = time.Now().Add(watchFor)
	if a.watching {
		a.mu.Unlock()
		return
	}
	a.watching = true
	a.mu.Unlock()

	tick := time.NewTicker(watchEvery)
	go func() {
		defer tick.Stop()
		for range tick.C {
			a.mu.Lock()
			if !time.Now().Before(a.watchUntil) || !a.holding() {
				a.watching = false
				a.mu.Unlock()
				return
			}
			a.mu.Unlock()
			a.refreshAccount()
		}
	}()
}

// holding says a chat is holding a message. Caller holds App.mu.
func (a *App) holding() bool {
	for _, c := range a.chats {
		if len(c.held) > 0 {
			return true
		}
	}
	return false
}
