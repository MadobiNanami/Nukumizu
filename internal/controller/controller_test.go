package controller

import (
	"os"
	"path/filepath"
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

// heldOffline returns the Offline notification the manager is currently holding
// for the given node, if any. It lets a test drive the hold/fire/release race
// deterministically instead of racing a real timer.
func (m *Manager) heldOffline(uuid string) *flapPending {
	m.flapMu.Lock()
	defer m.flapMu.Unlock()
	return m.flapPending[uuid]
}

// recordingController keeps every status change it is handed, so a test can
// assert on what actually reached the controllers. The embedded fakeController
// satisfies the rest of the Controller interface.
type recordingController struct {
	*fakeController
	changes chan node.StatusChange
}

func newRecording(name string) *recordingController {
	return &recordingController{
		fakeController: newFake(name),
		changes:        make(chan node.StatusChange, 16),
	}
}

func (r *recordingController) SendStatusChange(change node.StatusChange) error {
	r.changes <- change
	return nil
}

// awaitChange blocks until a status change reaches the controller, failing the
// test rather than hanging if none does.
func awaitChange(t *testing.T, rec *recordingController, what string) node.StatusChange {
	t.Helper()
	select {
	case change := <-rec.changes:
		return change
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for the %s notification", what)
		return node.StatusChange{}
	}
}

// expectNoChange asserts that no status change reaches the controller for the
// given window. Only a negative assertion needs a real wait; the manager has no
// observable "decided to stay silent" event.
func expectNoChange(t *testing.T, rec *recordingController, window time.Duration, what string) {
	t.Helper()
	select {
	case change := <-rec.changes:
		t.Fatalf("unexpected %s notification: %+v", what, change)
	case <-time.After(window):
	}
}

func offlineChange(uuid string) node.StatusChange {
	return node.StatusChange{Event: "Offline", UUID: uuid, Name: uuid, OldOnline: true, NewOnline: false}
}

func onlineChange(uuid string) node.StatusChange {
	return node.StatusChange{Event: "Online", UUID: uuid, Name: uuid, OldOnline: false, NewOnline: true}
}

// flapTestManager returns a manager with a recording controller installed and
// the flap window stubbed to tolerance, so the tests never touch the global
// bot_node_config.json or wait on real seconds. ReplaceAll makes the controller
// routable before it starts it, so no start handshake is needed here.
func flapTestManager(tolerance time.Duration) (*Manager, *recordingController) {
	m := newTestManager()
	m.flapToleranceFor = func(string) time.Duration { return tolerance }
	rec := newRecording("recorder")
	m.ReplaceAll([]Controller{rec}, config.ControllerMethodConfig{})
	return m, rec
}

// TestFlapInhibitionSuppressesBothHalves is the core guarantee: a node that
// returns inside its flap window produces no notification at all, so a flapping
// node does not flood the controllers.
func TestFlapInhibitionSuppressesBothHalves(t *testing.T) {
	m, rec := flapTestManager(50 * time.Millisecond)

	m.NotifyStatusChange(offlineChange("flappy"))
	m.NotifyStatusChange(onlineChange("flappy"))

	expectNoChange(t, rec, 300*time.Millisecond, "suppressed offline/online pair")

	if held := m.heldOffline("flappy"); held != nil {
		t.Error("the held offline notification was not released when the node returned")
	}
}

// TestFlapInhibitionReportsLateOffline: a node still down when the window
// elapses reports Offline as usual, however late the notification is.
func TestFlapInhibitionReportsLateOffline(t *testing.T) {
	m, rec := flapTestManager(50 * time.Millisecond)

	m.NotifyStatusChange(offlineChange("down"))

	if change := awaitChange(t, rec, "offline"); change.Event != "Offline" || change.UUID != "down" {
		t.Errorf("got %+v, want the Offline change for node down", change)
	}
	if held := m.heldOffline("down"); held != nil {
		t.Error("the held notification was still pending after it was dispatched")
	}
}

// TestFlapInhibitionReportsRecoveryAfterWindow: once the Offline has gone out,
// the node's eventual return is a genuine recovery and must be reported. This
// is the honest half of the "both or neither, never Offline alone" rule.
func TestFlapInhibitionReportsRecoveryAfterWindow(t *testing.T) {
	m, rec := flapTestManager(50 * time.Millisecond)

	m.NotifyStatusChange(offlineChange("down"))
	if change := awaitChange(t, rec, "offline"); change.Event != "Offline" {
		t.Fatalf("got %+v, want the Offline change", change)
	}

	m.NotifyStatusChange(onlineChange("down"))
	if change := awaitChange(t, rec, "online"); change.Event != "Online" {
		t.Errorf("got %+v, want the Online change", change)
	}
}

// TestFlapInhibitionDisabledWithoutTolerance guards the previous behavior: with
// no window (the default for every node), an Offline still goes out at once.
func TestFlapInhibitionDisabledWithoutTolerance(t *testing.T) {
	m, rec := flapTestManager(0)

	m.NotifyStatusChange(offlineChange("plain"))

	if change := awaitChange(t, rec, "offline"); change.Event != "Offline" {
		t.Errorf("got %+v, want the Offline change", change)
	}
	if held := m.heldOffline("plain"); held != nil {
		t.Error("a node without a flap window must not have a held notification")
	}
}

// TestFlapInhibitionHonorsEnableStatusNotify pins the gate ordering: a node
// whose notifications are switched off is skipped before any hold is taken, so
// it cannot report a late Offline either.
func TestFlapInhibitionHonorsEnableStatusNotify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bot_node_config.json")
	content := `{"muted": {"enableStatusNotify": false, "flap_tolerant": 600}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write node config: %v", err)
	}
	if err := config.LoadBotNodeConfig(path); err != nil {
		t.Fatalf("LoadBotNodeConfig: %v", err)
	}
	// Leave nothing behind for the other tests in this package, which reach the
	// same package-global node config.
	t.Cleanup(func() {
		if err := config.LoadBotNodeConfig(filepath.Join(dir, "empty.json")); err != nil {
			t.Errorf("reset node config: %v", err)
		}
	})

	// The stub reports a window, so only the allow-list can be responsible for
	// the silence.
	m, rec := flapTestManager(time.Hour)
	m.NotifyStatusChange(offlineChange("muted"))

	expectNoChange(t, rec, 100*time.Millisecond, "offline for a muted node")
	if held := m.heldOffline("muted"); held != nil {
		t.Error("a muted node must not have its Offline held")
	}
}

// TestFlapInhibitionClaimOrder pins both sides of the hold/fire/release race
// deterministically, with a window long enough that the real timer never fires:
// whichever claims the held notification first wins, and the loser sees it gone.
func TestFlapInhibitionClaimOrder(t *testing.T) {
	t.Run("release first suppresses the pair", func(t *testing.T) {
		m, rec := flapTestManager(time.Hour)
		m.NotifyStatusChange(offlineChange("node"))

		held := m.heldOffline("node")
		if held == nil {
			t.Fatal("the offline notification was not held")
		}
		if !m.releaseHeldOffline("node") {
			t.Fatal("releaseHeldOffline reported no pending notification")
		}
		// The timer goroutine loses the claim and must not dispatch.
		m.fireHeldOffline("node", held)

		expectNoChange(t, rec, 100*time.Millisecond, "notification after a release")
	})

	t.Run("fire first reports the offline alone", func(t *testing.T) {
		m, rec := flapTestManager(time.Hour)
		m.NotifyStatusChange(offlineChange("node"))

		held := m.heldOffline("node")
		if held == nil {
			t.Fatal("the offline notification was not held")
		}
		m.fireHeldOffline("node", held)

		if change := awaitChange(t, rec, "offline"); change.Event != "Offline" {
			t.Errorf("got %+v, want the Offline change", change)
		}
		if m.releaseHeldOffline("node") {
			t.Error("a fired notification must not be claimable again")
		}
	})
}

// TestStopAllDropsHeldNotifications: a hold must not outlive the controllers it
// was meant for.
func TestStopAllDropsHeldNotifications(t *testing.T) {
	m, _ := flapTestManager(time.Hour)
	m.NotifyStatusChange(offlineChange("node"))

	m.StopAll()

	if held := m.heldOffline("node"); held != nil {
		t.Error("StopAll left a held offline notification armed")
	}
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
// old controllers must be shut down, or a rebuilt onebot or Telegram controller
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
