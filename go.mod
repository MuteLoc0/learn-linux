module learn-linux

go 1.27.4

require (
	golang.org/x/sys v0.50.0
	golang.org/x/term v0.48.0
	mvdan.cc/sh/v3 v3.14.1
)

replace mvdan.cc/sh/v3 => ./third_party/mvdan-sh
