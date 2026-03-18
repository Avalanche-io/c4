//go:build linux
// +build linux

package scan

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// scanDirectoryFast uses optimized syscalls on Linux
func (ps *ProgressiveScanner) scanDirectoryFast(dirPath string) error {
	// Open directory
	fd, err := syscall.Open(dirPath, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	
	// Get parent entry
	parentEntry, ok := ps.entries.Load(dirPath)
	if !ok {
		return nil
	}
	parent := parentEntry.(*ScanEntry)
	
	// Use getdents64 for faster directory reading
	buf := make([]byte, 64*1024) // 64KB buffer
	
	for {
		n, err := syscall.Getdents(fd, buf)
		if err != nil {
			return err
		}
		if n <= 0 {
			break
		}
		
		parseDirentsLinux(buf[:n], func(name string, typ uint8) {
			// Skip . and ..
			if name == "." || name == ".." {
				return
			}
			
			// Skip hidden files if configured
			if !ps.includeHidden && name[0] == '.' {
				return
			}
			
			fullPath := filepath.Join(dirPath, name)
			
			// Get file info to create metadata
			var info os.FileInfo
			var err error
			
			// Try to determine type from dirent
			var needsStat bool
			switch typ {
			case syscall.DT_DIR, syscall.DT_REG, syscall.DT_LNK:
				// We can create basic info from dirent type
				needsStat = true
			default:
				needsStat = true
			}
			
			if needsStat {
				info, err = os.Lstat(fullPath)
				if err != nil {
					return // Skip this entry
				}
			}
			
			// Create metadata and scan entry
			parentDepth := 0
			if parent.FileMetadata != nil {
				parentDepth = parent.FileMetadata.Depth()
			}
			md := NewFileMetadata(fullPath, info, parentDepth+1)
			
			entry := &ScanEntry{
				FileMetadata: md,
				Path:        fullPath,
				Stage:       StageStructure,
				parent:      parent,
			}
			
			// Store entry
			ps.entries.Store(fullPath, entry)
			atomic.AddInt64(&ps.totalFound, 1)
			
			// Add to parent's children
			parent.mu.Lock()
			parent.children = append(parent.children, entry)
			parent.mu.Unlock()
			
			// Queue for metadata scanning
			select {
			case ps.metadataChan <- entry:
			case <-ps.ctx.Done():
				return
			}
			
			// If directory, queue for recursion
			if entry.FileMetadata.IsDir() {
				select {
				case ps.structureChan <- fullPath:
				case <-ps.ctx.Done():
					return
				}
			}
		})
	}
	
	return nil
}

// LinuxDirent represents the Linux dirent structure
type LinuxDirent struct {
	Ino    uint64
	Off    int64
	Reclen uint16
	Type   uint8
	Name   [256]int8
}

// parseDirentsLinux parses the dirent structures from getdents
func parseDirentsLinux(buf []byte, fn func(name string, typ uint8)) {
	offset := 0
	for offset < len(buf) {
		if offset+19 > len(buf) { // Minimum size of dirent
			break
		}
		
		// Cast to dirent struct
		dirent := (*LinuxDirent)(unsafe.Pointer(&buf[offset]))
		
		// Check if we have a complete entry
		if offset+int(dirent.Reclen) > len(buf) {
			break
		}
		
		// Extract name
		nameBytes := (*[256]byte)(unsafe.Pointer(&dirent.Name[0]))
		name := string(nameBytes[:clen(nameBytes[:])])
		
		// Call callback
		fn(name, dirent.Type)
		
		// Move to next entry
		offset += int(dirent.Reclen)
	}
}

// clen returns the length of a null-terminated C string
func clen(b []byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == 0 {
			return i
		}
	}
	return len(b)
}

// EnableFastScan enables platform-specific optimizations
func (ps *ProgressiveScanner) EnableFastScan() {
	// Linux-specific optimizations enabled
}