package vfs

import (
	"io"
	"io/fs"
	"os"
	"slices"
	"time"

	"learn-linux/internal/contract"
)

func (v *MemVFS) ReadFile(p string) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, err := v.lookup(p, true)
	if err != nil {
		return nil, err
	}
	if e.node.info.IsDir() {
		return nil, contract.ErrIsDir
	}
	if e.node.info.Mode&0444 == 0 {
		return nil, contract.ErrPermission
	}
	return slices.Clone(e.node.data), nil
}

func (n *node) contentChanged() {
	n.info.Size = int64(len(n.data))
	n.info.ModTime = time.Now()
	n.info.IsVirtualBinary = false
	n.info.VirtualBinaryOutput = ""
	n.info.VirtualBinarySource = ""
}

func (v *MemVFS) fileForWrite(p string, mode fs.FileMode) (*node, error) {
	e, err := v.lookup(p, true)
	switch {
	case err == contract.ErrNotExist && e.parent != nil:
		return e.create(contract.TypeFile, mode), nil
	case err != nil:
		return nil, err
	case e.node.info.IsDir():
		return nil, contract.ErrIsDir
	default:
		return e.node, nil
	}
}

func (v *MemVFS) WriteFile(p string, data []byte, perm fs.FileMode) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	n, err := v.fileForWrite(p, defaultMode(perm, 0644))
	if err != nil {
		return err
	}
	n.data = slices.Clone(data)
	n.contentChanged()
	return nil
}

func (v *MemVFS) AppendFile(p string, data []byte, perm fs.FileMode) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	n, err := v.fileForWrite(p, defaultMode(perm, 0644))
	if err != nil {
		return err
	}
	n.data = append(n.data, data...)
	n.contentChanged()
	return nil
}

func (v *MemVFS) Touch(p string, modTime time.Time) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, err := v.lookup(p, true)
	if err == contract.ErrNotExist && e.parent != nil {
		e.node = e.create(contract.TypeFile, 0644)
	} else if err != nil {
		return err
	}
	if modTime.IsZero() {
		modTime = time.Now()
	}
	e.node.info.ModTime = modTime
	return nil
}

func (v *MemVFS) SetVirtualBinary(p string, perm fs.FileMode, output, sourceInfo string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	mode := defaultMode(perm, 0755)
	n, err := v.fileForWrite(p, mode)
	if err != nil {
		return err
	}
	n.data = []byte("\x7fELFvirtual-binary\n")
	n.contentChanged()
	n.info.Mode = mode & 07777
	n.info.IsVirtualBinary = true
	n.info.VirtualBinaryOutput = output
	n.info.VirtualBinarySource = sourceInfo
	return nil
}

type openFile struct {
	v        *MemVFS
	node     *node
	offset   int
	readable bool
	writable bool
	append   bool
	closed   bool
}

func (v *MemVFS) OpenFile(p string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	exclusive := flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL
	e, err := v.lookup(p, !exclusive)
	if err == nil && exclusive {
		return nil, contract.ErrExist
	}

	readable := flag&os.O_WRONLY == 0
	writable := flag&(os.O_WRONLY|os.O_RDWR) != 0
	switch {
	case err == contract.ErrNotExist && e.parent != nil && flag&os.O_CREATE != 0:
		e.node = e.create(contract.TypeFile, defaultMode(perm, 0644))
	case err != nil:
		return nil, err
	case e.node.info.IsDir() && (writable || flag&(os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0):
		return nil, contract.ErrIsDir
	case (readable && e.node.info.Mode&0444 == 0) || (writable && e.node.info.Mode&0222 == 0):
		return nil, contract.ErrPermission
	}
	if writable && flag&os.O_TRUNC != 0 {
		e.node.data = nil
		e.node.contentChanged()
	}
	return &openFile{
		v: v, node: e.node, readable: readable, writable: writable,
		append: flag&os.O_APPEND != 0,
	}, nil
}

func (f *openFile) Read(p []byte) (int, error) {
	f.v.mu.Lock()
	defer f.v.mu.Unlock()
	switch {
	case f.closed:
		return 0, fs.ErrClosed
	case !f.readable:
		return 0, contract.ErrPermission
	case f.node.info.IsDir():
		return 0, contract.ErrIsDir
	case len(p) == 0:
		return 0, nil
	case f.offset >= len(f.node.data):
		return 0, io.EOF
	}
	n := copy(p, f.node.data[f.offset:])
	f.offset += n
	return n, nil
}

func (f *openFile) Write(p []byte) (int, error) {
	f.v.mu.Lock()
	defer f.v.mu.Unlock()
	switch {
	case f.closed:
		return 0, fs.ErrClosed
	case !f.writable:
		return 0, contract.ErrPermission
	case len(p) == 0:
		return 0, nil
	}
	if f.append {
		f.offset = len(f.node.data)
	}
	end := f.offset + len(p)
	if end > len(f.node.data) {
		f.node.data = append(f.node.data, make([]byte, end-len(f.node.data))...)
	}
	copy(f.node.data[f.offset:end], p)
	f.offset = end
	f.node.contentChanged()
	return len(p), nil
}

func (f *openFile) Close() error {
	f.v.mu.Lock()
	defer f.v.mu.Unlock()
	if f.closed {
		return fs.ErrClosed
	}
	f.closed = true
	return nil
}
