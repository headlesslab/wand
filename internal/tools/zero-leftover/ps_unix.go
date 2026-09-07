//go:build !windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// listProcesses lists every process through ps, which Linux and macOS both
// ship: the id and the executable, a bare name on Linux and a path on macOS.
func listProcesses(ctx context.Context) ([]process, error) {
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return parsePS(string(out)), nil
}

// parsePS reads the ps output, one "<pid> <comm>" per line, the id padded on
// the left; comm may hold spaces, as "Google Chrome for Testing Helper
// (Renderer)" does.
func parsePS(out string) []process {
	processes := []process{}
	for _, line := range strings.Split(out, "\n") {
		pid, name, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		n, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		processes = append(processes, process{pid: n, name: strings.TrimSpace(name)})
	}
	return processes
}
