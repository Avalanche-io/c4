# Name refusals

`name-refusals.txt` says, for each name and platform, whether a writer on
that platform holds the name as it is or refuses it, and by which rule. A
writer refuses an entry it cannot hold: it reports the path and the reason
and writes the rest; it never renames, maps or joins it into a path.

One case per line:

```
<goos> <hold|refuse> <class> <name, Go-quoted>
```

`goos` is `linux`, `darwin` or `windows`. `class` is `-` for hold, else the
first rule the name breaks, taken in this order: the bytes from the first,
each checked for `backslash`, then `char`, then `control`; then `trailing`;
then `device`. On darwin the only class is `utf8`. No name holds `/` or NUL
(no c4m name does), and `.` and `..` are not here (no c4m name is either).

| goos    | class       | Refuses                                                                                                                                                                                    | Source                                                         |
| ------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------- |
| linux   |             | nothing                                                                                                                                                                                    | measured: every byte but `/` and NUL is held                   |
| darwin  | `utf8`      | a name that is not valid UTF-8                                                                                                                                                             | measured on APFS: "Illegal byte sequence"                      |
| windows | `backslash` | a `\`                                                                                                                                                                                      | git v2.47.0 read-cache.c `verify_path_internal`                |
| windows | `char`      | any of `: < > " \| ? *`                                                                                                                                                                    | git v2.47.0 compat/mingw.c `is_valid_win32_path`               |
| windows | `control`   | a byte 0x01 to 0x1F (0x7F is held)                                                                                                                                                         | git v2.47.0 compat/mingw.c `is_valid_win32_path`               |
| windows | `trailing`  | a name ending in a space or a dot                                                                                                                                                          | git v2.47.0 compat/mingw.c `is_valid_win32_path`, not measured |
| windows | `device`    | a name whose part before its first `.` or `:`, trailing spaces trimmed, is AUX CON CONIN$ CONOUT$ NUL PRN, COM1 to COM9 or LPT0 to LPT9, in any case (`CON .txt` is refused, `CON x` held) | git v2.47.0 compat/mingw.c `is_valid_win32_path`, not measured |

Git's repository guards (`is_ntfs_dotgit`: `.git` look-alikes, `GIT~1`) are
not taken; they protect a repository, not a filesystem. APFS also refuses
some UTF-8 (noncharacters such as U+FFFE, unassigned code points); that is
left to the write to fail on and is not here. `trailing` and `device` are
git's rule, not a measurement: a writer using `\\?\` paths may hold them, and
a Windows measurement that they read back exactly would move them to hold.
Two names a folder takes for one (case, Unicode form) are not a per-name
rule and are not here.

How it was made: `fsname.Refusal` in c4-operator (`internal/fsname`, main
0e798151 and earlier from def67e3e) wrote every line; every windows line was
checked against a line-by-line Go port of git v2.47.0's
`is_valid_win32_path` (with `protect_ntfs`) and `verify_path_internal`'s
backslash and drive-prefix checks: 0 disagreements over these 647 names.
The same port agreed with `Refusal` on every name up to 5 bytes over a
23-symbol alphabet and on 2 million random names.
