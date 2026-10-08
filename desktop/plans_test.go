package main

import (
	"testing"

	"github.com/justin06lee/caveira/core/llm"
)

// Without a plan a message is held under the plans, and goes, with any
// sent after it, once one is picked.
func TestNoPlanHoldsTheMessage(t *testing.T) {
	srv := fakeModel(t, sse("Hello.", "", "", ""))
	a, dir := testApp(t, srv)
	a.prefs.Plan = ""

	v, err := a.NewChat(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"hi", "and this"} {
		started, err := a.Send(v.ID, text)
		if err != nil || started {
			t.Fatalf("send %q without a plan: started %v, %v", text, started, err)
		}
	}
	v, _ = a.OpenChat(v.ID)
	if got := kinds(v.Items); got != "user user paywall" || v.Running {
		t.Fatalf("held: %q running %v", got, v.Running)
	}

	if _, err := a.Subscribe("nope", v.ID); err == nil {
		t.Fatal("subscribed to a plan that is not there")
	}
	if _, err := a.Subscribe("free", v.ID); err != nil {
		t.Fatal(err)
	}
	v = waitIdle(t, a, v.ID, func(v ChatView) bool { return !v.Running && len(v.Items) == 3 })
	if got := kinds(v.Items); got != "user user assistant" {
		t.Fatalf("after subscribing: %q", got)
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
	if NewApp("again").prefs.Plan != "free" {
		t.Fatal("the plan was not kept")
	}
}

// Free runs abliterated-model, not the large ones; moving to one of them
// holds the message until the plan is big enough.
func TestFreeHoldsTheLargeModels(t *testing.T) {
	srv := fakeModel(t, sse("Big hello.", "", "", ""))
	a, dir := testApp(t, srv)

	v, err := a.NewChat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModel(v.ID, "abliterated-model-large-v2", 0, ""); err != nil {
		t.Fatal(err)
	}
	if started, err := a.Send(v.ID, "hi"); err != nil || started {
		t.Fatalf("large model on free: started %v, %v", started, err)
	}
	v, _ = a.OpenChat(v.ID)
	if got := kinds(v.Items); got != "user paywall" || v.Items[1].Text != "GLM-5.3" {
		t.Fatalf("held: %q, %+v", got, v.Items)
	}
	if _, err := a.Subscribe("lightweight", v.ID); err != nil {
		t.Fatal(err)
	}
	v = waitIdle(t, a, v.ID, func(v ChatView) bool { return !v.Running && len(v.Items) == 2 })
	if got := kinds(v.Items); got != "user assistant" || v.Items[1].Text != "Big hello." {
		t.Fatalf("after Lightweight: %q", got)
	}
}

func TestModelsAreTheCatalogOnAbliteration(t *testing.T) {
	a, _ := testApp(t, nil)
	models, err := a.Models()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range models {
		names = append(names, m.Name+"|"+m.Plan)
	}
	want := []string{"Qwen3.5|", "GLM-5.2|Lightweight", "GLM-5.3|Lightweight"}
	if len(names) != len(want) {
		t.Fatalf("models %q", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("models %q, want %q", names, want)
		}
	}
}

// Switching a held chat to a model its plan runs sends what it held.
func TestSwitchingModelsSendsWhatWasHeld(t *testing.T) {
	srv := fakeModel(t, sse("Small hello.", "", "", ""))
	a, dir := testApp(t, srv)

	v, err := a.NewChat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModel(v.ID, "abliterated-model-large", 0, ""); err != nil {
		t.Fatal(err)
	}
	if started, _ := a.Send(v.ID, "hi"); started {
		t.Fatal("GLM-5.2 ran on Free")
	}
	if _, err := a.SetModel(v.ID, "abliterated-model", 0, ""); err != nil {
		t.Fatal(err)
	}
	v = waitIdle(t, a, v.ID, func(v ChatView) bool { return !v.Running && len(v.Items) == 2 })
	if got := kinds(v.Items); got != "user assistant" || v.Items[1].Text != "Small hello." {
		t.Fatalf("after switching: %q", got)
	}
}
