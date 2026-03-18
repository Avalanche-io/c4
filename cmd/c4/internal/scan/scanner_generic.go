//go:build !darwin && !linux
// +build !darwin,!linux

package scan

// scanDirectoryFast falls back to standard scanning on unsupported platforms
func (ps *ProgressiveScanner) scanDirectoryFast(dirPath string) error {
	// Fall back to standard scanDirectory
	ps.scanDirectory(dirPath)
	return nil
}

// EnableFastScan is a no-op on unsupported platforms
func (ps *ProgressiveScanner) EnableFastScan() {
	// No platform-specific optimizations available
}