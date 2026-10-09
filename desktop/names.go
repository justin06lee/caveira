package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// A new chat is named by the small model from its first message. Until
// the name comes, and if it never does, the chat goes by that message's
// first line.

// nameModel names chats: abliterated-model, Qwen3.5, which every plan
// runs.
const nameModel = config.DefaultModel

const namePrompt = "You name conversations with a coding agent. Reply with a short title for the conversation " +
	"that begins with the user's message: two to six words saying what it is about, in sentence case, " +
	"with no quotes and no full stop. Reply with the title and nothing else."

// nameLimit is how much of a long first message the model reads, in
// runes; the start says what it is about.
const nameLimit = 4000

// nameChat asks the small model for a chat's name from its first message,
// without reasoning, so it comes back in a moment.
func nameChat(ctx context.Context, client *llm.Client, first string) (string, error) {
	if r := []rune(first); len(r) > nameLimit {
		first = string(r[:nameLimit])
	}
	comp, err := client.Stream(ctx, llm.Request{
		Model: nameModel,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: namePrompt},
			{Role: llm.RoleUser, Content: first},
		},
		MaxTokens:       64,
		ReasoningEffort: "none",
	}, func(llm.Delta) error { return nil })
	if err != nil {
		return "", err
	}
	name := cleanName(comp.Message.Content)
	if name == "" {
		return "", errors.New("the model gave no name")
	}
	return name, nil
}

// cleanName is the title in a model's reply, without the thinking,
// quotes, or full stop it may come with.
func cleanName(s string) string {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const marks = " \t\"'`*#“”‘’"
	s = strings.Trim(s, marks)
	if len(s) > 6 && strings.EqualFold(s[:6], "title:") {
		s = strings.Trim(s[6:], marks)
	}
	s = strings.TrimRight(s, ".。")
	if r := []rune(s); len(r) > 60 {
		s = strings.TrimSpace(string(r[:60])) + "…"
	}
	return s
}

// name has the small model name a chat that has none yet, once. It is
// asked as the first turn starts; the name is kept when it comes, and
// told to the window.
func (a *App) name(c *chat) {
	a.mu.Lock()
	first := ""
	for _, it := range c.items {
		if it.Kind == "user" {
			first = it.Text
			break
		}
	}
	if a.namer == nil || c.title != "" || c.naming || first == "" {
		a.mu.Unlock()
		return
	}
	c.naming = true
	client := llm.New(c.ag.Client.BaseURL, c.ag.Client.APIKey)
	namer := a.namer
	a.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		title, err := namer(ctx, client, first)
		if err != nil || title == "" {
			return
		}
		a.mu.Lock()
		if a.chats[c.id] != c {
			// Deleted while it was being named.
			a.mu.Unlock()
			return
		}
		c.title, c.named = title, true
		if c.cancel == nil {
			c.keepName()
		}
		a.mu.Unlock()
		a.emit("chat", ChatEvent{Chat: c.id, Type: "title", Title: title})
	}()
}

// keepName writes the model's name for the chat into its session, once
// the session is on disk. Caller holds App.mu, with no turn running: a
// turn saves the session itself.
func (c *chat) keepName() {
	s := c.ag.Session
	if !c.named || s == nil || s.Title == c.title || len(s.Messages) == 0 {
		return
	}
	s.Title = c.title
	_ = s.Save()
}
