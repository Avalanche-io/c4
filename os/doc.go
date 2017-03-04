// Package os implements c4 into os interactions like file reading and writing.
//
//
// This should be acquitted for most uses cases.  Certificates
// and certificate chains will be added in a future release.
//
// Philosophically we want C4 to stream data as soon as it's available.
// This influences order of operations for things like file systems walking.
//
// Normally a file walker gives us the info for the nearest folders first (breadth first).
// However, C4 must do a depth first traversal to compute IDs for folders.
//
// C4 addresses this by doing two passes when walking file systems.
// Fist we get metadata in breadth first order, then we fill in IDs.
//
// As the walker steps down it send FileInfo, it IDs files as it encounters them,
// as it steps up it IDs the folders.
//
// When identifying any folder we expect to see the following events:
// 1. Folder name, and metadata  (FileInfo)
// 2. child file and folder names, and metadata (FileInfo...)
// 3. child file IDs (ID...)
// 4. repeat 1-3 for every child folder. (FileInfo..., ID...)
// 5. folder ID (ID)
//
package os
