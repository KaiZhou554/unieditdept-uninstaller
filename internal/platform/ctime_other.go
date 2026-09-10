//go:build !windows

package platform

import (
	"os"
	"time"
)

// CreationTime 返回文件或目录的创建时间。
// 非 Windows 平台通常无法获取创建时间，回退为修改时间。
func CreationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
