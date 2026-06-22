//go:build !unix

package store

// lockFile is a no-op on platforms without POSIX advisory file locking. jl
// targets macOS and Linux; this keeps the build green elsewhere.
func lockFile(path string) (func() error, error) {
	return func() error { return nil }, nil
}
