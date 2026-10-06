//go:build !darwin

package cmd

import (
	"errors"
	"time"
)

var errServiceUnsupported = errors.New("weclaw service is only supported on macOS (launchd)")

func serviceInstalled() bool                   { return false }
func serviceStart() error                      { return errServiceUnsupported }
func serviceStop() error                       { return errServiceUnsupported }
func serviceRestart() (bool, error)            { return false, errServiceUnsupported }
func serviceReload() error                     { return errServiceUnsupported }
func servicePid() int                          { return 0 }
func waitServicePid(timeout time.Duration) int { return 0 }
func serviceInstall() error                    { return errServiceUnsupported }
func serviceUninstall() error                  { return errServiceUnsupported }
func plistPath() string                        { return "" }
