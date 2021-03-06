package fuse

import (
	"reflect"
	"sync"
	"unsafe"
)

// #include "wrapper.h"
// #include <stdlib.h>  // for free()
import "C"

// State which tracks instances of RawFileSystem, with a unique identifier used
// by C code.  This avoids passing Go pointers into C code.
var fsMapLock sync.RWMutex
var rawFsMap map[int]RawFileSystem = make(map[int]RawFileSystem)
var nextFsId int = 1

// RegisterRawFs registers a filesystem with the bridge layer.
// Returns an integer id, which identifies the filesystem instance.
func RegisterRawFs(fs RawFileSystem) int {
	fsMapLock.Lock()
	defer fsMapLock.Unlock()

	id := nextFsId
	nextFsId++
	rawFsMap[id] = fs
	return id
}

func DeregisterRawFs(id int) {
	fsMapLock.Lock()
	defer fsMapLock.Unlock()

	delete(rawFsMap, id)
}

func GetRawFs(id int) RawFileSystem {
	fsMapLock.RLock()
	fs := rawFsMap[id]
	fsMapLock.RUnlock()
	return fs
}

func Version() int {
	return int(C.fuse_version())
}

//export ll_Init
func ll_Init(id C.int, cinfo *C.struct_fuse_conn_info) {
	fs := GetRawFs(int(id))
	info := &ConnInfo{}
	fs.Init(info)
}

//export ll_Destroy
func ll_Destroy(id C.int) {
	fs := GetRawFs(int(id))
	fs.Destroy()
}

//export ll_StatFs
func ll_StatFs(id C.int, ino C.fuse_ino_t, stat *C.struct_statvfs) C.int {
	fs := GetRawFs(int(id))
	s, err := fs.StatFs(int64(ino))
	if err == OK {
		s.toCStat(stat)
	}
	return C.int(err)
}

//export ll_Lookup
func ll_Lookup(id C.int, dir C.fuse_ino_t, name *C.char,
	cent *C.struct_fuse_entry_param) C.int {

	fs := GetRawFs(int(id))
	ent, err := fs.Lookup(int64(dir), C.GoString(name))
	if err == OK {
		ent.toCEntry(cent)
	}
	return C.int(err)
}

//export ll_Forget
func ll_Forget(id C.int, ino C.fuse_ino_t, n C.int) {
	fs := GetRawFs(int(id))
	fs.Forget(int64(ino), int(n))
}

//export ll_GetAttr
func ll_GetAttr(id C.int, ino C.fuse_ino_t, fi *C.struct_fuse_file_info,
	cattr *C.struct_stat, ctimeout *C.double) C.int {

	fs := GetRawFs(int(id))
	attr, err := fs.GetAttr(int64(ino), newFileInfo(fi))
	if err == OK {
		attr.toCStat(cattr, ctimeout)
	}
	return C.int(err)
}

//export ll_SetAttr
func ll_SetAttr(id C.int, ino C.fuse_ino_t, attr *C.struct_stat, toSet C.int,
	fi *C.struct_fuse_file_info, cattr *C.struct_stat, ctimeout *C.double) C.int {

	fs := GetRawFs(int(id))
	var ia InoAttr
	ia.fromCStat(attr)
	oattr, err := fs.SetAttr(int64(ino), &ia, SetAttrMask(toSet), newFileInfo(fi))
	if err == OK {
		oattr.toCStat(cattr, ctimeout)
	}
	return C.int(err)
}

//export ll_ReadDir
func ll_ReadDir(id C.int, ino C.fuse_ino_t, size C.size_t, off C.off_t,
	fi *C.struct_fuse_file_info, db *C.struct_DirBuf) C.int {

	fs := GetRawFs(int(id))
	writer := &dirBuf{db}
	err := fs.ReadDir(int64(ino), newFileInfo(fi), int64(off), int(size), writer)
	return C.int(err)
}

