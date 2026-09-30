//go:build !windows

package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// ErrAlreadyRunning is returned when another weclaw bridge holds the instance lock.
var ErrAlreadyRunning = errors.New("another weclaw instance is already running")

// instanceLock is held for the lifetime of the bridge process. The kernel drops
// the flock when the process exits, including on crash, so a stale lock file
// never blocks a restart.
type instanceLock struct {
	f *os.File
}

// acquireInstanceLock takes an exclusive, non-blocking flock on path and records
// the current pid in it. When another process holds the lock, the returned error
// wraps ErrAlreadyRunning and names the holder's pid if it is known.
func acquireInstanceLock(path string) (*instanceLock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder := readLockPid(f)
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			if holder > 0 {
				return nil, fmt.Errorf("%w (pid=%d)", ErrAlreadyRunning, holder)
			}
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &instanceLock{f: f}, nil
}

// Release drops the lock. Safe to call on a nil lock.
func (l *instanceLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	l.f.Close()
	l.f = nil
}

func readLockPid(f *os.File) int {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	return pid
}

// lockHolderPid reports whether a live process holds the lock at path and, if
// recorded, its pid. It never takes the lock for longer than the probe.
func lockHolderPid(path string) (int, bool) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return readLockPid(f), true
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return 0, false
}
