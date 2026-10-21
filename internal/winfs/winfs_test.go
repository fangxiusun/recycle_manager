package winfs

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// DriveType 只读系统卷表缓存：不存在的盘符必须回 DRIVE_NO_ROOT_DIR，而不是把请求发到设备上
// ——扫描就是靠这一条把失联的映射盘挡在探测之前的。
func TestDriveType(t *testing.T) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatalf("GetLogicalDrives 失败: %v", err)
	}
	for i := 0; i < 26; i++ {
		if mask>>i&1 != 0 {
			continue
		}
		free := string(rune('A' + i))
		if got := DriveType(free); got != windows.DRIVE_NO_ROOT_DIR {
			t.Errorf("空盘符 %s 的卷类型 = %d，期望 DRIVE_NO_ROOT_DIR(1)", free, got)
		}
		break
	}
	drive := systemDrive(t)
	if drive != "" {
		if got := DriveType(drive); got != windows.DRIVE_FIXED && got != windows.DRIVE_REMOVABLE {
			t.Errorf("系统盘 %s 的卷类型 = %d，期望本地卷", drive, got)
		}
	}
}

// systemDrive 从环境变量取系统盘符（首字母），拿不到就返回空串。
func systemDrive(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"SystemDrive", "HOMEDRIVE"} {
		if v := os.Getenv(key); len(v) >= 1 {
			return string(v[0])
		}
	}
	return ""
}
