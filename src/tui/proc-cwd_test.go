package tui

import (
	"errors"
	"slices"
	"testing"

	"github.com/Ken-Ingle/op-process-compose/src/types"
)

func TestShortenCwd(t *testing.T) {
	const (
		start = "/Users/ken/src/ope"
		home  = "/Users/ken"
	)
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"trims bin/output", "/Users/ken/src/ope/IRIS.Cloud.WebAPI/IRIS.Encounter.Backend.Admin/bin/output", "./IRIS.Cloud.WebAPI/IRIS.Encounter.Backend.Admin"},
		{"trims bin/Debug/tfm", "/Users/ken/src/ope/IRIS.Test.Automation/Decoy/src/Decoy/bin/Debug/net10.0", "./IRIS.Test.Automation/Decoy/src/Decoy"},
		{"trims bin/Release", "/opt/app/bin/Release", "/opt/app"},
		{"trims last bin only", "/opt/bin/tools/bin/Release/netcoreapp3.1", "/opt/bin/tools"},
		{"keeps plain bin dir", "/Users/ken/src/ope/bin", "./bin"},
		{"keeps unrelated bin subdir", "/opt/app/bin/scripts", "/opt/app/bin/scripts"},
		{"keeps non-tfm after Debug", "/opt/app/bin/Debug/assets", "/opt/app/bin/Debug/assets"},
		{"equal to start dir", "/Users/ken/src/ope", "."},
		{"relative to start dir", "/Users/ken/src/ope/IRIS.Web.Blazor", "./IRIS.Web.Blazor"},
		{"start dir prefix is not a parent", "/Users/ken/src/ope-other", "~/src/ope-other"},
		{"home", "/Users/ken/src/ext/process-compose", "~/src/ext/process-compose"},
		{"equal to home", "/Users/ken", "~"},
		{"absolute", "/private/tmp/work", "/private/tmp/work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenCwd(tt.cwd, start, home); got != tt.want {
				t.Errorf("shortenCwd(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestShortenCwdUnknownBaseDirs(t *testing.T) {
	if got := shortenCwd("/Users/ken/src", "", ""); got != "/Users/ken/src" {
		t.Errorf("shortenCwd() = %q, want absolute", got)
	}
}

func TestLeafChain(t *testing.T) {
	tests := []struct {
		name     string
		children map[int32][]int32
		want     []int32
	}{
		{"no children", map[int32][]int32{}, []int32{10}},
		// python wrapper → func start → func host
		{"linear", map[int32][]int32{10: {20}, 20: {30}}, []int32{30, 20, 10}},
		{"deepest wins over lower pid", map[int32][]int32{10: {11, 50}, 50: {60}}, []int32{60, 50, 10}},
		{"tie goes to lowest pid", map[int32][]int32{10: {40, 20}, 40: {41}, 20: {25}}, []int32{25, 20, 10}},
		{"ignores cycles", map[int32][]int32{10: {20}, 20: {10, 30}}, []int32{30, 20, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leafChain(10, tt.children); !slices.Equal(got, tt.want) {
				t.Errorf("leafChain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstReadableCwd(t *testing.T) {
	cwds := map[int32]string{20: "/wrapper/state", 10: "/root"}
	read := func(pid int32) (string, error) {
		if cwd, ok := cwds[pid]; ok {
			return cwd, nil
		}
		return "", errors.New("permission denied")
	}
	if got := firstReadableCwd([]int32{30, 20, 10}, read); got != "/wrapper/state" {
		t.Errorf("firstReadableCwd() = %q, want nearest readable ancestor", got)
	}
	if got := firstReadableCwd([]int32{30}, read); got != "" {
		t.Errorf("firstReadableCwd() = %q, want empty", got)
	}
}

func TestSortByCwd(t *testing.T) {
	states := &types.ProcessesState{States: []types.ProcessState{
		{Name: "b", IsRunning: true},
		{Name: "stopped", IsRunning: false},
		{Name: "a", IsRunning: true},
		{Name: "c", IsRunning: true},
	}}
	cwds := map[string]string{"a": "./z", "b": "./a", "c": "./a", "stopped": "./0"}
	if err := sortProcessesState(ProcessStateCwd, true, states, cwds); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range states.States {
		got = append(got, s.Name)
	}
	if want := []string{"stopped", "b", "c", "a"}; !slices.Equal(got, want) {
		t.Errorf("sorted = %v, want %v", got, want)
	}
}
