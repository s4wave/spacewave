//go:build !windows

package launcher_helper

func prepareHelperExecutable(_ string, helperPath string) (string, func() error, error) {
	return helperPath, nil, nil
}
