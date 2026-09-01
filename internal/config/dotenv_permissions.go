//go:build !windows

package config

import (
	"io/fs"
	"os"
)

func validateDotEnvPermissions(path string, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return &DotEnvError{Path: path, Stage: StageStat, Err: err}
	}
	if info.Mode().Perm()&dotEnvPublicPermissionMask != 0 {
		return &DotEnvError{Path: path, Stage: StageOpen, Err: fs.ErrPermission}
	}
	return nil
}
