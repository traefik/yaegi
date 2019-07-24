// +build !nacl

package interp

import "os"

func getGoPath(options Options) string {
	if options.GoPath != "" {
		return options.GoPath
	}

	goPath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return goPath
}
