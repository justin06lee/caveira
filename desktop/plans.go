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
	// Usage ranks how much the plan allows, 1 to 5, for its meter.
	Usage int `json:"usage"`
	// Large says it runs the large models, not only abliterated-model.
	Large bool     `json:"large"`
	Lines []string `json:"lines"`
}

var plans = []Plan{
	{ID: "free", Name: "Free", Price: 0, Usage: 1, Lines: []string{"Small models", "A small usage limit"}},
	{ID: "lightweight", Name: "Lightweight", Price: 20, Usage: 2, Large: true, Lines: []string{"Larger models", "More usage"}},
	{ID: "middleweight", Name: "Middleweight", Price: 50, Usage: 3, Large: true, Lines: []string{"Larger models", "Even more usage"}},
	{ID: "heavyweight", Name: "Heavyweight", Price: 100, Usage: 4, Large: true, Lines: []string{"Larger models", "Heavy usage"}},
	{ID: "champion", Name: "Champion", Price: 200, Usage: 5, Large: true, Lines: []string{"Larger models", "The most usage"}},
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
	id, name, group string
	logos           []string
	large           bool
}

// catalog is the models offered on abliteration.ai, in the order the
// picker shows them. The large ones are GLM under the hood.
var catalog = []catalogModel{
	{id: "abliterated-model", name: "abliterated-model", group: "abliteration.ai", logos: []string{"abliteration"}},
	{id: "abliterated-model-large", name: "GLM-5.2", group: "abliteration.ai × Z.ai", logos: []string{"abliteration", "zai"}, large: true},
	{id: "abliterated-model-large-v2", name: "GLM-5.3", group: "abliteration.ai × Z.ai", logos: []string{"abliteration", "zai"}, large: true},
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
// of the model it cannot. A local model runs on any plan.
func allows(plan, model string, onLocal bool) (ok bool, locked string) {
	p, subscribed := planByID(plan)
	if !subscribed {
		return false, ""
	}
	if onLocal {
		return true, ""
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
	if ok, _ := allows(a.prefs.Plan, model, c.local); !ok {
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
			ID: m.id, Name: m.name, Group: m.group, Logos: m.logos,
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
