package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeFile 在指定路径写入指定大小的文件。
func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

// setupRoots 把三个根位置指向临时目录。
func setupRoots(t *testing.T, namespace string) (appdata, local, temp string) {
	t.Helper()
	base := t.TempDir()
	appdata = filepath.Join(base, "roaming")
	local = filepath.Join(base, "local")
	temp = filepath.Join(base, "temp")
	t.Setenv("APPDATA", appdata)
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("TEMP", temp)
	return appdata, local, temp
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1024 * 1024, "1.00 MB"},
		{5 * 1024 * 1024 * 1024, "5.00 GB"},
	}
	for _, c := range cases {
		if got := HumanSize(c.in); got != c.want {
			t.Errorf("HumanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanCount(t *testing.T) {
	cases := map[int]string{0: "0", 12: "12", 999: "999", 1000: "1,000", 1234567: "1,234,567"}
	for in, want := range cases {
		if got := HumanCount(in); got != want {
			t.Errorf("HumanCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestScannerMergesSameNameAcrossRoots(t *testing.T) {
	appdata, local, temp := setupRoots(t, "unieditdept")

	writeFile(t, filepath.Join(appdata, "unieditdept", "Alpha", "a.bin"), 1000)
	writeFile(t, filepath.Join(local, "unieditdept", "Alpha", "b.bin"), 2000)
	writeFile(t, filepath.Join(temp, "unieditdept", "Beta", "c.bin"), 3000)
	writeFile(t, filepath.Join(temp, "unieditdept", "nope.txt"), 10) // 文件不应被视为软件

	sc := NewScanner("unieditdept")
	items := sc.Run(context.Background(), nil)
	Sort(items, SortBySize)

	if len(items) != 2 {
		t.Fatalf("期望 2 个软件，实际 %d", len(items))
	}
	if items[0].Name != "Alpha" {
		t.Fatalf("占用最大的应为 Alpha，实际 %s", items[0].Name)
	}
	if got := items[0].TotalSize(); got != 3000 {
		t.Errorf("Alpha 合并占用应为 3000，实际 %d", got)
	}
	if len(items[0].Installs) != 2 {
		t.Errorf("Alpha 应来自 2 个位置，实际 %d", len(items[0].Installs))
	}
	if !items[0].Has(RootAppData) || !items[0].Has(RootLocalAppData) || items[0].Has(RootTemp) {
		t.Error("Alpha 的分布位置不正确")
	}
	if items[0].FileCount() != 2 {
		t.Errorf("Alpha 文件数应为 2，实际 %d", items[0].FileCount())
	}
}

func TestScannerEmitsEvents(t *testing.T) {
	appdata, _, _ := setupRoots(t, "unieditdept")
	writeFile(t, filepath.Join(appdata, "unieditdept", "Solo", "a.bin"), 42)

	sc := NewScanner("unieditdept")
	events := sc.Stream(context.Background())

	var started, enumerated, measured, completed bool
	for ev := range events {
		switch e := ev.(type) {
		case ScanStarted:
			// 枚举阶段应立刻给出软件名，此时占用尚未统计。
			started = e.Total == 1 && len(e.Items) == 1 && e.Items[0].Name == "Solo"
			enumerated = len(e.Items) == 1 && e.Items[0].Installs[0].Size == 0
		case SoftwareMeasured:
			measured = e.Install.Size == 42 && e.Name == "Solo"
		case ScanCompleted:
			completed = len(e.Items) == 1 && e.Items[0].TotalSize() == 42
		}
	}
	if !started || !enumerated || !measured || !completed {
		t.Errorf("事件序列不完整: started=%v enumerated=%v measured=%v completed=%v",
			started, enumerated, measured, completed)
	}
}

func TestScannerReportsCreationTime(t *testing.T) {
	appdata, _, _ := setupRoots(t, "unieditdept")
	writeFile(t, filepath.Join(appdata, "unieditdept", "Solo", "a.bin"), 10)

	items := NewScanner("unieditdept").Run(context.Background(), nil)
	if len(items) != 1 {
		t.Fatalf("期望 1 个软件，实际 %d", len(items))
	}
	if items[0].Created().IsZero() {
		t.Error("应能取得目录创建时间")
	}
}

func TestScannerWithoutNamespace(t *testing.T) {
	setupRoots(t, "unieditdept")
	sc := NewScanner("unieditdept")
	if items := sc.Run(context.Background(), nil); len(items) != 0 {
		t.Errorf("期望没有软件，实际 %d", len(items))
	}
}

func TestRemoverDryRunKeepsFiles(t *testing.T) {
	appdata, local, _ := setupRoots(t, "unieditdept")
	dirA := filepath.Join(appdata, "unieditdept", "Alpha")
	dirB := filepath.Join(local, "unieditdept", "Alpha")
	writeFile(t, filepath.Join(dirA, "a.bin"), 1000)
	writeFile(t, filepath.Join(dirB, "b.bin"), 2000)

	items := []Software{{Name: "Alpha", Selected: true, Installs: []Install{
		{Root: RootAppData, Path: dirA, Size: 1000},
		{Root: RootLocalAppData, Path: dirB, Size: 2000},
	}}}

	r := &Remover{DryRun: true, Prune: true}
	res := r.Run(context.Background(), items, []Root{}, nil)

	if res.Deleted != 2 || res.Freed != 3000 {
		t.Errorf("演练结果不正确: %+v", res)
	}
	if _, err := os.Stat(dirA); err != nil {
		t.Error("演练模式不应删除文件")
	}
}

func TestRemoverDeletesAndPrunes(t *testing.T) {
	appdata, local, _ := setupRoots(t, "unieditdept")
	dirA := filepath.Join(appdata, "unieditdept", "Alpha")
	dirB := filepath.Join(local, "unieditdept", "Alpha")
	writeFile(t, filepath.Join(dirA, "a.bin"), 1000)
	writeFile(t, filepath.Join(dirB, "nested", "b.bin"), 2000)

	items := []Software{{Name: "Alpha", Selected: true, Installs: []Install{
		{Root: RootAppData, Path: dirA, Size: 1000},
		{Root: RootLocalAppData, Path: dirB, Size: 2000},
	}}}
	roots := ResolveRoots("unieditdept")

	r := &Remover{DryRun: false, Prune: true}
	res := r.Run(context.Background(), items, roots, nil)

	if res.Deleted != 2 || len(res.Failures) != 0 {
		t.Fatalf("删除结果不正确: %+v", res)
	}
	if _, err := os.Stat(dirA); !os.IsNotExist(err) {
		t.Error("目录应已被删除")
	}
	if len(res.Pruned) != 2 {
		t.Errorf("应清理 2 个空目录，实际 %d", len(res.Pruned))
	}
}

func TestRemoverSkipsUnselected(t *testing.T) {
	appdata, _, _ := setupRoots(t, "unieditdept")
	dir := filepath.Join(appdata, "unieditdept", "Keep")
	writeFile(t, filepath.Join(dir, "a.bin"), 10)

	items := []Software{{Name: "Keep", Selected: false, Installs: []Install{{Root: RootAppData, Path: dir, Size: 10}}}}
	r := &Remover{}
	res := r.Run(context.Background(), items, nil, nil)

	if res.Deleted != 0 {
		t.Error("未选中的软件不应被删除")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("未选中的目录应保留")
	}
}

func TestSortModes(t *testing.T) {
	items := []Software{
		{Name: "B", Installs: []Install{{Size: 10}}},
		{Name: "A", Installs: []Install{{Size: 30}}},
		{Name: "C", Installs: []Install{{Size: 20}}},
	}
	Sort(items, SortBySize)
	if items[0].Name != "A" || items[2].Name != "B" {
		t.Errorf("按占用排序错误: %+v", items)
	}
	Sort(items, SortByName)
	if items[0].Name != "A" || items[2].Name != "C" {
		t.Errorf("按名称排序错误: %+v", items)
	}
}
