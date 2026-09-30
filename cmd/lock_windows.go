//go:build windows

package cmd

import "errors"

// ErrAlreadyRunning is returned when another weclaw bridge holds the instance lock.
var ErrAlreadyRunning = errors.New("another weclaw instance is already running")

// instanceLock is a no-op on Windows; single-instance protection is Unix-only.
type instanceLock struct{}

func acquireInstanceLock(path string) (*instanceLock, error) { return &instanceLock{}, nil }

// Release is a no-op on Windows.
func (l *instanceLock) Release() {}

// lockHolderPid is unsupported on Windows; callers fall back to the pid file.
func lockHolderPid(path string) (int, bool) { return 0, false }
