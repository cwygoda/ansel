//go:build !darwin

package massstorage

import "context"

func mountKnownCardVolumes(context.Context) error {
	return nil
}
