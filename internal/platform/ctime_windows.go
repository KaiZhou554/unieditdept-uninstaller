//go:build windows

package platform

import (
	"os"
	"syscall"
	"time"
)

// CreationTime 返回文件或目录的创建时间。
// Windows 下通过文件属性数据中的 CreationTime 获取。
func CreationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		if ns := data.CreationTime.Nanoseconds(); ns > 0 {
			return time.Unix(0, ns)
		}
	}
	return info.ModTime()
}
