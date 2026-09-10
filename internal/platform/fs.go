// Package platform 封装与操作系统相关的文件系统操作。
package platform

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenURL 用系统默认程序打开链接（通常是浏览器）。
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// rundll32 比 "cmd /c start" 更可靠，后者会把 & 之类当作命令分隔符。
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// PrepareForDelete 递归清除只读属性，尽量降低删除失败的概率。
// 该操作是尽力而为的：单个文件的失败不会影响整体。
func PrepareForDelete(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 无法访问的路径交给 RemoveAll 去报告
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		perm := info.Mode().Perm()
		if d.IsDir() {
			if perm&0o700 != 0o700 {
				_ = os.Chmod(path, perm|0o700)
			}
			return nil
		}
		if perm&0o600 != 0o600 {
			_ = os.Chmod(path, perm|0o600)
		}
		return nil
	})
}

// RemoveAll 尽力删除目录树：先直接删除，失败后清理只读属性再重试一次。
func RemoveAll(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	_ = PrepareForDelete(path)
	return os.RemoveAll(path)
}
