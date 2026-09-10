package core

import (
	"sort"
	"time"
)

// Install 表示某软件在某一个根位置下的安装目录。
type Install struct {
	Root    RootKind
	Path    string
	Size    int64
	Files   int
	Dirs    int
	Created time.Time // 目录创建时间
	ModTime time.Time
	Err     error // 统计过程中遇到的错误（不代表删除会失败）
}

// Software 表示一个软件：三个根位置下的同名目录合并后的聚合结果。
type Software struct {
	Name     string
	Installs []Install
	Selected bool
}

// TotalSize 返回该软件在全部根位置下的占用总和。
func (s Software) TotalSize() int64 {
	var total int64
	for _, in := range s.Installs {
		total += in.Size
	}
	return total
}

// FileCount 返回文件总数。
func (s Software) FileCount() int {
	var n int
	for _, in := range s.Installs {
		n += in.Files
	}
	return n
}

// DirCount 返回目录总数。
func (s Software) DirCount() int {
	var n int
	for _, in := range s.Installs {
		n += in.Dirs
	}
	return n
}

// Created 返回该软件最早的创建时间（三个位置中最早创建的那个目录）。
func (s Software) Created() time.Time {
	var earliest time.Time
	for _, in := range s.Installs {
		if in.Created.IsZero() {
			continue
		}
		if earliest.IsZero() || in.Created.Before(earliest) {
			earliest = in.Created
		}
	}
	return earliest
}

// ModTime 返回最近一次修改时间。
func (s Software) ModTime() time.Time {
	var latest time.Time
	for _, in := range s.Installs {
		if in.ModTime.After(latest) {
			latest = in.ModTime
		}
	}
	return latest
}

// Has 判断该软件在指定根位置是否存在数据。
func (s Software) Has(k RootKind) bool {
	_, ok := s.Install(k)
	return ok
}

// Install 返回指定根位置下的安装信息。
func (s Software) Install(k RootKind) (Install, bool) {
	for _, in := range s.Installs {
		if in.Root == k {
			return in, true
		}
	}
	return Install{}, false
}

// Clone 返回深拷贝，便于在 goroutine 之间安全传递快照。
func (s Software) Clone() Software {
	c := s
	if s.Installs != nil {
		c.Installs = append([]Install(nil), s.Installs...)
	}
	return c
}

// SortMode 表示列表的排序方式。
type SortMode int

const (
	// SortBySize 按占用从大到小。
	SortBySize SortMode = iota
	// SortByName 按名称。
	SortByName
	// SortByCreated 按创建日期（最新的在前）。
	SortByCreated
)

func (m SortMode) String() string {
	switch m {
	case SortByName:
		return "名称"
	case SortByCreated:
		return "日期"
	default:
		return "占用"
	}
}

// Next 切换到下一种排序方式。
func (m SortMode) Next() SortMode {
	return SortMode((int(m) + 1) % 3)
}

// Sort 原地排序软件列表。
func Sort(items []Software, mode SortMode) {
	switch mode {
	case SortByName:
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	case SortByCreated:
		sort.Slice(items, func(i, j int) bool {
			ci, cj := items[i].Created(), items[j].Created()
			if ci.IsZero() != cj.IsZero() {
				return cj.IsZero() // 未知日期排在最后
			}
			if !ci.Equal(cj) {
				return ci.After(cj)
			}
			return items[i].Name < items[j].Name
		})
	default:
		sort.Slice(items, func(i, j int) bool {
			si, sj := items[i].TotalSize(), items[j].TotalSize()
			if si != sj {
				return si > sj
			}
			return items[i].Name < items[j].Name
		})
	}
}

// TotalSize 汇总所有软件的占用。
func TotalSize(items []Software) int64 {
	var total int64
	for _, it := range items {
		total += it.TotalSize()
	}
	return total
}

// SelectedSize 汇总已选中软件的占用。
func SelectedSize(items []Software) int64 {
	var total int64
	for _, it := range items {
		if it.Selected {
			total += it.TotalSize()
		}
	}
	return total
}

// SelectedCount 返回已选中的软件数量。
func SelectedCount(items []Software) int {
	var n int
	for _, it := range items {
		if it.Selected {
			n++
		}
	}
	return n
}
