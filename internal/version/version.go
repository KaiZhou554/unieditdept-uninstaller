// Package version 提供版本信息。版本号就是构建日期，形如 260910。
package version

import (
	"os"
	"runtime/debug"
	"time"
)

// layout 是版本号的格式：两位年 + 两位月 + 两位日。
const layout = "060102"

// buildDate 由构建时注入，例如：
//
//	go build -ldflags "-X github.com/unieditdept/ued-uninstaller/internal/version.buildDate=260910"
//
// 使用仓库里的 build.ps1 会自动填入当天日期。
var buildDate = ""

// String 返回版本号，依次尝试：
//  1. 构建时注入的日期（build.ps1 会写入当天日期）；
//  2. 可执行文件自身的修改时间——链接完成的时间，即构建时间；
//  3. VCS 提交时间。
//
// 都取不到时返回 "dev"。
func String() string {
	if buildDate != "" {
		return buildDate
	}
	if t := executableTime(); !t.IsZero() {
		return t.Format(layout)
	}
	if t, ok := vcsTime(); ok {
		return t.Format(layout)
	}
	return "dev"
}

// executableTime 返回当前可执行文件的修改时间。
func executableTime() time.Time {
	exe, err := os.Executable()
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(exe)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// vcsTime 读取 go build 写入的 VCS 提交时间。
func vcsTime() (time.Time, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return time.Time{}, false
	}
	for _, setting := range info.Settings {
		if setting.Key != "vcs.time" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
