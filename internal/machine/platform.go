package machine

import "runtime"

var (
	runtimeOS           = runtime.GOOS
	runtimeArchitecture = runtime.GOARCH
)
