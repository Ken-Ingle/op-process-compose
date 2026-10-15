package tui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ken-Ingle/op-process-compose/src/types"
	"github.com/rs/zerolog/log"
)

const (
	// Port discovery scans the whole system socket table (on macOS it shells
	// out to lsof), so it runs far less often than the state refresh.
	portsInitialDelay    = 5 * time.Second
	portsRefreshInterval = 30 * time.Second
	maxPortsShown        = 4
)

func (pv *pcView) updateProcPorts(ctx context.Context) {
	timer := time.NewTimer(portsInitialDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("Ports monitoring canceled")
			return
		case <-timer.C:
			pv.refreshProcPorts()
			timer.Reset(portsRefreshInterval)
		}
	}
}

func (pv *pcView) refreshProcPorts() {
	// Fetch a private copy of the states: the shared pv.procStates is sorted
	// in place by the UI goroutine.
	states, err := pv.project.GetProcessesState()
	if err != nil {
		log.Err(err).Msg("failed to get processes state for ports")
		return
	}
	ports := make(map[string]string)
	for _, state := range states.States {
		if !state.IsRunning {
			continue
		}
		procPorts, err := pv.project.GetProcessPorts(state.Name)
		if err != nil {
			log.Debug().Err(err).Msgf("failed to get ports for %s", state.Name)
			continue
		}
		ports[state.Name] = formatPorts(procPorts)
	}
	pv.procPortsMtx.Lock()
	pv.procPorts = ports
	pv.procPortsMtx.Unlock()
}

func (pv *pcView) getStrForPorts(name string, running bool) string {
	if !running {
		return types.PlaceHolderValue
	}
	pv.procPortsMtx.Lock()
	defer pv.procPortsMtx.Unlock()
	if s := pv.procPorts[name]; s != "" {
		return s
	}
	return types.PlaceHolderValue
}

// formatPorts renders sorted, de-duplicated ports (IPv4 and IPv6 listeners on
// the same port are reported separately), e.g. "3000,8080,53/udp,+2".
func formatPorts(ports *types.ProcessPorts) string {
	var parts []string
	for _, p := range sortedUnique(ports.TcpPorts) {
		parts = append(parts, strconv.Itoa(int(p)))
	}
	for _, p := range sortedUnique(ports.UdpPorts) {
		parts = append(parts, strconv.Itoa(int(p))+"/udp")
	}
	if len(parts) > maxPortsShown {
		parts = append(parts[:maxPortsShown], "+"+strconv.Itoa(len(parts)-maxPortsShown))
	}
	return strings.Join(parts, ",")
}

func sortedUnique(ports []uint16) []uint16 {
	out := slices.Clone(ports)
	slices.Sort(out)
	return slices.Compact(out)
}
