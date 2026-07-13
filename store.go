package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.RWMutex
	db   Database
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.db = Database{Version: 1, Agents: []AgentConfig{}}
		return s.saveLocked()
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		s.db = Database{Version: 1, Agents: []AgentConfig{}}
		return s.saveLocked()
	}
	if err := json.Unmarshal(b, &s.db); err != nil {
		return fmt.Errorf("read %s: %w", s.path, err)
	}
	if s.db.Version == 0 {
		s.db.Version = 1
	}
	if s.db.Agents == nil {
		s.db.Agents = []AgentConfig{}
	}
	for i := range s.db.Agents {
		s.db.Agents[i] = normalizeAgent(s.db.Agents[i])
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) All() []AgentConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentConfig, len(s.db.Agents))
	copy(out, s.db.Agents)
	return out
}

func (s *Store) Get(id string) (AgentConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, agent := range s.db.Agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return AgentConfig{}, false
}

func (s *Store) Create(cfg AgentConfig) (AgentConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg.ID = strings.TrimSpace(cfg.ID)
	if cfg.ID == "" {
		cfg.ID = newID()
	}
	if s.existsLocked(cfg.ID) {
		return AgentConfig{}, fmt.Errorf("agent id %q already exists", cfg.ID)
	}
	cfg = normalizeAgent(cfg)
	if err := validateAgent(cfg); err != nil {
		return AgentConfig{}, err
	}
	s.db.Agents = append(s.db.Agents, cfg)
	if err := s.saveLocked(); err != nil {
		return AgentConfig{}, err
	}
	return cfg, nil
}

func (s *Store) Update(id string, cfg AgentConfig) (AgentConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.db.Agents {
		if s.db.Agents[i].ID != id {
			continue
		}
		cfg.ID = id
		cfg = normalizeAgent(cfg)
		if err := validateAgent(cfg); err != nil {
			return AgentConfig{}, err
		}
		s.db.Agents[i] = cfg
		if err := s.saveLocked(); err != nil {
			return AgentConfig{}, err
		}
		return cfg, nil
	}
	return AgentConfig{}, fmt.Errorf("agent %q not found", id)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.db.Agents {
		if s.db.Agents[i].ID != id {
			continue
		}
		s.db.Agents = append(s.db.Agents[:i], s.db.Agents[i+1:]...)
		return s.saveLocked()
	}
	return fmt.Errorf("agent %q not found", id)
}

func (s *Store) SetEnabled(id string, enabled bool) (AgentConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.db.Agents {
		if s.db.Agents[i].ID != id {
			continue
		}
		s.db.Agents[i].Enabled = enabled
		s.db.Agents[i] = normalizeAgent(s.db.Agents[i])
		if err := validateAgent(s.db.Agents[i]); err != nil {
			return AgentConfig{}, err
		}
		if err := s.saveLocked(); err != nil {
			return AgentConfig{}, err
		}
		return s.db.Agents[i], nil
	}
	return AgentConfig{}, fmt.Errorf("agent %q not found", id)
}

func (s *Store) existsLocked(id string) bool {
	for _, agent := range s.db.Agents {
		if agent.ID == id {
			return true
		}
	}
	return false
}

func normalizeAgent(cfg AgentConfig) AgentConfig {
	def := defaultAgentConfig()
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.DiscoveryKey = strings.TrimSpace(cfg.DiscoveryKey)
	cfg.RegisteredUUID = strings.TrimSpace(cfg.RegisteredUUID)
	cfg.Token = strings.TrimSpace(cfg.Token)

	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.ReportIntervalSeconds <= 0 {
		cfg.ReportIntervalSeconds = def.ReportIntervalSeconds
	}
	if cfg.BasicInfoIntervalMinutes <= 0 {
		cfg.BasicInfoIntervalMinutes = def.BasicInfoIntervalMinutes
	}
	if cfg.ReconnectIntervalSeconds <= 0 {
		cfg.ReconnectIntervalSeconds = def.ReconnectIntervalSeconds
	}
	if cfg.BasicInfo == (BasicInfo{}) {
		cfg.BasicInfo = def.BasicInfo
	}
	if cfg.Behavior == (AgentBehavior{}) {
		cfg.Behavior = def.Behavior
	}
	if cfg.BasicInfo.Version == "" {
		cfg.BasicInfo.Version = "fake-komari-agent/" + appVersion
	}
	if cfg.BasicInfo.Arch == "" {
		cfg.BasicInfo.Arch = def.BasicInfo.Arch
	}
	if cfg.BasicInfo.OS == "" {
		cfg.BasicInfo.OS = def.BasicInfo.OS
	}
	cfg.BasicInfo.DiskTotal = nonNegative64(cfg.BasicInfo.DiskTotal)
	cfg.BasicInfo.MemTotal = nonNegative64(cfg.BasicInfo.MemTotal)
	cfg.BasicInfo.SwapTotal = nonNegative64(cfg.BasicInfo.SwapTotal)
	cfg.BasicInfo.CPUCores = nonNegativeInt(cfg.BasicInfo.CPUCores)
	cfg.BasicInfo.CPUPhysicalCores = nonNegativeInt(cfg.BasicInfo.CPUPhysicalCores)
	cfg.Behavior = normalizeBehavior(cfg.Behavior)
	return cfg
}

