package main

import (
	"fmt"
	"strings"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/config"
)

// Plan is a caveira subscription. A chat runs only on one: without a
// plan, a message is held in the chat behind the plans until one is
// picked.
//
// There is no caveira cloud to pay yet, so the plan picked is kept in
// desktop.json (prefs.Plan) and nothing is charged. Everything that asks
// whether a chat may run goes through allows, so pointing that at the
// cloud's answer later changes nothing else.
type Plan struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"` // US dollars a month
	// Features is what it comes with.
	Features []string `json:"features"`
	// Popular marks the plan the plans page points to.
	Popular bool `json:"popular,omitempty"`
	// Large says it runs the large models, not only abliterated-model.
	Large bool `json:"large"`
}

// Usage counts from Free, and goes up with the price the way the labs'
// plans do (Claude's $100 and $200 Max are 5x and 20x its $20 Pro):
// every $10 is one Free's worth, so $200 is 20x.
var plans = []Plan{
	{ID: "free", Name: "Free", Price: 0,
		Features: []string{"Abliterated Qwen3.5, 256K token context", "A small usage limit"}},
	{ID: "lightweight", Name: "Lightweight", Price: 20, Popular: true, Large: true,
		Features: []string{"Abliterated GLM-5.2, 1M token context", "Abliterated GLM-5.3, 1M token context", "2x usage"}},
	{ID: "middleweight", Name: "Middleweight", Price: 50, Large: true,
		Features: []string{"Everything in Lightweight", "5x usage"}},
	{ID: "heavyweight", Name: "Heavyweight", Price: 100, Large: true,
		Features: []string{"Everything in Middleweight", "10x usage"}},
	{ID: "champion", Name: "Champion", Price: 200, Large: true,
		Features: []string{"Everything in Heavyweight", "20x usage"}},
}

func planByID(id string) (Plan, bool) {
	for _, p := range plans {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}

// catalogModel is a model caveira offers on abliteration.ai.
type catalogModel struct {
	id, name string
	logos    []string
	large    bool
}

// catalog is the models the desktop app runs, all on abliteration.ai, in
// the order the picker shows them, by the open model each is abliterated
// from: Qwen3.5, and GLM for the large ones.
var catalog = []catalogModel{
	{id: "abliterated-model", name: "Qwen3.5", logos: []string{"abliteration", "qwen"}},
	{id: "abliterated-model-large", name: "GLM-5.2", logos: []string{"abliteration", "zai"}, large: true},
	{id: "abliterated-model-large-v2", name: "GLM-5.3", logos: []string{"abliteration", "zai"}, large: true},
}

func catalogEntry(id string) (catalogModel, bool) {
	for _, m := range catalog {
		if m.id == id {
			return m, true
		}
	}
	return catalogModel{}, false
}

// largePlan is the cheapest plan with the large models, for "comes with".
func largePlan() Plan {
	for _, p := range plans {
		if p.Large {
			return p
		}
	}
	return plans[len(plans)-1]
}

// allows says whether plan can run model, and if not, why not as the name
// of the model it cannot.
func allows(plan, model string) (ok bool, locked string) {
	p, subscribed := planByID(plan)
	if !subscribed {
		return false, ""
	}
	if m, known := catalogEntry(model); known && m.large && !p.Large {
		return false, m.name
	}
	return true, ""
}

// Subscribe puts this install on a plan, then sends what chatID was
// holding for one, if the plan runs it.
func (a *App) Subscribe(plan, chatID string) (string, error) {
	if _, ok := planByID(plan); !ok {
		return "", fmt.Errorf("there is no %q plan", plan)
	}
	a.mu.Lock()
	a.prefs.Plan = plan
	err := a.prefs.save()
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	if chatID != "" {
		a.release(chatID)
	}
	return plan, nil
}

// hold keeps a message the chat's plan cannot run, under the plans. A
// second one goes with the first. Caller holds App.mu.
func (c *chat) hold(text, locked string) []ChatEvent {
	var out []ChatEvent
	if id := c.paywall(); id != "" {
		c.remove(id)
		out = append(out, ChatEvent{Chat: c.id, Type: "remove", ID: id})
	}
	c.held = append(c.held, text)
	out = append(out, c.push(Item{Kind: "user", Text: text}), c.push(Item{Kind: "paywall", Text: locked}))
	return out
}

// paywall is the plans item in the transcript, "" if there is none.
func (c *chat) paywall() string {
	for _, it := range c.items {
		if it.Kind == "paywall" {
			return it.ID
		}
	}
	return ""
}

// release sends what a chat was holding once its plan runs it: the
// messages already on screen go to the model as they are.
func (a *App) release(id string) {
	a.mu.Lock()
	c, ok := a.chats[id]
	if !ok || len(c.held) == 0 || c.cancel != nil || c.problem != "" {
		a.mu.Unlock()
		return
	}
	model, _ := c.ag.ModelInfo()
	if ok, _ := allows(a.prefs.Plan, model); !ok {
		a.mu.Unlock()
		return
	}
	var out []ChatEvent
	if pid := c.paywall(); pid != "" {
		c.remove(pid)
		out = append(out, ChatEvent{Chat: c.id, Type: "remove", ID: pid})
	}
	texts := c.held
	c.held = nil
	ag := c.ag
	ctx := c.begin()
	a.mu.Unlock()

	for _, e := range out {
		a.emit("chat", e)
	}
	go func() {
		notes(ag, texts[:len(texts)-1])
		a.turn(ctx, c, ag, texts[len(texts)-1])
	}()
}

// notes puts messages sent while the chat was held ahead of the last one,
// the way the terminal client sends what was queued.
func notes(ag *agent.Agent, texts []string) {
	if len(texts) == 0 {
		return
	}
	if ag.Session == nil {
		ag.NewSession()
	}
	for _, t := range texts {
		ag.Note(t)
	}
}

// catalogOptions is the picker's list for abliteration.ai.
func catalogOptions() []ModelOption {
	out := make([]ModelOption, 0, len(catalog))
	for _, m := range catalog {
		o := ModelOption{
			ID: m.id, Name: m.name, Group: "abliteration.ai", Logos: m.logos,
			Context: config.Spec(m.id).ContextWindow,
		}
		if m.large {
			o.Plan = largePlan().Name
		}
		out = append(out, o)
	}
	return out
}

// isCatalog says the endpoint is abliteration.ai, whose models are the
// catalog's.
func isCatalog(baseURL string) bool {
	return strings.TrimRight(baseURL, "/") == config.DefaultBaseURL
}
