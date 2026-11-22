package devcmds

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"learn-linux/internal/contract"
)

type tarOptions struct {
	action   byte
	archive  string
	dir      string
	compress bool
	verbose  bool
	sources  []string
}

func parseTar(args []string) (tarOptions, error) {
	var opt tarOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opt.sources = append(opt.sources, args[i+1:]...)
			break
		}
		traditional := i == 0 && !strings.HasPrefix(arg, "-")
		if !traditional && (arg == "-" || !strings.HasPrefix(arg, "-")) {
			opt.sources = append(opt.sources, arg)
			continue
		}
		flags := strings.TrimPrefix(arg, "-")
		for j := 0; j < len(flags); j++ {
			flag := flags[j]
			switch flag {
			case 'c', 'x', 't':
				if opt.action != 0 && opt.action != flag {
					return opt, fmt.Errorf("you may not specify more than one of the '-cxt' options")
				}
				opt.action = flag
			case 'z':
				opt.compress = true
			case 'v':
				opt.verbose = true
			case 'f', 'C':
				var val string
				if j+1 < len(flags) {
					val = flags[j+1:]
					j = len(flags)
				} else {
					i++
					if i == len(args) {
						return opt, fmt.Errorf("option requires an argument -- '%c'", flag)
					}
					val = args[i]
				}
				if flag == 'f' {
					opt.archive = val
				} else {
					opt.dir = val
				}
			default:
				return opt, fmt.Errorf("invalid option -- '%c'", flag)
			}
		}
	}
	if opt.action == 0 || opt.archive == "" {
		return opt, fmt.Errorf("you must specify an operation (-c, -x, or -t) and an archive filename (-f)")
	}
	if opt.action == 'c' && len(opt.sources) == 0 {
		return opt, fmt.Errorf("cowardly refusing to create an empty archive")
	}
	return opt, nil
}

func tarCommand(ec *contract.ExecContext, args []string) int {
	opt, err := parseTar(args)
	if err != nil {
		return commandError(ec, "tar", "%v", err)
	}
	base := resolve(ec, ".")
	if opt.dir != "" {
		base = resolve(ec, opt.dir)
		info, err := ec.VFS.Stat(base)
		if err != nil {
			return commandError(ec, "tar", "%s: %v", opt.dir, err)
		}
		if !info.IsDir() {
			return commandError(ec, "tar", "%s: Not a directory", opt.dir)
		}
	}
	archive := resolve(ec, opt.archive)
	if opt.action == 'c' {
		err = createTar(ec, opt, base, archive)
	} else {
		err = readTar(ec, opt, base, archive)
	}
	if err != nil {
		return commandError(ec, "tar", "%v", err)
	}
	return 0
}

