# This Readme contains my notes on improving the db module.

The goal of the db module is to provide a very simple database interface that associates
logical information with C4 IDs, and possibly (but not necessarily) the opposite.  

An additional value could be that the DB module directly supports the storage module to provide storage features that needn't obligate the user to handle C4 IDs directly.  But if the DB module does not allow for that (under best focused design for the core goals), then this additional value can be moved to a db solution specific to storage module.

---

# Goals

The need of the DB module is to solve the problem of relating C4 IDs to meaning, and to relate C4 IDs together.

The very simplest feature would be a key->c4 database.
Then a database that holds metadata, so c4->metadata.
Database splitting so that the system is infinity scalable.
Then versions so, perhaps a key to sequence of sequential 'versions'.
    1 This could either be a simple list of version ordered IDs
    2 Or a nested (binary tree) of ids for previous IDs + ID of diff = new version ID

I don't think C4 should use #2 because it asserts a mechanism of modification, that may not be the actual mechanism, or the optimal way to 'compress' a set of 'versions'.  Consider for example 4 takes of a performance.  A generic 'diff' algorithm is unlikely to gain much in terms of compression due to the unlikelihood of 'matching data' to be conveniently aligned such that only the changes need to be retained. However, the problem remains that distributed systems my contain a list of versions that do not agree. One version missing from one tree other the other, or different missing items in each.

So looks like versions should be represented as a C4 tree.

Metadata could be similarly represented as a list of different metadata items (added as they are discovered)

So now we include a c4tree db so that we can recognize and skip 'partial' trees.

On top of all of this we need to know when a c4id has been seen or used, so that we can remove it form the database for storage if needed for a least recently used. 

May need to store universal time, bloom filters, usage count, access times, and other data in the db.

More generally this is simply a mapping of c4 to 'system' data (version, datetime, reference count, etc.)  This data seems to be mostly unidirectional (cumulative).

KV: Key -> C4

ID Trees: C4 -> [2]C4 (every node in c4 id tree)
Versions: C4 -> []C4
Attributes: C4 -> []C4

Temporary buckets (or db files)

## Interface Notes
When storing an *id.ID, it should store the root id of the tree, and also store the nodes of an ID tree as well.

- Key->ID: 
    + Core
        * `Put(key, *id.ID) error`
        * `Get(key) (*id.ID, bool)`
        * `Delete(key) (*id.ID)`
        * `List(prefix, stop) chan struct{Key, *id.ID}`
    + Non essential:  
        `Copy(key)`
        `Move(key)`

ID->Metadata/Attributes ID
ID -> []KEYs:
ID to ID tree

## Other Notes

#### Basic File System operations.

- create
- open
- read
- write
- sync
- seek
- close
- unlink
- rename



 

