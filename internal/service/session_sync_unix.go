//go:build !windows

package service

import "errors"

func syncSessionDirectory(root sessionRoot) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