func createTar(ec *contract.ExecContext, opt tarOptions, base, archive string) error {
	var buf bytes.Buffer
	var output io.Writer = &buf
	var compressed *gzip.Writer
	if opt.compress {
		compressed = gzip.NewWriter(output)
		output = compressed
	}
	writer := tar.NewWriter(output)
	for _, name := range opt.sources {
		root := ec.VFS.ResolvePath(base, ec.Home, name)
		member := strings.TrimLeft(path.Clean(name), "/")
		if name == "~" || strings.HasPrefix(name, "~/") || !safeTarName(member) {
			member = path.Base(root)
		}
		if root == "/" {
			member = "."
		}
		err := ec.VFS.Walk(root, func(p string, info *contract.NodeInfo) error {
			if p == archive {
				return nil
			}
			suffix := strings.TrimPrefix(p, root)
			entry := path.Join(member, suffix)
			header := &tar.Header{
				Name: entry, Mode: int64(info.Mode & 07777), ModTime: info.ModTime,
				Typeflag: tar.TypeReg, Format: tar.FormatPAX,
			}
			var data []byte
			switch info.Type {
			case contract.TypeDir:
				header.Typeflag = tar.TypeDir
				header.Name += "/"
			case contract.TypeSymlink:
				header.Typeflag, header.Linkname = tar.TypeSymlink, info.LinkTarget
			case contract.TypeFile:
				var err error
				data, err = ec.VFS.ReadFile(p)
				if err != nil {
					return err
				}
				header.Size = int64(len(data))
			}
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			if _, err := writer.Write(data); err != nil {
				return err
			}
			if opt.verbose {
				fmt.Fprintln(ec.Stdout, header.Name)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if compressed != nil {
		if err := compressed.Close(); err != nil {
			return err
		}
	}
	return ec.VFS.WriteFile(archive, buf.Bytes(), 0644)
}

func readTar(ec *contract.ExecContext, opt tarOptions, base, archive string) error {
	data, err := ec.VFS.ReadFile(archive)
	if err != nil {
		return fmt.Errorf("%s: Cannot open: %w", opt.archive, err)
	}
	var input io.Reader = bytes.NewReader(data)
	var compressed *gzip.Reader
	if opt.compress || bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		compressed, err = gzip.NewReader(input)
		if err != nil {
			return err
		}
		defer compressed.Close()
		input = compressed
	}
	reader := tar.NewReader(input)
	dirs := make(map[string]tar.Header)
	var dirOrder []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if opt.action == 't' {
			if opt.verbose {
				fmt.Fprintf(ec.Stdout, "%s %d %s\n", header.FileInfo().Mode(), header.Size, header.Name)
			} else {
				fmt.Fprintln(ec.Stdout, header.Name)
			}
			continue
		}
		if err := extractTarEntry(ec.VFS, base, header, reader); err != nil {
			return fmt.Errorf("%s: %w", header.Name, err)
		}
		if header.Typeflag == tar.TypeDir {
			p := path.Join(base, header.Name)
			if _, exists := dirs[p]; !exists {
				dirOrder = append(dirOrder, p)
			}
			dirs[p] = *header
		}
		if opt.verbose {
			fmt.Fprintln(ec.Stdout, header.Name)
		}
	}
	if compressed != nil {
		// Consume the gzip trailer to validate its checksum.
		if _, err := io.Copy(io.Discard, compressed); err != nil {
			return err
		}
	}
	// Restore directory metadata after writing children.
	slices.SortFunc(dirOrder, func(a, b string) int {
		return strings.Count(b, "/") - strings.Count(a, "/")
	})
	for _, p := range dirOrder {
		header := dirs[p]
		if err := restoreTarMetadata(ec.VFS, p, &header); err != nil {
			return fmt.Errorf("%s: %w", header.Name, err)
		}
	}
	return nil
}

func safeTarName(name string) bool {
	return name != "" && !path.IsAbs(name) && !slices.Contains(strings.Split(name, "/"), "..")
}

func extractTarEntry(vfs contract.VFS, base string, header *tar.Header, reader io.Reader) error {
	if !safeTarName(header.Name) {
		return fmt.Errorf("unsafe archive member path")
	}
	name := path.Clean(header.Name)
	p := path.Join(base, name)
	if name == "." {
		if header.Typeflag != tar.TypeDir {
			return fmt.Errorf("unsafe archive member path")
		}
		return nil
	}

	// Lstat each component before allowing MkdirAll or WriteFile to follow it.
	parent := base
	parts := strings.Split(name, "/")
	for _, part := range parts[:len(parts)-1] {
		parent = path.Join(parent, part)
		info, err := vfs.Lstat(parent)
		if errors.Is(err, contract.ErrNotExist) {
			break
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return contract.ErrNotDir
		}
	}
	if err := vfs.MkdirAll(path.Dir(p), 0755); err != nil {
		return err
	}
	info, err := vfs.Lstat(p)
	if err != nil && !errors.Is(err, contract.ErrNotExist) {
		return err
	}
	switch header.Typeflag {
	case tar.TypeDir:
		if info != nil && !info.IsDir() {
			return contract.ErrNotDir
		}
		return vfs.MkdirAll(p, 0755)
	case tar.TypeSymlink:
		if info != nil {
			if info.IsDir() {
				return contract.ErrIsDir
			}
			if err := vfs.Remove(p); err != nil {
				return err
			}
		}
		return vfs.Symlink(header.Linkname, p)
	case tar.TypeReg, tar.TypeRegA:
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		if info != nil && info.Type == contract.TypeSymlink {
			if err := vfs.Remove(p); err != nil {
				return err
			}
		}
		if err := vfs.WriteFile(p, data, 0644); err != nil {
			return err
		}
		return restoreTarMetadata(vfs, p, header)
	default:
		return fmt.Errorf("unsupported archive entry type %q", header.Typeflag)
	}
}

func restoreTarMetadata(vfs contract.VFS, p string, header *tar.Header) error {
	if err := vfs.Chmod(p, fs.FileMode(header.Mode)&07777); err != nil {
		return err
	}
	if !header.ModTime.IsZero() {
		return vfs.Touch(p, header.ModTime)
	}
	return nil
}

func gzipCommand(cmd string, decompress bool) contract.CommandHandler {
	return func(ec *contract.ExecContext, args []string) int {
		decode := decompress
		var keep, stdout, force bool
		var files []string
		for i, arg := range args {
			if arg == "--" {
				files = append(files, args[i+1:]...)
				break
			}
			if arg == "-" || !strings.HasPrefix(arg, "-") {
				files = append(files, arg)
				continue
			}
			for _, flag := range arg[1:] {
				switch flag {
				case 'd':
					decode = true
				case 'k':
					keep = true
				case 'c':
					stdout = true
				case 'f':
					force = true
				default:
					return commandError(ec, cmd, "invalid option -- '%c'", flag)
				}
			}
		}
		if len(files) == 0 {
			if !stdout {
				return commandError(ec, cmd, "missing file operand")
			}
			files = []string{"-"}
		}
		status := 0
		for _, name := range files {
			if err := gzipFile(ec, name, decode, keep, stdout, force); err != nil {
				status = commandError(ec, cmd, "%s: %v", name, err)
			}
		}
		return status
	}
}

func gzipFile(ec *contract.ExecContext, name string, decode, keep, stdout, force bool) error {
	var data []byte
	var err error
	var mode fs.FileMode
	src := resolve(ec, name)
	if name == "-" {
		stdout = true
		if ec.Stdin != nil {
			data, err = io.ReadAll(ec.Stdin)
		}
	} else {
		info, statErr := ec.VFS.Lstat(src)
		if statErr != nil {
			return statErr
		}
		if info.Type == contract.TypeSymlink && !stdout {
			return fmt.Errorf("is a symbolic link -- ignored")
		}
		mode = info.Mode & 07777
		data, err = ec.VFS.ReadFile(src)
	}
	if err != nil {
		return err
	}
	dst := src + ".gz"
	if decode {
		var ok bool
		dst, ok = strings.CutSuffix(src, ".gz")
		if !stdout && (!ok || path.Base(src) == ".gz") {
			return fmt.Errorf("unknown suffix -- ignored")
		}
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		data, err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return err
		}
	} else {
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		if _, err := writer.Write(data); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
		data = buf.Bytes()
	}
	if stdout {
		_, err := ec.Stdout.Write(data)
		return err
	}
	if err := writeGzipFile(ec.VFS, dst, data, mode, force); err != nil {
		return fmt.Errorf("%s: %w", dst, err)
	}
	if !keep {
		return ec.VFS.Remove(src)
	}
	return nil
}

func writeGzipFile(vfs contract.VFS, dst string, data []byte, mode fs.FileMode, force bool) error {
	if force {
		info, err := vfs.Lstat(dst)
		if err == nil && info.Type == contract.TypeSymlink {
			if err := vfs.Remove(dst); err != nil {
				return err
			}
		}
		return vfs.WriteFile(dst, data, mode)
	}
	file, err := vfs.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return errors.Join(err, file.Close())
}
