archlog
=======

Utility that generates a ChangeLog based on the last N elements in the svn log.

Think of it as a "svn log" to "changelog" conversion utility.

Example use:

```archlog 2```

Example output:

```
2014-03-17 arodseth
    * upgpkg: python-cx_freeze 4.3.2-2
2014-01-06 arodseth
    * upgpkg: python-cx_freeze 4.3.2-1
```

* License: GPL2
* Author: Alexander F Rødseth <xyproto@archlinux.org>