//export ll_Open
func ll_Open(id C.int, ino C.fuse_ino_t, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	info := newFileInfo(fi)
	err := fs.Open(int64(ino), info)
	if err == OK {
		fi.fh = C.uint64_t(info.Handle)
	}
	return C.int(err)
}

//export ll_OpenDir
func ll_OpenDir(id C.int, ino C.fuse_ino_t, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	info := newFileInfo(fi)
	err := fs.OpenDir(int64(ino), info)
	if err == OK {
		fi.fh = C.uint64_t(info.Handle)
	}
	return C.int(err)
}

//export ll_Release
func ll_Release(id C.int, ino C.fuse_ino_t, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	err := fs.Release(int64(ino), newFileInfo(fi))
	return C.int(err)
}

//export ll_ReleaseDir
func ll_ReleaseDir(id C.int, ino C.fuse_ino_t, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	err := fs.ReleaseDir(int64(ino), newFileInfo(fi))
	return C.int(err)
}

//export ll_FSync
func ll_FSync(id C.int, ino C.fuse_ino_t, datasync C.int, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	var dataOnly bool = datasync != 0
	err := fs.FSync(int64(ino), dataOnly, newFileInfo(fi))
	return C.int(err)
}

//export ll_FSyncDir
func ll_FSyncDir(id C.int, ino C.fuse_ino_t, datasync C.int, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	var dataOnly bool = datasync != 0
	err := fs.FSyncDir(int64(ino), dataOnly, newFileInfo(fi))
	return C.int(err)
}

//export ll_Flush
func ll_Flush(id C.int, ino C.fuse_ino_t, fi *C.struct_fuse_file_info) C.int {
	fs := GetRawFs(int(id))
	err := fs.Flush(int64(ino), newFileInfo(fi))
	return C.int(err)
}

//export ll_Read
func ll_Read(id C.int, ino C.fuse_ino_t, off C.off_t,
	fi *C.struct_fuse_file_info, buf unsafe.Pointer, size *C.int) C.int {

	fs := GetRawFs(int(id))

	// Create slice backed by C buffer.
	hdr := reflect.SliceHeader{
		Data: uintptr(buf),
		Len:  int(*size),
		Cap:  int(*size),
	}
	out := *(*[]byte)(unsafe.Pointer(&hdr))
	n, err := fs.Read(out, int64(ino), int64(off), newFileInfo(fi))
	if err == OK {
		*size = C.int(n)
	}
	return C.int(err)
}

//export ll_Write
func ll_Write(id C.int, ino C.fuse_ino_t, buf unsafe.Pointer, n *C.size_t, off C.off_t,
	fi *C.struct_fuse_file_info) C.int {

	fs := GetRawFs(int(id))
	// Create slice backed by C buffer.
	hdr := reflect.SliceHeader{
		Data: uintptr(buf),
		Len:  int(*n),
		Cap:  int(*n),
	}
	in := *(*[]byte)(unsafe.Pointer(&hdr))
	written, err := fs.Write(in, int64(ino), int64(off), newFileInfo(fi))
	if err == OK {
		*n = C.size_t(written)
	}
	return C.int(err)
}

//export ll_Mknod
func ll_Mknod(id C.int, dir C.fuse_ino_t, name *C.char, mode C.mode_t,
	rdev C.dev_t, cent *C.struct_fuse_entry_param) C.int {

	fs := GetRawFs(int(id))
	ent, err := fs.Mknod(int64(dir), C.GoString(name), int(mode), int(rdev))
	if err == OK {
		ent.toCEntry(cent)
	}
	return C.int(err)
}

//export ll_Access
func ll_Access(id C.int, ino C.fuse_ino_t, mask C.int) C.int {
	fs := GetRawFs(int(id))
	return C.int(fs.Access(int64(ino), int(mask)))
}

