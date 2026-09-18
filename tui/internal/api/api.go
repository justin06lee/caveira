// Package api talks to the caveira backend: the same HTTP API the website
// uses, with the CLI's session token sent as a bearer.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Error is a failed API call, carrying the message the server wrote for a
// human plus the status for the caller to branch on.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func rootCause(err error) string {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err.Error()
		}
		err = next
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		// Unwrapped, a transport failure reads as three nested errors that
		// repeat the URL. The screen showing this has one line for it.
		return &Error{Message: fmt.Sprintf("Cannot reach %s: %s", c.BaseURL, rootCause(err))}
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		var payload struct {
			Error  string `json:"error"`
			Status string `json:"status"`
		}
		_ = json.NewDecoder(res.Body).Decode(&payload)
		msg := payload.Error
		if msg == "" {
			msg = payload.Status
		}
		if msg == "" {
			msg = res.Status
		}
		return &Error{Status: res.StatusCode, Message: msg}
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

type Plan struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Tagline  string   `json:"tagline"`
	PriceUSD int      `json:"priceUsd"`
	Interval string   `json:"interval"`
	Features []string `json:"features"`
}

type PlansResponse struct {
	Plans   []Plan `json:"plans"`
	Billing string `json:"billing"` // "stripe" or "dev"
}

func (c *Client) Plans(ctx context.Context) (*PlansResponse, error) {
	var out PlansResponse
	if err := c.do(ctx, http.MethodGet, "/api/plans", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type Subscription struct {
	PlanID           string `json:"planId"`
	Status           string `json:"status"`
	CurrentPeriodEnd *int64 `json:"currentPeriodEnd"`
}

type Me struct {
	ID           string        `json:"id"`
	Email        string        `json:"email"`
	Name         *string       `json:"name"`
	Subscription *Subscription `json:"subscription"`
	HasAccess    bool          `json:"hasAccess"`
}

func (c *Client) Me(ctx context.Context) (*Me, error) {
	var out Me
	if err := c.do(ctx, http.MethodGet, "/api/me", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type DeviceStart struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURL         string `json:"verificationUrl"`
	VerificationURLComplete string `json:"verificationUrlComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

func (c *Client) StartDeviceLogin(ctx context.Context) (*DeviceStart, error) {
	var out DeviceStart
	if err := c.do(ctx, http.MethodPost, "/api/auth/device/start", struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type DevicePoll struct {
	Status       string        `json:"status"` // pending | approved | expired
	Token        string        `json:"token"`
	User         *Me           `json:"user"`
	Subscription *Subscription `json:"subscription"`
	HasAccess    bool          `json:"hasAccess"`
}

// PollDeviceLogin reports progress without treating "not yet" as a failure:
// pending and expired are states of a working flow, not errors.
func (c *Client) PollDeviceLogin(ctx context.Context, deviceCode string) (*DevicePoll, error) {
	var out DevicePoll
	err := c.do(ctx, http.MethodPost, "/api/auth/device/poll", map[string]string{"deviceCode": deviceCode}, &out)
	if err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) && (apiErr.Status == http.StatusGone || apiErr.Status == http.StatusNotFound) {
			return &DevicePoll{Status: "expired"}, nil
		}
		return nil, err
	}
	return &out, nil
}

type Checkout struct {
	Mode      string `json:"mode"` // stripe | dev
	URL       string `json:"url"`
	PlanID    string `json:"planId"`
	Activated bool   `json:"activated"`
}

func (c *Client) Checkout(ctx context.Context, planID string) (*Checkout, error) {
	var out Checkout
	if err := c.do(ctx, http.MethodPost, "/api/billing/checkout", map[string]string{"planId": planID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
