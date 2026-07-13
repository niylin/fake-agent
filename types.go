package main

import "time"

const appVersion = "0.1.0"

type Database struct {
	Version           int           `json:"version"`
	PanelPasswordHash string        `json:"panel_password_hash"`
	Agents            []AgentConfig `json:"agents"`
}

type AgentConfig struct {
	ID                       string        `json:"id"`
	Name                     string        `json:"name"`
	Enabled                  bool          `json:"enabled"`
	Endpoint                 string        `json:"endpoint"`
	DiscoveryKey             string        `json:"discovery_key"`
	RegisteredUUID           string        `json:"registered_uuid"`
	Token                    string        `json:"token"`
	IgnoreUnsafeCert         bool          `json:"ignore_unsafe_cert"`
	ReportIntervalSeconds    float64       `json:"report_interval_seconds"`
	BasicInfoIntervalMinutes float64       `json:"basic_info_interval_minutes"`
	ReconnectIntervalSeconds float64       `json:"reconnect_interval_seconds"`
	BasicInfo                BasicInfo     `json:"basic_info"`
	Behavior                 AgentBehavior `json:"behavior"`
}

type BasicInfo struct {
	Arch             string `json:"arch"`
	CPUCores         int    `json:"cpu_cores"`
	CPUPhysicalCores int    `json:"cpu_physical_cores"`
	CPUName          string `json:"cpu_name"`
	DiskTotal        int64  `json:"disk_total"`
	GPUName          string `json:"gpu_name"`
	IPv4             string `json:"ipv4"`
	IPv6             string `json:"ipv6"`
	MemTotal         int64  `json:"mem_total"`
	OS               string `json:"os"`
	KernelVersion    string `json:"kernel_version"`
	SwapTotal        int64  `json:"swap_total"`
	Version          string `json:"version"`
	Virtualization   string `json:"virtualization"`
}

type AgentBehavior struct {
	CPU                       FloatMetric `json:"cpu"`
	RAMUsedPercent            FloatMetric `json:"ram_used_percent"`
	SwapUsedPercent           FloatMetric `json:"swap_used_percent"`
	DiskUsedPercent           FloatMetric `json:"disk_used_percent"`
	NetworkUpBytesPerSecond   FloatMetric `json:"network_up_bytes_per_second"`
	NetworkDownBytesPerSecond FloatMetric `json:"network_down_bytes_per_second"`
	Load1                     FloatMetric `json:"load1"`
	Load5                     FloatMetric `json:"load5"`
	Load15                    FloatMetric `json:"load15"`
	TCPConnections            IntMetric   `json:"tcp_connections"`
	UDPConnections            IntMetric   `json:"udp_connections"`
	Processes                 IntMetric   `json:"processes"`
	InitialTotalUp            int64       `json:"initial_total_up"`
	InitialTotalDown          int64       `json:"initial_total_down"`
	InitialUptimeSeconds      int64       `json:"initial_uptime_seconds"`
	Message                   string      `json:"message"`
}

type FloatMetric struct {
	Base   float64 `json:"base"`
	Jitter float64 `json:"jitter"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type IntMetric struct {
	Base   int `json:"base"`
	Jitter int `json:"jitter"`
	Min    int `json:"min"`
	Max    int `json:"max"`
}

type AgentView struct {
	AgentConfig
	Status RuntimeStatus `json:"status"`
}

type RuntimeStatus struct {
	Running         bool      `json:"running"`
	Connected       bool      `json:"connected"`
	State           string    `json:"state"`
	LastError       string    `json:"last_error,omitempty"`
	LastReportAt    time.Time `json:"last_report_at,omitempty"`
	LastBasicInfoAt time.Time `json:"last_basic_info_at,omitempty"`
	ReportsSent     uint64    `json:"reports_sent"`
	IgnoredEvents   uint64    `json:"ignored_events"`
}

type Template struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	BasicInfo BasicInfo     `json:"basic_info"`
	Behavior  AgentBehavior `json:"behavior"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      any    `json:"id"`
}

type reportPayload struct {
	CPU         cpuReport         `json:"cpu"`
	RAM         memoryReport      `json:"ram"`
	Swap        memoryReport      `json:"swap"`
	Load        loadReport        `json:"load"`
	Disk        diskReport        `json:"disk"`
	Network     networkReport     `json:"network"`
	Connections connectionsReport `json:"connections"`
	Uptime      int64             `json:"uptime"`
	Process     int               `json:"process"`
	Message     string            `json:"message"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type cpuReport struct {
	Usage float64 `json:"usage"`
}

type memoryReport struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
}

type loadReport struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type diskReport struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
}

type networkReport struct {
	Up        int64 `json:"up"`
	Down      int64 `json:"down"`
	TotalUp   int64 `json:"totalUp"`
	TotalDown int64 `json:"totalDown"`
}

type connectionsReport struct {
	TCP int `json:"tcp"`
	UDP int `json:"udp"`
}
