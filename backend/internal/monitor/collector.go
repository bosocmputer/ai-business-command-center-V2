package monitor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// containerFileMaxAge is how old the host helper's file may be before it is
// treated as missing. The helper writes every few seconds.
const containerFileMaxAge = 30 * time.Second

type cpuTimes struct{ total, idle uint64 }

// Collector reads host resource use. It is not safe for concurrent use: CPU
// percentage is computed from the difference to the previous call.
type Collector struct {
	procRoot       string
	containersFile string
	diskPath       string
	now            func() time.Time
	previous       *cpuTimes
}

func NewCollector(procRoot, containersFile, diskPath string, now func() time.Time) *Collector {
	return &Collector{procRoot: procRoot, containersFile: containersFile, diskPath: diskPath, now: now}
}

func (c *Collector) Collect() (Sample, error) {
	host, err := c.readHost()
	if err != nil {
		return Sample{}, err
	}
	sample := Sample{At: c.now().UTC(), Host: host, Containers: []ContainerStats{}}
	if containers, err := readContainers(c.containersFile, c.now()); err == nil {
		sample.ContainersAvailable = true
		sample.Containers = containers
	}
	return sample, nil
}

func (c *Collector) readHost() (HostStats, error) {
	var host HostStats
	times, cores, err := readCPUTimes(filepath.Join(c.procRoot, "stat"))
	if err != nil {
		return host, err
	}
	host.Cores = cores
	if c.previous != nil && times.total > c.previous.total {
		totalDelta := float64(times.total - c.previous.total)
		idleDelta := float64(times.idle - c.previous.idle)
		host.CPUPercent = round1(clamp((totalDelta - idleDelta) / totalDelta * 100))
	}
	c.previous = &times

	if host.MemoryTotalBytes, host.MemoryAvailableBytes, err = readMemory(filepath.Join(c.procRoot, "meminfo")); err != nil {
		return host, err
	}
	if host.Load1, host.Load5, host.Load15, err = readLoad(filepath.Join(c.procRoot, "loadavg")); err != nil {
		return host, err
	}
	if seconds, err := readUptime(filepath.Join(c.procRoot, "uptime")); err == nil {
		host.UptimeSeconds = seconds
	}
	if c.diskPath != "" {
		var fs syscall.Statfs_t
		if err := syscall.Statfs(c.diskPath, &fs); err == nil {
			block := uint64(fs.Bsize)
			host.DiskTotalBytes = uint64(fs.Blocks) * block
			host.DiskUsedBytes = (uint64(fs.Blocks) - uint64(fs.Bfree)) * block
		}
	}
	return host, nil
}

func readCPUTimes(path string) (cpuTimes, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return cpuTimes{}, 0, err
	}
	defer file.Close()
	var times cpuTimes
	cores := 0
	found := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		if fields[0] != "cpu" {
			cores++
			continue
		}
		if len(fields) < 5 {
			return cpuTimes{}, 0, errors.New("unexpected /proc/stat cpu line")
		}
		for index, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return cpuTimes{}, 0, err
			}
			// Guest time is already counted inside user time; skip its columns.
			if index >= 8 {
				break
			}
			times.total += value
			if index == 3 || index == 4 {
				times.idle += value
			}
		}
		found = true
	}
	if err := scanner.Err(); err != nil {
		return cpuTimes{}, 0, err
	}
	if !found {
		return cpuTimes{}, 0, errors.New("/proc/stat has no cpu line")
	}
	if cores == 0 {
		cores = 1
	}
	return times, cores, nil
}

func readMemory(path string) (total, available int64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			available = value * 1024
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if total <= 0 {
		return 0, 0, errors.New("/proc/meminfo has no MemTotal")
	}
	return total, available, nil
}

func readLoad(path string) (one, five, fifteen float64, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 3 {
		return 0, 0, 0, errors.New("unexpected /proc/loadavg")
	}
	values := make([]float64, 3)
	for index := range values {
		if values[index], err = strconv.ParseFloat(fields[index], 64); err != nil {
			return 0, 0, 0, err
		}
	}
	return values[0], values[1], values[2], nil
}

func readUptime(path string) (int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 1 {
		return 0, errors.New("unexpected /proc/uptime")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	return int64(value), err
}

type containerFile struct {
	CheckedAt  time.Time `json:"checkedAt"`
	Containers []struct {
		Name     string `json:"Name"`
		CPUPerc  string `json:"CPUPerc"`
		MemUsage string `json:"MemUsage"`
	} `json:"containers"`
}

// readContainers parses the file written by deploy/host-monitor.sh, which holds
// the raw `docker stats --format '{{json .}}'` rows.
func readContainers(path string, now time.Time) ([]ContainerStats, error) {
	if path == "" {
		return nil, errors.New("container stats file is not configured")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file containerFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	if age := now.Sub(file.CheckedAt); age > containerFileMaxAge || age < -containerFileMaxAge {
		return nil, fmt.Errorf("container stats are %s old", age.Round(time.Second))
	}
	stats := make([]ContainerStats, 0, len(file.Containers))
	for _, row := range file.Containers {
		if row.Name == "" {
			continue
		}
		used, limit := parseMemoryUsage(row.MemUsage)
		stats = append(stats, ContainerStats{Name: row.Name, CPUPercent: parsePercent(row.CPUPerc), MemoryUsedBytes: used, MemoryLimitBytes: limit})
	}
	return stats, nil
}

func parsePercent(value string) float64 {
	number, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	if err != nil || number < 0 {
		return 0
	}
	return round1(number)
}

// parseMemoryUsage reads docker's "12.3MiB / 512MiB" form.
func parseMemoryUsage(value string) (used, limit int64) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseSize(parts[0]), parseSize(parts[1])
}

var sizeUnits = []struct {
	suffix string
	factor float64
}{
	{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
	{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1},
}

func parseSize(value string) int64 {
	value = strings.TrimSpace(value)
	for _, unit := range sizeUnits {
		if strings.HasSuffix(value, unit.suffix) {
			number, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 64)
			if err != nil || number < 0 {
				return 0
			}
			return int64(number * unit.factor)
		}
	}
	return 0
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func round1(value float64) float64 { return float64(int64(value*10+0.5)) / 10 }
