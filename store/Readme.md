# C4 Store
C4 Store is a go package that implements a simple virtual file system using the C4 DB and C4 ID systems. On the back-end it stores files in a user defined path with the filename set to the C4 ID of it's contents. On the front-end C4 presents a normal folder hierarchy and provides the typical file system functions.  

Because of the way C4 IDs work in the underlying implementation you can think of a C4 Store as a copy-on-write overly file system.

# Features

- Files are implicitly De-duplicated.
- Copy and Move operations are free.
- Free atomic 'point-in-time' snapshots.
- Multiple independent 'views' of the folder structure.
- Read-only and Read-write checkouts to a physical file system.
- Remote file system synchronization tools.
- Indelible metadata, with json export.
- Arbitrary user defined relationships between files.
- New container types in addition to folders, like frame sequences.
- Zero padding in filenames not required, 'natural' sorting order.
- Absolute (c4 ID) and logical (file path) file access.
- Remote views and asynchronous modification.
- Non-blocking, conflict free concurrent access.
- Watchers, TTL, hard and soft delete, event triggers and more.
