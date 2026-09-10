[English](README.md) | 简体中文

# ued-uninstaller

用 Go 编写的 TUI 卸载程序，用于清理 `unieditdept` 系列软件在三个位置留下的数据。

## 扫描什么

**UniEditDept 软件** —— 下面三个位置下的 `unieditdept/<名称>` 子目录。三个位置下的同名目录合并为一个软件：占用相加，删除时一并清除。

| 位置 | 环境变量 |
| --- | --- |
| Roaming | `%APPDATA%` |
| Local | `%LOCALAPPDATA%` |
| Temp | `%TEMP%` |

**其它软件**（顺带发现） —— `%APPDATA%` **根目录**下、名字以 `.exe` 结尾且含 `EBWebView` 的**文件夹**。它们属于别的程序（多为 WebView2 应用）而非 UniEditDept，因此单独分区并附一条谨慎提示；显示时去掉 `.exe`，按 `D` 删除整个文件夹。

## 使用

```powershell
.\build.ps1            # 构建并写入当天日期作为版本号
.\ued-uninstaller.exe
```

```
-namespace string   软件数据所在的命名空间目录名（默认 "unieditdept"）
-lang string        界面语言：zh 或 en（默认 zh）
-dry-run            只统计可释放空间，不真正删除
-no-anim            关闭动画
-no-prune           删除后保留空的命名空间目录
-log string         日志文件路径，为空则不记录
-version            显示版本信息
```

## 快捷键

| 按键 | 作用 |
| --- | --- |
| `↑` `↓` / `k` `j` | 移动光标 |
| `PgUp` `PgDn` / `Home` `End` | 翻页 / 跳到首尾 |
| `Space` | 选择或取消选择 |
| `A` / `I` | 全选或全不选 / 反选 |
| `S` | 切换排序（占用 / 名称 / 日期） |
| `/` | 按名称过滤，`Esc` 清空 |
| `D` | 卸载：进入待确认，再按 `D`/`Y` 执行 |
| `L` | 切换语言 |
| `P` | 打开 GitHub 仓库 |
| `R` | 重新扫描 |
| `?` | 帮助 |
| `Q` / `Esc` / `Ctrl+C` | 退出 |

## 鼠标

| 操作 | 效果 |
| --- | --- |
| 单击软件项 | 光标移过去并切换选中 |
| 单击底部按键提示 | 等同按下该键 |
| 单击 `P` / `L` / 某个语言 | 打开仓库 / 下一个语言 / 该语言 |
| 单击 `GitHub` | 打开仓库 |
| 单击顶部标题 | 回到主界面（关闭帮助、取消待确认） |
| 滚轮 | 移动光标 |

## 界面

- **进入即列出**：启动时只枚举目录名，占用在后台并发统计并逐行回填。
- **两段式卸载**：`D` 进入 6 秒待确认（选中行与「确认」提示发光），`D`/`Y` 执行，其它任意键或超时都取消。
- **右侧面板**跟随阶段：统计进度 → 待卸载摘要 → 卸载进度 → 结果。
- 非 UniEditDept 的条目刻意换了一套视觉：文字为灰色，确认时用蓝紫光带而非粉红。
- `Enter` 不做任何绑定，避免和 `/` 过滤时的回车混淆。

## 构建

版本号即构建日期（`yyMMdd`）。`build.ps1` 会注入当天日期并生成 Windows 的 VERSIONINFO 资源 —— 资源管理器「属性 → 详细信息」里的字段就来自它，而 Go 默认不写。生成的 `resource_windows_*.syso` 不入库，但 `go build` 会自动链接项目根目录下的它，所以手动构建前先跑一次 `build.ps1`。

## 项目结构

```
main.go                    命令行入口
internal/app               根模型：持有当前屏幕并分发消息
internal/config            运行配置
internal/core              领域层：路径解析、数据模型、扫描器、删除器（与界面无关）
internal/i18n              界面文案与语言切换
internal/version           版本号（构建日期）
internal/platform          与操作系统相关的文件操作
internal/logging           可选的文件日志
internal/ui                Screen 抽象与跨屏幕消息
internal/ui/theme          主色板与 lipgloss 样式
internal/ui/ascii          渐变着色、波形、指示器、擦除等动画
internal/ui/components     通用构件（盒子、进度条、帮助栏、文本裁剪）
internal/ui/screens/home   唯一的主屏幕
```

## 开发

- `internal/core` 不依赖任何 UI 代码；扫描与删除通过事件流与界面通信，均接收 `context.Context`（重新扫描会关闭旧任务）。
- 目录不可读时跳过；删除前清理只读属性；失败逐项记录。
- 界面在 80×24 至 200×60 之间不会溢出。
- **所有宽度计算统一使用 `charmbracelet/x/ansi`** —— 不要引入 `mattn/go-runewidth`：它在中文 Windows 上把 ambiguous 字符算作 2 列，而渲染按 1 列定位；行宽算错后会在 ANSI 序列中间截断。截断一律走 `ansi.Truncate`。

## 测试

```bash
go test ./...
```

设置 `UED_DUMP=<路径>` 后运行 `go test ./internal/app -run TestDumpHome` 可导出各状态的纯文本快照。
