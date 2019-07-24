// +build nacl

package interp

import "go/build"

func getGoPath(options Options) string {
	if options.GoPath != "" {
		return options.GoPath
	}

	return build.Default.GOPATH
}
