# `c4` Command Line Interface

C4 is a command line tool for generating C4 IDs for files, folders and piped data.

## Features

- Generate C4 IDs for files and directory hierarchies.
- Generate C4 IDs while copying files.
- Stores snapshots of hierarchy scans with filenames, C4 IDs and file metadata.
- Applies tags to file system snapshots.
- Compare two snapshots of the same folder at different times or two different folders.
- Provides a set of instructions that would synchronize one folder to another.

And more on the way.

The C4 tool is primarily used to quickly identifying files, and folders, it can do
this most efficiently in a media production pipeline by using it to copy files instead of
`cp` or `copy`. When copying `c4` will ID the file while copying at the same time and store the result for future reference. If a file is unmodified from the most recently
identified copy, then c4 uses the stored ID instead of re-identifying the file.

C4 can tag snapshots of the file system, and retain those snapshots for future reference.

C4 only stores the paths, file metadata, and C4 ID.  It is the users responsibility to 
retain copies of files if users wish to revert changes to previously tagged versions. C4 can provide a "patch" or set of operations that can be used make a directory match a snapshot or source directory.

To id a file

```bash
$ c4 id [filename]
c41YXr8u3uZC5kkwUr27TZkoYRYDprZmr8YCBJ13quTggGvjGrxMJzzF9qcoFhyGr5rxP2dMtySJevJqQbC3R3hzyE
```

To copy files or folders

```bash
$ c4 cp sourcefile targetfile
c42gCHUDtmQV2V7Zv3NCk1WFSVszLe4xCC7hRwUU1awTnUdjnTysxCoHmkVduWE4tX4dsJ4xNEZpNvuiC74vJiPxTJ: sourcefile -> targetfile 

$ c4 cp -R sourcefolder targetfolder
c41YXr8u3uZC5kkwUr27TZkoYRYDprZmr8YCBJ13quTggGvjGrxMJzzF9qcoFhyGr5rxP2dMtySJevJqQbC3R3hzyE: sourcefolder/file1.data -> targetfolder/file1.data
c45VWLhapqAhdyWeQH5rWr6WMdUsbDY2X3MAycQohPpvJJeLB2AAKqw8RmCNgLCGuAPw9Sg7ywurSCmH7xCf86Y9zN:
 sourcefolder/file2.data -> targetfolder/file2.data
...
```

### flags

```bash
  -a, --absolute: Output absolute paths, instead of relative paths.
  -d, --depth=0: Only output ids for files and folders 'depth' directories deep.
  -f, --formatting="id": Output formatting options.
          "id": c4id oriented.
          "path": path oriented.
  -L, --links: All symbolic links are followed.
  -m, --metadata: Include filesystem metadata.
          "path" is always included unless data is piped, or only a single file is specified.
  -R, --recursive: Recursively identify all files for the given path.
  -v, --version: Show version information.
```

