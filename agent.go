package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	rand "math/rand/v2"
	"sync"
	"time"
)

type Manager struct {
	store   *Store
	mu      sync.RWMutex
	runners map[string]*AgentRunner
}

func NewManager(store *Store) *Manager {
	return &Manager{
		store:   store,
		runners: make(map[string]*AgentRunner),
	}
}

func (m *Manager) StartEnabled() {
	for _, cfg := range m.store.All() {
		if cfg.Enabled {
			_ = m.Start(cfg.ID)
		}
	}
}

func (m *Manager) List() []AgentView {
	agents := m.store.All()
	views := make([]AgentView, 0, len(agents))
	for _, cfg := range agents {
		views = append(views, AgentView{
			AgentConfig: cfg,
			Status:      m.Status(cfg.ID),
		})
	}
	return views
}

func (m *Manager) Create(cfg AgentConfig) (AgentView, error) {
	shouldStart := cfg.Enabled
	cfg.Enabled = false
	cfg, err := m.store.Create(cfg)
	if err != nil {
		return AgentView{}, err
	}
	if shouldStart {
		if err := m.startRunner(cfg, true); err != nil {
			return AgentView{}, err
		}
	}
	latest, _ := m.store.Get(cfg.ID)
	return AgentView{AgentConfig: latest, Status: m.Status(cfg.ID)}, nil
}

func (m *Manager) Update(id string, cfg AgentConfig) (AgentView, error) {
	shouldStart := cfg.Enabled
	cfg.Enabled = false
	cfg, err := m.store.Update(id, cfg)
	if err != nil {
		return AgentView{}, err
	}
	m.stopRunner(id)
	if shouldStart {
		if err := m.startRunner(cfg, true); err != nil {
			return AgentView{}, err
		}
	}
	latest, _ := m.store.Get(cfg.ID)
	return AgentView{AgentConfig: latest, Status: m.Status(cfg.ID)}, nil
}

func (m *Manager) Delete(id string) error {
	m.stopRunner(id)
	return m.store.Delete(id)
}

func (m *Manager) Start(id string) error {
	cfg, ok := m.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	return m.startRunner(cfg, true)
}

func (m *Manager) Stop(id string) error {
	if _, ok := m.store.Get(id); !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	if _, err := m.store.SetEnabled(id, false); err != nil {
		return err
	}
	m.stopRunner(id)
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	runners := make([]*AgentRunner, 0, len(m.runners))
	for _, r := range m.runners {
		runners = append(runners, r)
	}
	m.runners = make(map[string]*AgentRunner)
	m.mu.Unlock()
	for _, r := range runners {
		r.Stop()
	}
}

func (m *Manager) Status(id string) RuntimeStatus {
	m.mu.RLock()
	runner := m.runners[id]
	m.mu.RUnlock()
	if runner == nil {
		return RuntimeStatus{Running: false, State: "stopped"}
	}
	return runner.Status()
}

