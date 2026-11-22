package vfs

// NewDefaultLinuxVFS seeds a Linux filesystem and creates the requested home.
func NewDefaultLinuxVFS(home string) *MemVFS {
	v := NewMemVFS()
	for _, p := range []string{"/bin", "/etc", "/home/learner", "/tmp", "/usr/bin", "/var/log"} {
		// These paths have no conflicting entries in a new filesystem.
		_ = v.MkdirAll(p, 0755)
	}
	if home == "" {
		home = "/home/learner"
	}
	home = absolutePath("/", home)
	_ = v.Chmod("/tmp", 01777)
	_ = v.WriteFile("/etc/hostname", []byte("linux-virtual\n"), 0644)
	_ = v.WriteFile("/etc/os-release", []byte("NAME=\"Debian GNU/Linux\"\nID=debian\n"), 0644)
	passwd := "root:x:0:0:root:/root:/bin/bash\nlearner:x:1000:1000:Learner:" + home + ":/bin/bash\n"
	_ = v.WriteFile("/etc/passwd", []byte(passwd), 0644)
	_ = v.MkdirAll(home, 0755)
	return v
}
