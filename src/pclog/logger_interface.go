package pclog

import "github.com/Ken-Ingle/op-process-compose/src/types"

type PcLogger interface {
	Open(filePath string, rotation *types.LoggerConfig)
	Info(message string, process string, replica int)
	Error(message string, process string, replica int)
	Close()
}