func (m *Manager) startRunner(cfg AgentConfig, persistEnabled bool) error {
	cfg.Enabled = true
	cfg, err := m.ensureRunnable(cfg)
	if err != nil {
		return err
	}
	if persistEnabled {
		var err error
		cfg, err = m.store.Update(cfg.ID, cfg)
		if err != nil {
			return err
		}
	}
	runner := NewAgentRunner(cfg)

	m.mu.Lock()
	old := m.runners[cfg.ID]
	m.runners[cfg.ID] = runner
	m.mu.Unlock()

	if old != nil {
		old.Stop()
	}
	go func() {
		runner.Run()
		m.mu.Lock()
		if m.runners[cfg.ID] == runner {
			delete(m.runners, cfg.ID)
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) ensureRunnable(cfg AgentConfig) (AgentConfig, error) {
	cfg = normalizeAgent(cfg)
	if err := validateAgent(cfg); err != nil {
		return AgentConfig{}, err
	}
	if cfg.Token != "" {
		return cfg, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := RegisterByDiscoveryKey(ctx, cfg)
	if err != nil {
		return AgentConfig{}, err
	}
	cfg.RegisteredUUID = result.UUID
	cfg.Token = result.Token
	return cfg, nil
}

func (m *Manager) stopRunner(id string) {
	m.mu.Lock()
	runner := m.runners[id]
	delete(m.runners, id)
	m.mu.Unlock()
	if runner != nil {
		runner.Stop()
	}
}

type AgentRunner struct {
	cfg       AgentConfig
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	statusMu  sync.RWMutex
	status    RuntimeStatus
	totalUp   int64
	totalDown int64
	startedAt time.Time
	lastTick  time.Time
}

func NewAgentRunner(cfg AgentConfig) *AgentRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentRunner{
		cfg:       cfg,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		totalUp:   cfg.Behavior.InitialTotalUp,
		totalDown: cfg.Behavior.InitialTotalDown,
		startedAt: time.Now(),
		lastTick:  time.Now(),
		status: RuntimeStatus{
			Running: true,
			State:   "starting",
		},
	}
}

func (r *AgentRunner) Run() {
	defer close(r.done)

	reportEvery := secondsDuration(r.cfg.ReportIntervalSeconds, time.Second)
	basicEvery := secondsDuration(r.cfg.BasicInfoIntervalMinutes*60, 5*time.Minute)
	reconnectEvery := secondsDuration(r.cfg.ReconnectIntervalSeconds, 5*time.Second)

	for {
		if r.ctx.Err() != nil {
			r.setStopped()
			return
		}
		r.setState("connecting", false, "")
		conn, err := DialWebSocket(r.ctx, r.cfg.Endpoint, r.cfg.Token, r.cfg.IgnoreUnsafeCert)
		if err != nil {
			r.setState("reconnecting", false, err.Error())
			if !sleepContext(r.ctx, reconnectEvery) {
				r.setStopped()
				return
			}
			continue
		}

		r.setState("connected", true, "")
		err = r.connectedLoop(conn, reportEvery, basicEvery)
		_ = conn.Close()
		if r.ctx.Err() != nil {
			r.setStopped()
			return
		}
		if err != nil {
			r.setState("reconnecting", false, err.Error())
		}
		if !sleepContext(r.ctx, reconnectEvery) {
			r.setStopped()
			return
		}
	}
}

func (r *AgentRunner) Stop() {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
	}
}

func (r *AgentRunner) Status() RuntimeStatus {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

func (r *AgentRunner) connectedLoop(conn *WSConn, reportEvery, basicEvery time.Duration) error {
	readErr := make(chan error, 1)
	go r.readLoop(conn, readErr)

	if err := r.sendBasicInfo(conn); err != nil {
		return err
	}
	if err := r.sendReport(conn); err != nil {
		return err
	}

	reportTicker := time.NewTicker(reportEvery)
	defer reportTicker.Stop()
	basicTicker := time.NewTicker(basicEvery)
	defer basicTicker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return nil
		case err := <-readErr:
			if err == nil {
				err = errors.New("websocket reader stopped")
			}
			return err
		case <-basicTicker.C:
			if err := r.sendBasicInfo(conn); err != nil {
				return err
			}
		case <-reportTicker.C:
			if err := r.sendReport(conn); err != nil {
				return err
			}
		}
	}
}

func (r *AgentRunner) readLoop(conn *WSConn, readErr chan<- error) {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			select {
			case readErr <- err:
			default:
			}
			return
		}
		r.ignoreServerPayload(payload)
	}
}

func (r *AgentRunner) sendBasicInfo(conn *WSConn) error {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "agent.basicInfo",
		Params: map[string]any{
			"info": r.cfg.BasicInfo,
		},
		ID: nil,
	}
	if err := conn.WriteJSON(req); err != nil {
		return err
	}
	r.statusMu.Lock()
	r.status.LastBasicInfoAt = time.Now()
	r.statusMu.Unlock()
	return nil
}

func (r *AgentRunner) sendReport(conn *WSConn) error {
	report := r.buildReport()
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "agent.report",
		Params: map[string]any{
			"report": report,
		},
		ID: nil,
	}
	if err := conn.WriteJSON(req); err != nil {
		return err
	}
	r.statusMu.Lock()
	r.status.LastReportAt = time.Now()
	r.status.ReportsSent++
	r.statusMu.Unlock()
	return nil
}

