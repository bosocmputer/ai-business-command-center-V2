package monitor

import "time"

// HostStats describes the machine the stack runs on. The API container reads
// /proc, which is not namespaced for these files, so the values are host-wide.
type HostStats struct {
	Cores                int     `json:"cores"`
	CPUPercent           float64 `json:"cpuPercent"`
	Load1                float64 `json:"load1"`
	Load5                float64 `json:"load5"`
	Load15               float64 `json:"load15"`
	MemoryTotalBytes     int64   `json:"memoryTotalBytes"`
	MemoryAvailableBytes int64   `json:"memoryAvailableBytes"`
	DiskTotalBytes       uint64  `json:"diskTotalBytes"`
	DiskUsedBytes        uint64  `json:"diskUsedBytes"`
	UptimeSeconds        int64   `json:"uptimeSeconds"`
}

// ContainerStats is one container's usage as reported by the host helper.
type ContainerStats struct {
	Name             string  `json:"name"`
	CPUPercent       float64 `json:"cpuPercent"`
	MemoryUsedBytes  int64   `json:"memoryUsedBytes"`
	MemoryLimitBytes int64   `json:"memoryLimitBytes"`
}

// Sample is one reading. Containers is empty and ContainersAvailable false when
// the host helper is not running or its file is stale.
type Sample struct {
	At                  time.Time        `json:"at"`
	Host                HostStats        `json:"host"`
	ContainersAvailable bool             `json:"containersAvailable"`
	Containers          []ContainerStats `json:"containers"`
}

// HistoryPoint is a Sample reduced for charting.
type HistoryPoint struct {
	At                time.Time        `json:"at"`
	CPUPercent        float64          `json:"cpuPercent"`
	MemoryUsedPercent float64          `json:"memoryUsedPercent"`
	Load1             float64          `json:"load1"`
	Containers        []ContainerStats `json:"containers"`
}
