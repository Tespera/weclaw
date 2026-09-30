//go:build !windows

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestInstanceLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weclaw.lock")

	if _, running := lockHolderPid(path); running {
		t.Fatal("no lock file yet, want not running")
	}

	first, err := acquireInstanceLock(path)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// flock is per open file description, so a second open in the same
	// process contends exactly like a second weclaw process would.
	_, err = acquireInstanceLock(path)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire error = %v, want ErrAlreadyRunning", err)
	}
	if !strings.Contains(err.Error(), "pid="+strconv.Itoa(os.Getpid())) {
		t.Errorf("error %q should name holder pid %d", err, os.Getpid())
	}

	pid, running := lockHolderPid(path)
	if !running || pid != os.Getpid() {
		t.Fatalf("lockHolderPid = (%d, %v), want (%d, true)", pid, running, os.Getpid())
	}

	first.Release()
	if _, running := lockHolderPid(path); running {
		t.Fatal("after release, want not running")
	}
	again, err := acquireInstanceLock(path)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	again.Release()
	again.Release() // idempotent
}
