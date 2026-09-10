// Command ued-uninstaller 是一个 TUI 卸载程序：
// 扫描 %APPDATA%、%LOCALAPPDATA% 与 %TEMP% 下 unieditdept 目录中的软件数据，
// 合并同名目录的占用，并允许用户选择删除。
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/unieditdept/ued-uninstaller/internal/app"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/logging"
)

// version 由构建时注入：-ldflags "-X main.version=v1.0.0"
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "错误："+err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Default()

	showVersion := flag.Bool("version", false, "显示版本信息")
	namespace := flag.String("namespace", cfg.Namespace, "软件数据所在的命名空间目录名")
	dryRun := flag.Bool("dry-run", false, "演练模式：只统计可释放空间，不真正删除")
	noAnim := flag.Bool("no-anim", false, "关闭动画（低配终端或远程会话可用）")
	noPrune := flag.Bool("no-prune", false, "删除后保留空的命名空间目录")
	logPath := flag.String("log", "", "日志文件路径，为空则不记录")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ued-uninstaller %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return nil
	}

	cfg.Namespace = *namespace
	cfg.DryRun = *dryRun
	cfg.Animate = !*noAnim
	cfg.Prune = !*noPrune
	cfg.LogPath = *logPath

	closeLog, err := logging.Setup(cfg.LogPath)
	if err != nil {
		return fmt.Errorf("无法打开日志文件：%w", err)
	}
	defer closeLog()

	// 启动时先探测一次终端尺寸，避免首帧用默认尺寸渲染。
	width, height := app.TerminalSize()
	return app.Run(cfg, width, height)
}
