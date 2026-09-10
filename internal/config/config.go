// Package config 保存程序的运行配置。
package config

import "github.com/unieditdept/ued-uninstaller/internal/core"

// LangZH / LangEN 是界面语言的可选值。
const (
	LangZH = "zh"
	LangEN = "en"
)

// Config 是全局运行配置。
type Config struct {
	Namespace string // 命名空间目录名，默认 unieditdept
	Lang      string // 界面语言：zh / en，空表示默认
	DryRun    bool   // 演练模式：只统计不删除
	Animate   bool   // 是否播放动画
	Prune     bool   // 删除后是否清理空的命名空间目录
	LogPath   string // 日志文件路径，空表示不记录
}

// Default 返回默认配置。
func Default() Config {
	return Config{
		Namespace: core.DefaultNamespace,
		Lang:      LangZH,
		Animate:   true,
		Prune:     true,
	}
}
