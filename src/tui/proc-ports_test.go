package tui

import (
	"testing"

	"github.com/f1bonacc1/process-compose/src/types"
)

func TestFormatPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports types.ProcessPorts
		want  string
	}{
		{"empty", types.ProcessPorts{}, ""},
		{"dedupes v4/v6 and sorts", types.ProcessPorts{TcpPorts: []uint16{8080, 3000, 8080}}, "3000,8080"},
		{"udp suffixed after tcp", types.ProcessPorts{TcpPorts: []uint16{80}, UdpPorts: []uint16{53, 53}}, "80,53/udp"},
		{"truncates", types.ProcessPorts{TcpPorts: []uint16{1, 2, 3, 4, 5, 6}}, "1,2,3,4,+2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPorts(&tt.ports); got != tt.want {
				t.Errorf("formatPorts() = %q, want %q", got, tt.want)
			}
		})
	}
}
