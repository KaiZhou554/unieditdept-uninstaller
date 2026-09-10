package core

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unieditdept/ued-uninstaller/internal/platform"
)

// Event 是扫描 / 删除过程中向外发出的事件。
type Event interface{ event() }

// ScanStarted 在枚举完成后立刻发出，携带完整的软件列表（此时占用尚未统计）。
type ScanStarted struct {
	Total int
	Items []Software
	Roots []Root
}

func (ScanStarted) event() {}

// SoftwareMeasured 每当一个目录统计完成就发出一次。
type SoftwareMeasured struct {
	Name    string
	Install Install
	Done    int
	Total   int
}

func (SoftwareMeasured) event() {}

// ScanCompleted 在全部统计结束后发出，携带最终结果（含占用）。
type ScanCompleted struct {
	Items   []Software
	Roots   []Root
	Elapsed time.Duration
}

func (ScanCompleted) event() {}

// ScanError 表示一次非致命的读取错误。
type ScanError struct{ Err error }

func (ScanError) event() {}

// Scanner 扫描三个根位置下的命名空间目录，并把同名目录聚合为软件。
//
// 扫描分两步：先快速枚举出全部软件（几乎无耗时），再在后台并发统计占用，
// 每统计完一个目录就推送一次事件，界面可以边显示边刷新。
type Scanner struct {
	Roots       []Root
	Concurrency int
}

// NewScanner 创建扫描器，会自动解析命名空间对应的根位置。
func NewScanner(namespace string) *Scanner {
	c := runtime.NumCPU()
	if c < 2 {
		c = 2
	}
	if c > 16 {
		c = 16
	}
	return &Scanner{Roots: ResolveRoots(namespace), Concurrency: c}
}

// Stream 在后台执行扫描，返回只读事件流。扫描结束后通道会被关闭。
func (s *Scanner) Stream(ctx context.Context) <-chan Event {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		s.Run(ctx, ch)
	}()
	return ch
}

type scanJob struct {
	name string
	inst Install
}

// Run 执行扫描，向 out 发送事件，并返回最终结果。
func (s *Scanner) Run(ctx context.Context, out chan<- Event) []Software {
	start := time.Now()

	var jobs []scanJob
	for _, r := range s.Roots {
		if r.Err != nil || !r.Exists {
			continue
		}
		entries, err := os.ReadDir(r.Dir)
		if err != nil {
			slog.Warn("读取命名空间目录失败", "dir", r.Dir, "err", err)
			emit(ctx, out, ScanError{Err: fmt.Errorf("读取 %s 失败：%w", r.Dir, err)})
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue // 只有子目录才被视为一个软件
			}
			path := filepath.Join(r.Dir, e.Name())
			inst := Install{Root: r.Kind, Path: path, Created: platform.CreationTime(path)}
			if info, err := e.Info(); err == nil {
				inst.ModTime = info.ModTime()
			}
			jobs = append(jobs, scanJob{name: e.Name(), inst: inst})
		}
	}

	// 先把枚举结果（不含占用）建好并立刻推送，界面可以马上列出软件。
	order := make([]string, 0, len(jobs))
	groups := make(map[string]*Software, len(jobs))
	for i := range jobs {
		sw := groups[jobs[i].name]
		if sw == nil {
			sw = &Software{Name: jobs[i].name}
			groups[jobs[i].name] = sw
			order = append(order, jobs[i].name)
		}
		sw.Installs = append(sw.Installs, jobs[i].inst)
	}
	for _, sw := range groups {
		sort.SliceStable(sw.Installs, func(a, b int) bool {
			return sw.Installs[a].Root < sw.Installs[b].Root
		})
	}

	initial := make([]Software, 0, len(order))
	for _, name := range order {
		initial = append(initial, groups[name].Clone())
	}
	Sort(initial, SortByName)

	total := len(jobs)
	if !emit(ctx, out, ScanStarted{Total: total, Items: initial, Roots: s.Roots}) {
		return initial
	}
	if total == 0 {
		emit(ctx, out, ScanCompleted{Items: []Software{}, Roots: s.Roots, Elapsed: time.Since(start)})
		return initial
	}

	var (
		doneCount int64
		mu        sync.Mutex
		wg        sync.WaitGroup
		sem       = make(chan struct{}, s.Concurrency)
	)

jobs:
	for i := range jobs {
		select {
		case <-ctx.Done():
			break jobs
		default:
		}
		job := jobs[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			metrics, err := measure(job.inst.Path)
			inst := job.inst
			inst.Size = metrics.size
			inst.Files = metrics.files
			inst.Dirs = metrics.dirs
			inst.Err = err
			if inst.ModTime.IsZero() || metrics.mod.After(inst.ModTime) {
				inst.ModTime = metrics.mod
			}

			mu.Lock()
			if sw := groups[job.name]; sw != nil {
				for k := range sw.Installs {
					if sw.Installs[k].Root == inst.Root {
						sw.Installs[k] = inst
						break
					}
				}
			}
			mu.Unlock()

			done := int(atomic.AddInt64(&doneCount, 1))
			emit(ctx, out, SoftwareMeasured{Name: job.name, Install: inst, Done: done, Total: total})
		}()
	}
	wg.Wait()

	items := make([]Software, 0, len(groups))
	for _, sw := range groups {
		items = append(items, sw.Clone())
	}
	Sort(items, SortBySize)
	emit(ctx, out, ScanCompleted{Items: items, Roots: s.Roots, Elapsed: time.Since(start)})
	return items
}

// emit 发送事件；上下文取消时放弃发送并返回 false。out 为 nil 时静默丢弃。
func emit(ctx context.Context, out chan<- Event, ev Event) bool {
	if out == nil {
		return true
	}
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// metrics 是一次目录测量的结果。
type metrics struct {
	size  int64
	files int
	dirs  int
	mod   time.Time
}

// measure 递归统计目录占用。不跟随符号链接，单个子路径不可读时跳过并继续。
func measure(root string) (metrics, error) {
	var m metrics
	if info, err := os.Lstat(root); err == nil {
		m.mod = info.ModTime()
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if d.IsDir() {
			if path != root {
				m.dirs++
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // 跳过符号链接、设备、套接字等
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		m.files++
		m.size += info.Size()
		if t := info.ModTime(); t.After(m.mod) {
			m.mod = t
		}
		return nil
	})
	if err != nil {
		return m, err
	}
	return m, nil
}
