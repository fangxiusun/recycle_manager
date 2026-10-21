// Package winfs 是对回收站文件的 Win32 级操作：存在性判断走 GetFileAttributesW
// （os.Stat/Lstat 在 Windows 上要 CreateFile，逐条判断时慢一个量级），删除前清只读属性
// 走 SetFileAttributesW。语义对齐 Python 版的 os.path.lexists / os.chmod(S_IWRITE)。
package winfs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// FiletimeEpochTicks 是 1601-01-01 到 1970-01-01 之间的 100 纳秒拍数。
const FiletimeEpochTicks = 116444736000000000

// FiletimeUnixMicro 把 FILETIME（UTC 100ns 拍）换成 Unix 微秒。
func FiletimeUnixMicro(ticks int64) int64 { return (ticks - FiletimeEpochTicks) / 10 }

// ModTimeMicro 用 FindFirstFile 取写入时间（100ns 精度），比 os.Stat 少一次 CreateFile。
func ModTimeMicro(path string) (int64, bool) {
	var data windows.Win32finddata
	h, err := windows.FindFirstFile(PathPtr(path), &data)
	if err != nil {
		return 0, false
	}
	windows.FindClose(h)
	ft := int64(data.LastWriteTime.HighDateTime)<<32 | int64(data.LastWriteTime.LowDateTime)
	return FiletimeUnixMicro(ft), true
}

// PathPtr 把路径转成 Win32 用的 UTF-16 指针。
func PathPtr(path string) *uint16 { return syscall.StringToUTF16Ptr(path) }

// DriveType 返回盘符的卷类型（windows.DRIVE_FIXED / DRIVE_REMOTE / DRIVE_CDROM…）。
// 它读的是系统卷表里的缓存，不会把请求发到网络服务器上，所以可以拿它在不碰盘的前提下
// 判断「这个盘值不值得探测」；不存在的盘返回 DRIVE_NO_ROOT_DIR。
func DriveType(drive string) uint32 {
	return windows.GetDriveType(PathPtr(drive + `:\`))
}

// Attrs 返回路径的 Win32 属性；路径不存在（或无权访问）时 ok 为 false。
// 对符号链接/目录 junction 取的是链接自身，等价于 lexists 而不是 exists。
func Attrs(path string) (attr uint32, ok bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	a, err := windows.GetFileAttributes(p)
	if err != nil {
		return 0, false
	}
	return a, true
}

// Exists 判断路径是否存在（不跟随重解析点）。
func Exists(path string) bool {
	_, ok := Attrs(path)
	return ok
}

// IsDir 判断是否为普通目录（重解析点按非目录处理，删除时只 unlink，同 Python 的 isdir && !islink）。
func IsDir(path string) bool {
	a, ok := Attrs(path)
	if !ok {
		return false
	}
	if a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return false
	}
	return a&windows.FILE_ATTRIBUTE_DIRECTORY != 0
}

// ClearReadOnly 去掉只读属性，让删除不被挡；文件已不存在时按成功处理。
func ClearReadOnly(path string) {
	if strings.HasSuffix(path, `\`) {
		path = strings.TrimRight(path, `\`)
	}
	_ = windows.SetFileAttributes(syscall.StringToUTF16Ptr(path), windows.FILE_ATTRIBUTE_NORMAL)
}

// RemoveFile 删除单个文件；不存在视为已删。
func RemoveFile(path string) error {
	ClearReadOnly(path)
	if err := syscall.DeleteFile(syscall.StringToUTF16Ptr(path)); err != nil {
		if err == syscall.ERROR_FILE_NOT_FOUND || err == syscall.ERROR_PATH_NOT_FOUND {
			return nil
		}
		return err
	}
	return nil
}

// RemoveTree 递归删除目录；逐层清只读后重试一次。返回第一个拿不掉的子项名。
// 与 Python 的 shutil.rmtree(onexc=…) 同口径：子项失败就跳过该子项继续，最后目录可能仍在。
func RemoveTree(path string) (skipped string, err error) {
	remove := func(p string) error {
		if e := syscall.DeleteFile(syscall.StringToUTF16Ptr(p)); e != nil {
			ClearReadOnly(p)
			if e2 := syscall.DeleteFile(syscall.StringToUTF16Ptr(p)); e2 != nil && e2 != syscall.ERROR_FILE_NOT_FOUND {
				return e2
			}
		}
		return nil
	}
	var walk func(p string) error
	walk = func(p string) error {
		entries, rerr := os.ReadDir(p)
		if rerr != nil {
			return rerr
		}
		for _, e := range entries {
			child := filepath.Join(p, e.Name())
			if e.IsDir() && !isReparse(child) {
				if werr := walk(child); werr != nil {
					if skipped == "" {
						skipped = e.Name()
					}
					continue
				}
				if derr := removeDir(child); derr != nil {
					if skipped == "" {
						skipped = e.Name()
					}
				}
				continue
			}
			if derr := remove(child); derr != nil {
				if skipped == "" {
					skipped = e.Name()
				}
			}
		}
		return nil
	}
	if err := walk(path); err != nil {
		return pathBase(path), err
	}
	if err := removeDir(path); err != nil {
		return pathBase(path), err
	}
	return "", nil
}

func isReparse(path string) bool {
	a, ok := Attrs(path)
	return ok && a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func removeDir(path string) error {
	ClearReadOnly(path)
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if e := syscall.RemoveDirectory(p); e != nil {
		if e == syscall.ERROR_FILE_NOT_FOUND || e == syscall.ERROR_PATH_NOT_FOUND {
			return nil
		}
		return e
	}
	return nil
}

func pathBase(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}
