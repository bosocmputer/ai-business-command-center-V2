package monitor

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeProc(t *testing.T, dir string, user, idle uint64) {
	t.Helper()
	writeFile(t, dir, "stat", "cpu  "+strconv.FormatUint(user, 10)+" 0 0 "+strconv.FormatUint(idle, 10)+" 0 0 0 0 99 0\ncpu0 1 0 0 1 0 0 0 0 0 0\ncpu1 1 0 0 1 0 0 0 0 0 0\nintr 1\n")
	writeFile(t, dir, "meminfo", "MemTotal:        8000000 kB\nMemFree:          100 kB\nMemAvailable:    2000000 kB\n")
	writeFile(t, dir, "loadavg", "0.50 0.75 1.00 1/200 12345\n")
	writeFile(t, dir, "uptime", "3600.55 7000.00\n")
}

func TestCollectorComputesHostStatsFromProc(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	collector := NewCollector(dir, "", "", func() time.Time { return now })

	writeProc(t, dir, 100, 900)
	first, err := collector.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if first.Host.CPUPercent != 0 || first.Host.Cores != 2 {
		t.Fatalf("first sample = %+v; CPU must be 0 until a baseline exists and cores counted from cpuN lines", first.Host)
	}
	if first.Host.MemoryTotalBytes != 8000000*1024 || first.Host.MemoryAvailableBytes != 2000000*1024 {
		t.Fatalf("memory = %+v", first.Host)
	}
	if first.Host.Load1 != 0.5 || first.Host.Load15 != 1 || first.Host.UptimeSeconds != 3600 {
		t.Fatalf("load/uptime = %+v", first.Host)
	}
	if first.ContainersAvailable || len(first.Containers) != 0 {
		t.Fatalf("containers = %+v; want unavailable without a file", first)
	}

	// 300 busy ticks and 100 idle ticks between samples is 75% busy. Guest time
	// (column 9) must not be double counted.
	writeProc(t, dir, 400, 1000)
	second, err := collector.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if second.Host.CPUPercent != 75 {
		t.Fatalf("CPU = %v, want 75", second.Host.CPUPercent)
	}
}

func TestCollectorReportsMissingProc(t *testing.T) {
	collector := NewCollector(t.TempDir(), "", "", time.Now)
	if _, err := collector.Collect(); err == nil {
		t.Fatal("expected an error when /proc files are missing")
	}
}

func TestReadContainersParsesDockerStatsAndRejectsStaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "containers.json")
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	writeFile(t, dir, "containers.json", `{"version":1,"checkedAt":"2026-09-30T11:59:55Z","containers":[
	  {"Name":"ai-bcc-v2-api-1","CPUPerc":"1.25%","MemUsage":"48.5MiB / 512MiB"},
	  {"Name":"hermes-shop1","CPUPerc":"0.00%","MemUsage":"1.5GiB / 15.6GiB"},
	  {"Name":"","CPUPerc":"1%","MemUsage":"1B / 2B"},
	  {"Name":"odd","CPUPerc":"--","MemUsage":"nonsense"}]}`)

	got, err := readContainers(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("containers = %+v; unnamed row must be skipped", got)
	}
	if got[0].Name != "ai-bcc-v2-api-1" || got[0].CPUPercent != 1.3 {
		t.Fatalf("api = %+v", got[0])
	}
	if got[0].MemoryUsedBytes != int64(48.5*(1<<20)) || got[0].MemoryLimitBytes != 512<<20 {
		t.Fatalf("api memory = %+v", got[0])
	}
	if got[1].MemoryUsedBytes != int64(1.5*(1<<30)) {
		t.Fatalf("hermes memory = %+v", got[1])
	}
	if got[2].CPUPercent != 0 || got[2].MemoryUsedBytes != 0 {
		t.Fatalf("unparseable row = %+v; want zeros not an error", got[2])
	}

	if _, err := readContainers(path, now.Add(2*time.Minute)); err == nil {
		t.Fatal("a stale file must be rejected")
	}
	if _, err := readContainers(filepath.Join(dir, "missing.json"), now); err == nil {
		t.Fatal("a missing file must be rejected")
	}
}
