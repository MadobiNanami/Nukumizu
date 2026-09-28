package controller

import (
	"sync"
	"testing"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
)

// fakeController records the lifecycle calls it receives so a test can assert
// what ReplaceAll did to it. Start signals on started, because ReplaceAll starts
// controllers off the calling goroutine.
type fakeController struct {
	name    string
	started chan struct{}

	mu     sync.Mutex
	starts int
	stops  int
}

func newFake(name string) *fakeController {
	return &fakeController{name: name, started: make(chan struct{}, 4)}
}

func (f *fakeController) Name() string     { return f.name }
func (f *fakeController) IsEnabled() bool  { return true }
func (f *fakeController) IsMarkdown() bool { return false }

func (f *fakeController) Start() error {
	f.mu.Lock()
	f.starts++
	f.mu.Unlock()
	select {
	case f.started <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeController) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
}

func (f *fakeController) lifecycle() (starts, stops int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.stops
}

func (f *fakeController) SendStatusChange(node.StatusChange) error               { return nil }
func (f *fakeController) SendServerList(string, string) error                    { return nil }
func (f *fakeController) SendExecuteResult(string, string, string, string) error { return nil }
func (f *fakeController) SendAlert(Alert) error                                  { return nil }

// waitStarted blocks until the controller's Start has run.
func (f *fakeController) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("controller %s was never started", f.name)
	}
}

func newTestManager() *Manager {
	return &Manager{controllers: make(map[string]Controller)}
}

// routedTo returns the controller the manager currently routes the given name
// to.
func (m *Manager) routedTo(name string) Controller {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.controllers[name]
}

func TestReplaceAllInstallsAndStarts(t *testing.T) {
	m := newTestManager()
	alpha, beta := newFake("alpha"), newFake("beta")

	m.ReplaceAll([]Controller{alpha, beta}, config.ControllerMethodConfig{})

	alpha.waitStarted(t)
	beta.waitStarted(t)

	if got := m.routedTo("alpha"); got != Controller(alpha) {
		t.Errorf("alpha is not routable after ReplaceAll: %v", got)
	}
	if got := m.routedTo("beta"); got != Controller(beta) {
		t.Errorf("beta is not routable after ReplaceAll: %v", got)
	}
}

// TestReplaceAllStopsTheOutgoingSet is the invariant the rebuild relies on: the
// old controllers must be shut down, or a rebuilt NapCat or Telegram controller
// would leave its previous connection running.
func TestReplaceAllStopsTheOutgoingSet(t *testing.T) {
	m := newTestManager()
	outgoing := newFake("alpha")
	m.ReplaceAll([]Controller{outgoing}, config.ControllerMethodConfig{})
	outgoing.waitStarted(t)

	incoming := newFake("alpha")
	m.ReplaceAll([]Controller{incoming}, config.ControllerMethodConfig{})
	incoming.waitStarted(t)

	if starts, stops := outgoing.lifecycle(); starts != 1 || stops != 1 {
		t.Errorf("outgoing controller lifecycle = %d starts / %d stops, want 1/1", starts, stops)
	}
	if _, stops := incoming.lifecycle(); stops != 0 {
		t.Errorf("incoming controller was stopped %d times", stops)
	}
	if got := m.routedTo("alpha"); got != Controller(incoming) {
		t.Error("routing still points at the outgoing controller")
	}
}

func TestReplaceAllDropsChannelsLeftOut(t *testing.T) {
	m := newTestManager()
	m.ReplaceAll([]Controller{newFake("alpha"), newFake("beta")}, config.ControllerMethodConfig{})

	m.ReplaceAll([]Controller{newFake("beta")}, config.ControllerMethodConfig{})

	if got := m.routedTo("alpha"); got != nil {
		t.Errorf("a channel missing from the new set is still routable: %v", got)
	}
	if got := m.routedTo("beta"); got == nil {
		t.Error("the surviving channel is not routable")
	}
}

func TestNeedsRebuild(t *testing.T) {
	m := newTestManager()
	base := config.ControllerMethodConfig{
		Email: config.EmailConfig{Enabled: true, SMTPPort: 587, To: []string{"a@example.com"}},
	}

	if !m.NeedsRebuild(base) {
		t.Error("a manager with nothing installed must report that a rebuild is needed")
	}

	m.ReplaceAll(nil, base)
	if m.NeedsRebuild(base) {
		t.Error("the settings the set was built from must not ask for another rebuild")
	}

	changed := base
	changed.Email.Enabled = false
	if !m.NeedsRebuild(changed) {
		t.Error("a changed email setting must ask for a rebuild")
	}

	// The section carries maps and slices, so it is compared by value rather
	// than by identity: equal content must not trigger a rebuild.
	withHeaders := config.ControllerMethodConfig{
		Webhook: config.WebhookConfig{Headers: map[string]string{"X-Token": "t"}},
	}
	m.ReplaceAll(nil, withHeaders)
	equalHeaders := config.ControllerMethodConfig{
		Webhook: config.WebhookConfig{Headers: map[string]string{"X-Token": "t"}},
	}
	if m.NeedsRebuild(equalHeaders) {
		t.Error("equal header maps must not ask for a rebuild")
	}

	differentHeaders := config.ControllerMethodConfig{
		Webhook: config.WebhookConfig{Headers: map[string]string{"X-Token": "other"}},
	}
	if !m.NeedsRebuild(differentHeaders) {
		t.Error("a changed header must ask for a rebuild")
	}
}

// TestReplaceAllDuringNotification drives ReplaceAll while notifications are
// being routed. Run with -race: the swap replaces the map the routing path
// reads, which is what the registry lock exists to make safe.
func TestReplaceAllDuringNotification(t *testing.T) {
	m := newTestManager()
	m.ReplaceAll([]Controller{newFake("alpha")}, config.ControllerMethodConfig{})

	var readers, writers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 3; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				m.NotifyStatusChange(node.StatusChange{UUID: "u1", Name: "alpha", Event: "Online"})
				_ = m.IsMarkdown("alpha")
			}
		}()
	}

	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 20; i++ {
			m.ReplaceAll([]Controller{newFake("alpha"), newFake("beta")}, config.ControllerMethodConfig{})
		}
	}()

	// Let the writer finish, then release the readers: they only return once
	// stop is closed.
	writers.Wait()
	close(stop)
	readers.Wait()

	if got := m.routedTo("beta"); got == nil {
		t.Error("the last installed set is not routable")
	}
}
