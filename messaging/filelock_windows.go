//go:build windows

package messaging

// lockFile is a no-op on Windows; concurrent state writes there are rare
// enough (bridge + an occasional `weclaw send`) that the atomic rename suffices.
func lockFile(path string) (func(), error) {
	return func() {}, nil
}
