package ilink

import (
	"errors"
	"fmt"
)

// RetNotPrepared is the sendmessage ret code iLink returns (errmsg "prepare
// failed") when it will not deliver a bot message to this user right now:
// the user has not messaged the bot recently enough (about 24 hours), or the
// bot already sent too many messages since the user's last one (about 10).
// Only a new message from the user reopens the window.
const RetNotPrepared = -2

// RetError is a business-level failure: HTTP 200 with a non-zero ret.
type RetError struct {
	Op     string // "send message", "send media", ...
	Ret    int
	ErrMsg string
}

func (e *RetError) Error() string {
	return fmt.Sprintf("%s failed: ret=%d errmsg=%s", e.Op, e.Ret, e.ErrMsg)
}

// IsNotPrepared reports whether err is iLink refusing the send because the
// user's reply window is closed or used up.
func IsNotPrepared(err error) bool {
	var re *RetError
	return errors.As(err, &re) && re.Ret == RetNotPrepared
}

// transportError is a failure to get any HTTP response at all (DNS, reset,
// EOF, per-attempt timeout) or a 5xx. Retrying may succeed.
type transportError struct {
	err error
}

func (e *transportError) Error() string { return e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

// NetworkError is returned once retries for transport failures are used up.
type NetworkError struct {
	Attempts int
	Err      error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error after %d attempts: %v", e.Attempts, e.Err)
}

func (e *NetworkError) Unwrap() error { return e.Err }

// IsNetworkError reports whether err means the iLink server could not be
// reached, even after retrying.
func IsNetworkError(err error) bool {
	var ne *NetworkError
	return errors.As(err, &ne)
}
