package loader

import (
	"fmt"

	"github.com/Ken-Ingle/op-process-compose/src/types"
	"gopkg.in/yaml.v3"
)

// visit states used for cycle detection while resolving process `extends`.
const (
	extendsUnvisited = iota
	extendsInProgress
	extendsResolved
)

// resolveProcessExtends resolves process-level `extends` directives. A process
// that sets `extends: <name>` inherits the configuration of the referenced
// process, with the extending process's own values taking precedence.
//
// The merge reuses the same engine used for override files (mergeProcess):
// child scalar fields override the parent, list fields (entrypoint, args) are
// appended, and environment/depends_on are merged by key with the child
// winning.
//
// Resolution runs after all files are merged but before defaults, replica
// expansion, and template rendering, so derived processes inherit the raw
// user-supplied configuration. Transitive chains (c extends b extends a) are
// supported. Extending an unknown process, self-extension, and circular chains
// fail the load.
func resolveProcessExtends(p *types.Project) error {
	state := make(map[string]int, len(p.Processes))

	var resolve func(name string) error
	resolve = func(name string) error {
		switch state[name] {
		case extendsResolved:
			return nil
		case extendsInProgress:
			return fmt.Errorf("circular extends detected involving process %q", name)
		case extendsUnvisited:
			// fall through to resolve below
		}

		proc := p.Processes[name]
		if proc.Extends == "" {
			state[name] = extendsResolved
			return nil
		}

		parentName := proc.Extends
		if parentName == name {
			return fmt.Errorf("process %q cannot extend itself", name)
		}
		if _, ok := p.Processes[parentName]; !ok {
			return fmt.Errorf("process %q extends unknown process %q", name, parentName)
		}

		// Resolve the parent first so chains inherit the fully merged config.
		state[name] = extendsInProgress
		if err := resolve(parentName); err != nil {
			return err
		}

		// Start from an independent copy of the resolved parent and merge the
		// child on top so the child's values win. mergeProcess applies the same
		// append+override rules used when merging override files. The base must
		// be a deep copy: mergeProcess (mergo) merges into pointer fields in
		// place, so a shallow copy would mutate the parent's shared probe/logger
		// structs when both processes set them.
		base, err := deepCopyProcess(p.Processes[parentName])
		if err != nil {
			return fmt.Errorf("cannot copy process %q while resolving extends for %q: %w", parentName, name, err)
		}
		base.Extends = ""
		merged, err := mergeProcess(&base, &proc)
		if err != nil {
			return fmt.Errorf("cannot extend process %q from %q: %w", name, parentName, err)
		}
		merged.Extends = ""
		p.Processes[name] = *merged

		state[name] = extendsResolved
		return nil
	}

	for name := range p.Processes {
		if err := resolve(name); err != nil {
			return err
		}
	}
	return nil
}

// deepCopyProcess returns a fully independent copy of a process config so that
// merging a child onto its base never mutates the base — including pointer
// fields such as probes, logger, schedule and MCP configs. Extends resolution
// runs at load time on the raw, user-supplied config (before defaults and
// executable assignment), so a YAML round-trip is a faithful clone.
func deepCopyProcess(proc types.ProcessConfig) (types.ProcessConfig, error) {
	data, err := yaml.Marshal(proc)
	if err != nil {
		return types.ProcessConfig{}, err
	}
	var clone types.ProcessConfig
	if err := yaml.Unmarshal(data, &clone); err != nil {
		return types.ProcessConfig{}, err
	}
	return clone, nil
}
