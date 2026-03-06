package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"gogogot/infra/tools"
	"strings"
	"time"
)

func SystemInfoTool() tools.Tool {
	return tools.Tool{
		Name:        "system_info",
		Description: "Get a full system overview in one call: hostname, uptime, load average, CPU, RAM, disk usage, top 5 processes by CPU, and network interfaces. Linux only.",
		Parameters:  map[string]any{},
		Handler:     systemInfo,
	}
}

func systemInfo(_ context.Context, _ map[string]any) tools.Result {
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

	sb.WriteString("\n--- CPU ---\n")
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cores := strings.Count(string(data), "processor\t:")
		fmt.Fprintf(&sb, "Cores: %d\n", cores)
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				fmt.Fprintf(&sb, "%s\n", strings.TrimSpace(line))
				break
			}
		}
	}

	sb.WriteString("\n--- Memory ---\n")
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") ||
				strings.HasPrefix(line, "MemFree:") ||
				strings.HasPrefix(line, "MemAvailable:") ||
				strings.HasPrefix(line, "SwapTotal:") ||
				strings.HasPrefix(line, "SwapFree:") {
				fmt.Fprintf(&sb, "%s\n", strings.TrimSpace(line))
			}
		}
	}

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

	return tools.Result{Output: sb.String()}
}