func normalizeBehavior(b AgentBehavior) AgentBehavior {
	b.CPU = normalizeFloatMetric(b.CPU, 0, 100)
	b.RAMUsedPercent = normalizeFloatMetric(b.RAMUsedPercent, 0, 100)
	b.SwapUsedPercent = normalizeFloatMetric(b.SwapUsedPercent, 0, 100)
	b.DiskUsedPercent = normalizeFloatMetric(b.DiskUsedPercent, 0, 100)
	b.NetworkUpBytesPerSecond = normalizeFloatMetric(b.NetworkUpBytesPerSecond, 0, 0)
	b.NetworkDownBytesPerSecond = normalizeFloatMetric(b.NetworkDownBytesPerSecond, 0, 0)
	b.Load1 = normalizeFloatMetric(b.Load1, 0, 1000)
	b.Load5 = normalizeFloatMetric(b.Load5, 0, 1000)
	b.Load15 = normalizeFloatMetric(b.Load15, 0, 1000)
	b.TCPConnections = normalizeIntMetric(b.TCPConnections, 0, 0)
	b.UDPConnections = normalizeIntMetric(b.UDPConnections, 0, 0)
	b.Processes = normalizeIntMetric(b.Processes, 1, 0)
	b.InitialTotalUp = nonNegative64(b.InitialTotalUp)
	b.InitialTotalDown = nonNegative64(b.InitialTotalDown)
	b.InitialUptimeSeconds = nonNegative64(b.InitialUptimeSeconds)
	return b
}

func normalizeFloatMetric(m FloatMetric, minDefault, maxDefault float64) FloatMetric {
	if m.Jitter < 0 {
		m.Jitter = -m.Jitter
	}
	if m.Min == 0 && minDefault != 0 {
		m.Min = minDefault
	}
	if m.Max == 0 && maxDefault != 0 {
		m.Max = maxDefault
	}
	if m.Max > 0 && m.Min > m.Max {
		m.Min, m.Max = m.Max, m.Min
	}
	if m.Base < m.Min {
		m.Base = m.Min
	}
	if m.Max > 0 && m.Base > m.Max {
		m.Base = m.Max
	}
	return m
}

func normalizeIntMetric(m IntMetric, minDefault, maxDefault int) IntMetric {
	if m.Jitter < 0 {
		m.Jitter = -m.Jitter
	}
	if m.Min == 0 && minDefault != 0 {
		m.Min = minDefault
	}
	if m.Max == 0 && maxDefault != 0 {
		m.Max = maxDefault
	}
	if m.Max > 0 && m.Min > m.Max {
		m.Min, m.Max = m.Max, m.Min
	}
	if m.Base < m.Min {
		m.Base = m.Min
	}
	if m.Max > 0 && m.Base > m.Max {
		m.Base = m.Max
	}
	return m
}

func validateAgent(cfg AgentConfig) error {
	if cfg.Name == "" {
		return errors.New("name is required")
	}
	if cfg.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if cfg.Token == "" && cfg.DiscoveryKey == "" {
		return errors.New("auto discovery key is required")
	}
	if cfg.ReportIntervalSeconds < 0.2 {
		return errors.New("report interval must be at least 0.2 seconds")
	}
	if cfg.BasicInfoIntervalMinutes < 0.1 {
		return errors.New("basic info interval must be at least 0.1 minutes")
	}
	if cfg.ReconnectIntervalSeconds < 1 {
		return errors.New("reconnect interval must be at least 1 second")
	}
	if cfg.BasicInfo.MemTotal < 0 || cfg.BasicInfo.SwapTotal < 0 || cfg.BasicInfo.DiskTotal < 0 {
		return errors.New("capacity values must be non-negative")
	}
	return nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "agent-" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("agent-%d", time.Now().UnixNano())
}

func nonNegative64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func nonNegativeInt(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
