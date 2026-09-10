package core

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/unieditdept/ued-uninstaller/internal/platform"
)

// DeleteStarted 在开始删除时发出。
type DeleteStarted struct {
	Total  int
	DryRun bool
}

func (DeleteStarted) event() {}

// ItemDeleted 每处理完一个目录发出一次。
type ItemDeleted struct {
	Name  string
	Path  string
	Freed int64
	Done  int
	Total int
	Err   error
}

func (ItemDeleted) event() {}

// DeleteCompleted 在全部处理完毕后发出。
type DeleteCompleted struct{ Result DeleteResult }

func (DeleteCompleted) event() {}

// Failure 记录一次删除失败。
type Failure struct {
	Name string
	Path string
	Err  error
}

// DeleteResult 是删除任务的汇总结果。
type DeleteResult struct {
	Deleted  int           // 成功删除的目录数
	Freed    int64         // 释放的字节数
	Failures []Failure     // 失败明细
	Removed  []string      // 全部目录都删除成功的软件名
	Pruned   []string      // 被清理掉的空命名空间目录
	Elapsed  time.Duration // 总耗时
	DryRun   bool          // 是否为演练模式
}

// Remover 负责删除选中的软件目录。
type Remover struct {
	DryRun bool // 演练模式：只汇报，不真正删除
	Prune  bool // 删除后清理空的命名空间目录
}

// Stream 在后台执行删除，返回只读事件流。
func (r *Remover) Stream(ctx context.Context, items []Software, roots []Root) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		r.Run(ctx, items, roots, ch)
	}()
	return ch
}

// Run 执行删除，向 out 发送事件，并返回汇总结果。
func (r *Remover) Run(ctx context.Context, items []Software, roots []Root, out chan<- Event) DeleteResult {
	start := time.Now()
	result := DeleteResult{DryRun: r.DryRun}

	total := 0
	for i := range items {
		if items[i].Selected {
			total += len(items[i].Installs)
		}
	}
	if !emit(ctx, out, DeleteStarted{Total: total, DryRun: r.DryRun}) {
		return result
	}

	done := 0
	for i := range items {
		if !items[i].Selected {
			continue
		}
		deletedAll := true
		for _, inst := range items[i].Installs {
			select {
			case <-ctx.Done():
				result.Elapsed = time.Since(start)
				return result
			default:
			}

			var freed int64
			var err error
			if r.DryRun {
				freed = inst.Size
			} else {
				slog.Info("删除目录", "path", inst.Path)
				if err = platform.RemoveAll(inst.Path); err == nil {
					freed = inst.Size
				} else {
					slog.Warn("删除失败", "path", inst.Path, "err", err)
				}
			}
			done++

			if err != nil {
				deletedAll = false
				result.Failures = append(result.Failures, Failure{Name: items[i].Name, Path: inst.Path, Err: err})
			} else {
				result.Deleted++
				result.Freed += freed
			}
			emit(ctx, out, ItemDeleted{
				Name:  items[i].Name,
				Path:  inst.Path,
				Freed: freed,
				Done:  done,
				Total: total,
				Err:   err,
			})
		}
		if deletedAll {
			result.Removed = append(result.Removed, items[i].Name)
		}
	}

	if r.Prune && !r.DryRun {
		for _, root := range roots {
			if root.Dir == "" {
				continue
			}
			// 目录非空时 Remove 会失败，静默跳过即可。
			if err := os.Remove(root.Dir); err == nil {
				result.Pruned = append(result.Pruned, root.Dir)
			}
		}
	}

	result.Elapsed = time.Since(start)
	emit(ctx, out, DeleteCompleted{Result: result})
	return result
}
