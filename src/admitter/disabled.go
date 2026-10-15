package admitter

import "github.com/Ken-Ingle/op-process-compose/src/types"

type DisabledProcAdmitter struct {
}

func (d *DisabledProcAdmitter) Admit(proc *types.ProcessConfig) bool {
	return !proc.Disabled
}
