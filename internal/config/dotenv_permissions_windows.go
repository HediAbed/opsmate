//go:build windows

package config

import "os"

func validateDotEnvPermissions(string, *os.File) error {
	return nil
}
