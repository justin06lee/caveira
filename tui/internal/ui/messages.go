package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/justin06lee/caveira/tui/internal/api"
)

type meMsg struct {
	me  *api.Me
	err error
}

type deviceStartedMsg struct {
	device *api.DeviceStart
	err    error
}

type devicePollMsg struct {
	poll *api.DevicePoll
	err  error
}

type plansMsg struct {
	plans *api.PlansResponse
	err   error
}

type checkoutMsg struct {
	checkout *api.Checkout
	err      error
}

type pollTickMsg struct{}

func fetchMe(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		me, err := c.Me(ctx)
		return meMsg{me: me, err: err}
	}
}

func startDevice(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := c.StartDeviceLogin(ctx)
		return deviceStartedMsg{device: d, err: err}
	}
}

func pollDevice(c *api.Client, deviceCode string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		p, err := c.PollDeviceLogin(ctx, deviceCode)
		return devicePollMsg{poll: p, err: err}
	}
}

func fetchPlans(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		p, err := c.Plans(ctx)
		return plansMsg{plans: p, err: err}
	}
}

func checkout(c *api.Client, planID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := c.Checkout(ctx, planID)
		return checkoutMsg{checkout: out, err: err}
	}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return pollTickMsg{} })
}
