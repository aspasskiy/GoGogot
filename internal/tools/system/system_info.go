package system

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/tools/types"
)

func SystemInfoTool() types.Tool {
	return types.Tool{
		Name:        "system_info",
		Label:       "Checking system",
		Description: "Get a full system overview in one call: hostname, uptime, load average, CPU, RAM, disk usage, top 5 processes by CPU, and network interfaces. Container-aware: reports cgroup limits when running inside a container. Linux only.",
		Parameters:  map[string]any{},
		Handler:     systemInfo,
	}
}

func systemInfo(_ context.Context, _ map[string]any) types.Result {
	var sb strings.Builder

	hostname, _ := os.Hostname()
	fmt.Fprintf(&sb, "Hostname: %s\n", hostname)

	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		var upSec float64
		_, _ = fmt.Sscanf(string(data), "%f", &upSec)
		d := time.Duration(upSec) * time.Second
		days := int(d.Hours()) / 24
		hours := int(d.Hours()) % 24
		mins := int(d.Minutes()) % 60
		fmt.Fprintf(&sb, "Uptime: %dd %dh %dm\n", days, hours, mins)
	}

	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			fmt.Fprintf(&sb, "Load average: %s %s %s\n", parts[0], parts[1], parts[2])
		}
	}

	writeCPU(&sb)
	writeMemory(&sb)

	sb.WriteString("\n--- Disk ---\n")
	if out, err := exec.Command("df", "-h", "/").CombinedOutput(); err == nil {
		sb.WriteString(string(out))
	}

	sb.WriteString("\n--- Top 5 processes by CPU ---\n")
	if out, err := exec.Command("ps", "aux", "--sort=-%cpu").CombinedOutput(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		limit := 6 // header + 5
		if len(lines) < limit {
			limit = len(lines)
		}
		for _, line := range lines[:limit] {
			sb.WriteString(line + "\n")
		}
	}

	sb.WriteString("\n--- Network ---\n")
	if out, err := exec.Command("ip", "-brief", "addr").CombinedOutput(); err == nil {
		sb.WriteString(string(out))
	} else if out, err := exec.Command("hostname", "-I").CombinedOutput(); err == nil {
		fmt.Fprintf(&sb, "IPs: %s\n", strings.TrimSpace(string(out)))
	}

	return types.Result{Output: sb.String()}
}

func writeCPU(sb *strings.Builder) {
	sb.WriteString("\n--- CPU ---\n")

	if cpus, ok := cgroupCPU(); ok {
		fmt.Fprintf(sb, "CPU limit (container): %.2f cores\n", cpus)
	}

	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cores := strings.Count(string(data), "processor\t:")
		fmt.Fprintf(sb, "CPU cores (node): %d\n", cores)
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				fmt.Fprintf(sb, "%s\n", strings.TrimSpace(line))
				break
			}
		}
	}
}

// cgroupCPU returns the CPU limit in cores from cgroup v2 or v1.
func cgroupCPU() (float64, bool) {
	if data, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		parts := strings.Fields(strings.TrimSpace(string(data)))
		if len(parts) == 2 && parts[0] != "max" {
			quota, e1 := strconv.ParseFloat(parts[0], 64)
			period, e2 := strconv.ParseFloat(parts[1], 64)
			if e1 == nil && e2 == nil && period > 0 {
				return quota / period, true
			}
		}
	}

	quota := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if quota > 0 && period > 0 {
		return float64(quota) / float64(period), true
	}

	return 0, false
}

func writeMemory(sb *strings.Builder) {
	sb.WriteString("\n--- Memory ---\n")

	limit, used, ok := cgroupMemory()
	if ok {
		fmt.Fprintf(sb, "Memory limit (container): %s\n", humanBytes(limit))
		fmt.Fprintf(sb, "Memory used  (container): %s\n", humanBytes(used))
		free := limit - used
		if free < 0 {
			free = 0
		}
		fmt.Fprintf(sb, "Memory free  (container): %s\n", humanBytes(free))
		return
	}

	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") ||
				strings.HasPrefix(line, "MemFree:") ||
				strings.HasPrefix(line, "MemAvailable:") ||
				strings.HasPrefix(line, "SwapTotal:") ||
				strings.HasPrefix(line, "SwapFree:") {
				fmt.Fprintf(sb, "%s\n", strings.TrimSpace(line))
			}
		}
	}
}

const noMemLimit = int64(1) << 62

// cgroupMemory returns limit and current usage in bytes from cgroup v2 or v1.
func cgroupMemory() (limit, used int64, ok bool) {
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" {
			if l, err := strconv.ParseInt(s, 10, 64); err == nil && l > 0 && l < noMemLimit {
				cur := readIntFile("/sys/fs/cgroup/memory.current")
				return l, cur, true
			}
		}
	}

	l := readIntFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if l > 0 && l < noMemLimit {
		cur := readIntFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
		return l, cur, true
	}

	return 0, 0, false
}

func readIntFile(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return -1
	}
	return v
}

func humanBytes(b int64) string {
	if b < 0 {
		return "N/A"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(b)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