//export ll_Create
func ll_Create(id C.int, dir C.fuse_ino_t, name *C.char, mode C.mode_t,
	fi *C.struct_fuse_file_info, cent *C.struct_fuse_entry_param) C.int {

	fs := GetRawFs(int(id))
	info := newFileInfo(fi)
	ent, err := fs.Create(int64(dir), C.GoString(name), int(mode), info)
	if err == OK {
		ent.toCEntry(cent)
		fi.fh = C.uint64_t(info.Handle)
	}
	return C.int(err)
}

//export ll_Mkdir
func ll_Mkdir(id C.int, dir C.fuse_ino_t, name *C.char, mode C.mode_t,
	cent *C.struct_fuse_entry_param) C.int {

	fs := GetRawFs(int(id))
	ent, err := fs.Mkdir(int64(dir), C.GoString(name), int(mode))
	if err == OK {
		ent.toCEntry(cent)
	}
	return C.int(err)
}

//export ll_Rmdir
func ll_Rmdir(id C.int, dir C.fuse_ino_t, name *C.char) C.int {
	fs := GetRawFs(int(id))
	err := fs.Rmdir(int64(dir), C.GoString(name))
	return C.int(err)
}

//export ll_Symlink
func ll_Symlink(id C.int, link *C.char, parent C.fuse_ino_t, name *C.char,
	cent *C.struct_fuse_entry_param) C.int {
	fs := GetRawFs(int(id))
	ent, err := fs.Symlink(C.GoString(link), int64(parent), C.GoString(name))
	if err == OK {
		ent.toCEntry(cent)
	}
	return C.int(err)
}

//export ll_Link
func ll_Link(id C.int, ino C.fuse_ino_t, newparent C.fuse_ino_t, name *C.char,
	cent *C.struct_fuse_entry_param) C.int {
	fs := GetRawFs(int(id))
	ent, err := fs.Link(int64(ino), int64(newparent), C.GoString(name))
	if err == OK {
		ent.toCEntry(cent)
	}
	return C.int(err)
}

//export ll_ReadLink
func ll_ReadLink(id C.int, ino C.fuse_ino_t, err *C.int) *C.char {
	fs := GetRawFs(int(id))
	s, e := fs.ReadLink(int64(ino))
	*err = C.int(e)
	if e == OK {
		return C.CString(s)
	} else {
		return nil
	}
}

//export ll_Unlink
func ll_Unlink(id C.int, dir C.fuse_ino_t, name *C.char) C.int {
	fs := GetRawFs(int(id))
	err := fs.Unlink(int64(dir), C.GoString(name))
	return C.int(err)
}

//export ll_Rename
func ll_Rename(id C.int, dir C.fuse_ino_t, name *C.char,
	newdir C.fuse_ino_t, newname *C.char) C.int {

	fs := GetRawFs(int(id))
	err := fs.Rename(int64(dir), C.GoString(name), int64(newdir), C.GoString(newname))
	return C.int(err)
}

type dirBuf struct {
	db *C.struct_DirBuf
}

func (d *dirBuf) Add(name string, ino int64, mode int, next int64) bool {
	cstr := C.CString(name)
	res := C.DirBufAdd(d.db, cstr, C.fuse_ino_t(ino), C.int(mode), C.off_t(next))
	C.free(unsafe.Pointer(cstr))
	return res == 0
}

func newFileInfo(fi *C.struct_fuse_file_info) *FileInfo {
	if fi == nil {
		return nil
	}

	return &FileInfo{
		Flags:     int(fi.flags),
		Writepage: fi.writepage != 0,
		Handle:    uint64(fi.fh),
		LockOwner: uint64(fi.lock_owner),
	}
}

func (e *Entry) toCEntry(o *C.struct_fuse_entry_param) {
	o.ino = C.fuse_ino_t(e.Ino)
	o.generation = C.ulong(e.Generation)
	e.Attr.toCStat(&o.attr, nil)
	o.attr_timeout = C.double(e.AttrTimeout)
	o.entry_timeout = C.double(e.EntryTimeout)
}
