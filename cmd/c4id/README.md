# c4id command

Command line tool to generate C4 identifiers.

```
Usage of c4id:
  -n	  omit final line-feed
  -src  (string) source file (rather than stdin)
  -help show usage
```

## Install

```
go install github.com/etcenter/c4go/cmd/c4id
```

## Examples

Generate from `echo`:

```
$ echo -n "abc" | c4id 
c45r4RMzsnvNMXSS1U8kZSziCePG1aiGvtf4SG2WzwjmSUTYEaXF4YAtDUwuvr9hCnBeYPLDwM8fygNMd4W1RJ9dyP
```

Specify a source:

```
c4id -src="/path/to/file"
```

Or use pipes:

```
c4id << cat /path/to/file
```
or
```
cat /path/to/file | c4id
```