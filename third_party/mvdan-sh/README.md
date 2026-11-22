# mvdan.cc/sh/v3

This directory contains the runtime Go sources from upstream v3.14.1 and its
BSD license. The root module uses this local copy to build LearnLinux.

LearnLinux's virtual runner uses a POSIX working directory and case-sensitive
shell variables on every host. Its globber uses that working directory and
emits POSIX paths. The runner delegates file operations, file ownership, and
file identity to the virtual filesystem. LearnLinux loads and validates sourced
scripts before the runner executes them. Directory stack state carries across
interactive commands.

When updating the dependency, compare the runtime files against the current
upstream tag and reapply the changes in `expand/expand.go` and
`interp/{api,builtin,runner,test,vars}.go`. The LearnLinux shell and app tests
exercise this runtime directly on every host.
