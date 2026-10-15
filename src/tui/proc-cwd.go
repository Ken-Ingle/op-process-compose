package tui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Ken-Ingle/op-process-compose/src/app"
	"github.com/Ken-Ingle/op-process-compose/src/client"
	"github.com/Ken-Ingle/op-process-compose/src/types"
	"github.com/rs/zerolog/log"
	puproc "github.com/shirou/gopsutil/v4/process"
)

const (
	// Reading a cwd is cheap next to port discovery, but processes start
	// lazily (wrappers spawn the real program late), so rescan fairly often.
	cwdInitialDelay    = 3 * time.Second
	cwdRefreshInterval = 10 * time.Second
)

// targetFrameworkRe matches a .NET target-framework folder, e.g. net8.0,
// net10.0, netcoreapp3.1, netstandard2.0, net8.0-windows.
var targetFrameworkRe = regexp.MustCompile(`^net(coreapp|standard)?\d+(\.\d+)*(-[A-Za-z0-9.]+)?$`)

func (pv *pcView) updateProcCwds(ctx context.Context) {
	// PIDs from a server on another machine mean nothing here.
	if !pv.isProjectLocal() {
		log.Debug().Msg("Cwd monitoring disabled for a non-local project")
		return
	}
	tree := app.NewProcessTree(0)
	timer := time.NewTimer(cwdInitialDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("Cwd monitoring canceled")
			return
		case <-timer.C:
			pv.refreshProcCwds(tree)
			timer.Reset(cwdRefreshInterval)
		}
	}
}

func (pv *pcView) refreshProcCwds(tree *app.ProcessTree) {
	// Fetch a private copy of the states: the shared pv.procStates is sorted
	// in place by the UI goroutine.
	states, err := pv.project.GetProcessesState()
	if err != nil {
		log.Err(err).Msg("failed to get processes state for cwds")
		return
	}
	if err := tree.Update(); err != nil {
		log.Err(err).Msg("failed to update process tree for cwds")
		return
	}
	children := tree.ChildPids()
	cwds := make(map[string]string)
	for _, state := range states.States {
		if !state.IsRunning || state.Pid <= 0 {
			continue
		}
		chain := leafChain(int32(state.Pid), children)
		if cwd := firstReadableCwd(chain, readCwd); cwd != "" {
			cwds[state.Name] = shortenCwd(cwd, pv.startDir, pv.homeDir)
		}
	}
	pv.procCwdsMtx.Lock()
	pv.procCwds = cwds
	pv.procCwdsMtx.Unlock()
}

func (pv *pcView) getStrForCwd(name string, running bool) string {
	if !running {
		return types.PlaceHolderValue
	}
	pv.procCwdsMtx.Lock()
	defer pv.procCwdsMtx.Unlock()
	if s := pv.procCwds[name]; s != "" {
		return s
	}
	return types.PlaceHolderValue
}

// getProcCwds returns the current cwd snapshot. refreshProcCwds replaces the
// map rather than mutating it, so the result is safe to read without a lock.
func (pv *pcView) getProcCwds() map[string]string {
	pv.procCwdsMtx.Lock()
	defer pv.procCwdsMtx.Unlock()
	return pv.procCwds
}

// isProjectLocal reports whether the project's PIDs belong to this machine:
// a local run, or a client attached over a unix socket or loopback address.
func (pv *pcView) isProjectLocal() bool {
	if !pv.project.IsRemote() {
		return true
	}
	pcClient, ok := pv.project.(*client.PcClient)
	return ok && pcClient.IsLocal()
}

// leafChain returns the path from the deepest descendant of root back up to
// root (leaf first). Ties at the deepest level go to the lowest PID; a root
// with no children yields just [root].
func leafChain(root int32, children map[int32][]int32) []int32 {
	parent := map[int32]int32{}
	visited := map[int32]bool{root: true}
	level := []int32{root}
	for {
		var next []int32
		for _, pid := range level {
			for _, child := range children[pid] {
				if visited[child] {
					continue
				}
				visited[child] = true
				parent[child] = pid
				next = append(next, child)
			}
		}
		if len(next) == 0 {
			break
		}
		level = next
	}
	leaf := slices.Min(level)
	chain := []int32{leaf}
	for pid := leaf; pid != root; {
		pid = parent[pid]
		chain = append(chain, pid)
	}
	return chain
}

// firstReadableCwd returns the cwd of the first PID in chain whose cwd can be
// read, or "" when none can.
func firstReadableCwd(chain []int32, read func(pid int32) (string, error)) string {
	for _, pid := range chain {
		if cwd, err := read(pid); err == nil && cwd != "" {
			return cwd
		}
	}
	return ""
}

func readCwd(pid int32) (string, error) {
	proc, err := puproc.NewProcess(pid)
	if err != nil {
		return "", err
	}
	return proc.Cwd()
}

// shortenCwd trims a trailing build-output tail (…/bin/output,
// …/bin/Debug/net8.0) and then shows the path relative to startDir ("./…"),
// else relative to home ("~/…"), else absolute.
func shortenCwd(cwd, startDir, home string) string {
	cwd = trimBuildOutput(cwd)
	if rest, ok := underDir(cwd, startDir); ok {
		if rest == "" {
			return "."
		}
		return "./" + rest
	}
	if rest, ok := underDir(cwd, home); ok {
		if rest == "" {
			return "~"
		}
		return "~/" + rest
	}
	return cwd
}

// trimBuildOutput cuts from the last /bin/ when what follows is build output:
// "output", or "Debug"/"Release" optionally followed by a target framework.
func trimBuildOutput(path string) string {
	i := strings.LastIndex(path, "/bin/")
	if i <= 0 {
		return path
	}
	tail := strings.Split(path[i+len("/bin/"):], "/")
	isBuildOutput := false
	switch tail[0] {
	case "output":
		isBuildOutput = len(tail) == 1
	case "Debug", "Release":
		isBuildOutput = len(tail) == 1 || (len(tail) == 2 && targetFrameworkRe.MatchString(tail[1]))
	}
	if !isBuildOutput {
		return path
	}
	return path[:i]
}

// underDir returns path relative to dir ("" when equal) if path is dir or
// inside it.
func underDir(path, dir string) (string, bool) {
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return "", false
	}
	if path == dir {
		return "", true
	}
	if rest, ok := strings.CutPrefix(path, dir+"/"); ok {
		return rest, true
	}
	return "", false
}

// resolvedDir resolves symlinks so the result compares equal to the kernel's
// cwd (which is always physical, e.g. /private/tmp rather than /tmp).
func resolvedDir(dir string, err error) string {
	if err != nil || dir == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

// cwdBaseDirs captures op-process-compose's own working directory and the
// user's home, once, for shortenCwd.
func cwdBaseDirs() (startDir, home string) {
	return resolvedDir(os.Getwd()), resolvedDir(os.UserHomeDir())
}
