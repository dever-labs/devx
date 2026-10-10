//go:build !linux && !windows

package process

func isZombie(int) bool { return false }
