package tracer

import "testing"

func TestInitLogSmoke(t *testing.T) {
	InitLog(DEBUG, CONSOLE)
	LogDebug(ID_SYS, "debug smoke")
	LogInfo(ID_APP, "info smoke")
	LogWarn(ID_USER, "warn smoke")
	LogError(ID_DB, "error smoke")
}
