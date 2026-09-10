// Package i18n 集中管理界面文案，支持在运行时切换语言。
package i18n

import "strings"

// Lang 是语言标识。
type Lang string

const (
	// ZH 简体中文。
	ZH Lang = "zh"
	// EN English。
	EN Lang = "en"
)

// Order 是语言切换的顺序，也决定右上角切换器的展示顺序。
var Order = []Lang{ZH, EN}

// Parse 解析语言标识，无法识别时返回默认语言。
func Parse(s string) Lang {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, l := range Order {
		if string(l) == s {
			return l
		}
	}
	switch s {
	case "cn", "zh-cn", "zh_hans", "chinese":
		return ZH
	case "us", "en-us", "english":
		return EN
	}
	return Order[0]
}

// Next 返回下一种语言。
func (l Lang) Next() Lang {
	for i, v := range Order {
		if v == l {
			return Order[(i+1)%len(Order)]
		}
	}
	return Order[0]
}

// Label 返回该语言在切换器中的显示名，始终用其母语书写。
func (l Lang) Label() string {
	if s, ok := labels[l]; ok {
		return s
	}
	return labels[Order[0]]
}

var labels = map[Lang]string{
	ZH: "简体中文",
	EN: "English",
}

// Strings 是一整套界面文案。含动态数值的条目都是 fmt 模板。
type Strings struct {
	// 顶部
	AppTitle string

	// 列表
	ListTitle    string // %s = 数量
	ListFiltered string // %s = 数量，%s = 关键词
	ColName      string
	ColSize      string
	ColCreated   string
	Calculating  string

	// 空状态
	Loading    string
	NoApps     string
	AllRemoved string
	NoMatch    string // %s = 关键词

	// 任务面板标题
	TaskScanning string
	TaskConfirm  string
	TaskRemoving string
	TaskResult   string
	TaskIdle     string

	// 任务面板内容
	ScanningUsage  string
	FolderProgress string // %s / %s
	FolderMeasured string // %s
	ConfirmPending string
	AppCount       string // %s
	WillFree       string // %s
	AutoCancelIn   string // %s
	Removing       string
	DryRunning     string
	Freed          string // %s
	RemovedDirs    string // %s
	Elapsed        string // %s
	PrunedDirs     string // %s
	FailedCount    string // %s
	MoreItems      string // %s
	PressToRemove  string

	// 结果标题
	ResultDone       string
	ResultDoneDry    string
	ResultWithErrors string

	// 临时提示
	Canceled       string
	CanceledByTime string
	ScanFailed     string // %s

	// 快捷键说明
	KeyMove          string
	KeySelect        string
	KeySelectAll     string
	KeyUninstall     string
	KeySearch        string
	KeySort          string // %s = 排序名
	KeyInvert        string
	KeyRescan        string
	KeyHelp          string
	KeyQuit          string
	KeyConfirmFilter string
	KeyClear         string
	KeyAbort         string
	KeyConfirmN      string // %s = 数量
	KeyCancel        string
	KeyAnyKey        string

	// 排序名
	SortSize string
	SortName string
	SortDate string

	// 帮助页
	HelpCreatedNote string
	HelpCancelNote  string
	HelpEntries     [][2]string
}

// Get 返回指定语言的文案。
func Get(l Lang) *Strings {
	if s, ok := bundles[l]; ok {
		return s
	}
	return bundles[Order[0]]
}

var bundles = map[Lang]*Strings{ZH: zhStrings, EN: enStrings}