func (r *AgentRunner) buildReport() reportPayload {
	now := time.Now()
	elapsed := now.Sub(r.lastTick).Seconds()
	if elapsed <= 0 || elapsed > 300 {
		elapsed = r.cfg.ReportIntervalSeconds
	}
	r.lastTick = now

	behavior := r.cfg.Behavior
	up := int64(math.Round(sampleFloat(behavior.NetworkUpBytesPerSecond)))
	down := int64(math.Round(sampleFloat(behavior.NetworkDownBytesPerSecond)))
	r.totalUp += int64(math.Round(float64(up) * elapsed))
	r.totalDown += int64(math.Round(float64(down) * elapsed))

	return reportPayload{
		CPU: cpuReport{
			Usage: round(sampleFloat(behavior.CPU), 2),
		},
		RAM: memoryReport{
			Total: r.cfg.BasicInfo.MemTotal,
			Used:  percentOf(r.cfg.BasicInfo.MemTotal, sampleFloat(behavior.RAMUsedPercent)),
		},
		Swap: memoryReport{
			Total: r.cfg.BasicInfo.SwapTotal,
			Used:  percentOf(r.cfg.BasicInfo.SwapTotal, sampleFloat(behavior.SwapUsedPercent)),
		},
		Load: loadReport{
			Load1:  round(sampleFloat(behavior.Load1), 2),
			Load5:  round(sampleFloat(behavior.Load5), 2),
			Load15: round(sampleFloat(behavior.Load15), 2),
		},
		Disk: diskReport{
			Total: r.cfg.BasicInfo.DiskTotal,
			Used:  percentOf(r.cfg.BasicInfo.DiskTotal, sampleFloat(behavior.DiskUsedPercent)),
		},
		Network: networkReport{
			Up:        up,
			Down:      down,
			TotalUp:   r.totalUp,
			TotalDown: r.totalDown,
		},
		Connections: connectionsReport{
			TCP: sampleInt(behavior.TCPConnections),
			UDP: sampleInt(behavior.UDPConnections),
		},
		Uptime:    behavior.InitialUptimeSeconds + int64(now.Sub(r.startedAt).Seconds()),
		Process:   sampleInt(behavior.Processes),
		Message:   behavior.Message,
		UpdatedAt: now,
	}
}

func (r *AgentRunner) ignoreServerPayload(payload []byte) {
	var obj map[string]any
	_ = json.Unmarshal(payload, &obj)
	r.statusMu.Lock()
	r.status.IgnoredEvents++
	r.statusMu.Unlock()
}

func (r *AgentRunner) setState(state string, connected bool, lastErr string) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	r.status.Running = true
	r.status.Connected = connected
	r.status.State = state
	r.status.LastError = lastErr
}

func (r *AgentRunner) setStopped() {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	r.status.Running = false
	r.status.Connected = false
	r.status.State = "stopped"
}

func secondsDuration(seconds float64, fallback time.Duration) time.Duration {
	if seconds <= 0 {
		return fallback
	}
	d := time.Duration(seconds * float64(time.Second))
	if d <= 0 {
		return fallback
	}
	return d
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func sampleFloat(m FloatMetric) float64 {
	v := m.Base
	if m.Jitter > 0 {
		v += (rand.Float64()*2 - 1) * m.Jitter
	}
	if v < m.Min {
		v = m.Min
	}
	if m.Max > 0 && v > m.Max {
		v = m.Max
	}
	return v
}

func sampleInt(m IntMetric) int {
	v := m.Base
	if m.Jitter > 0 {
		v += rand.IntN(m.Jitter*2+1) - m.Jitter
	}
	if v < m.Min {
		v = m.Min
	}
	if m.Max > 0 && v > m.Max {
		v = m.Max
	}
	return v
}

func percentOf(total int64, percent float64) int64 {
	if total <= 0 {
		return 0
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return int64(math.Round(float64(total) * percent / 100))
}

func round(v float64, places int) float64 {
	pow := math.Pow10(places)
	return math.Round(v*pow) / pow
}
