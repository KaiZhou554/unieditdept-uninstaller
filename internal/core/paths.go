package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// DefaultNamespace 是软件数据存放的命名空间目录名。
const DefaultNamespace = "unieditdept"

// RootKind 标识一个扫描根位置。
type RootKind int

const (
	// RootAppData 对应 %APPDATA%（Roaming）。
	RootAppData RootKind = iota
	// RootLocalAppData 对应 %LOCALAPPDATA%（Local）。
	RootLocalAppData
	// RootTemp 对应 %TEMP%。
	RootTemp
)

// AllRootKinds 返回全部根位置的稳定顺序。
func AllRootKinds() []RootKind {
	return []RootKind{RootAppData, RootLocalAppData, RootTemp}
}

// String 返回环境变量风格的根名称。
func (k RootKind) String() string {
	switch k {
	case RootAppData:
		return "APPDATA"
	case RootLocalAppData:
		return "LOCALAPPDATA"
	case RootTemp:
		return "TEMP"
	default:
		return "UNKNOWN"
	}
}

// Short 返回紧凑显示用的短标签。
func (k RootKind) Short() string {
	switch k {
	case RootAppData:
		return "Roaming"
	case RootLocalAppData:
		return "Local"
	case RootTemp:
		return "Temp"
	default:
		return "?"
	}
}

// Tag 返回单字母标记，用于列表徽章。
func (k RootKind) Tag() string {
	switch k {
	case RootAppData:
		return "R"
	case RootLocalAppData:
		return "L"
	case RootTemp:
		return "T"
	default:
		return "?"
	}
}

// Root 是一个扫描根：基础目录与其下的命名空间目录。
type Root struct {
	Kind   RootKind
	Base   string // 基础目录，如 C:\Users\x\AppData\Roaming
	Dir    string // Base/namespace
	Exists bool   // 命名空间目录是否存在
	Err    error  // 解析或探测时发生的错误
}

// ErrUnresolved 表示无法定位基础目录。
var ErrUnresolved = errors.New("无法解析基础目录")

// ResolveRoots 解析全部扫描根，并探测命名空间目录是否存在。
func ResolveRoots(namespace string) []Root {
	kinds := AllRootKinds()
	roots := make([]Root, 0, len(kinds))
	for _, k := range kinds {
		r := Root{Kind: k, Base: BaseDir(k)}
		if r.Base == "" {
			r.Err = ErrUnresolved
			roots = append(roots, r)
			continue
		}
		r.Dir = filepath.Join(r.Base, namespace)
		switch st, err := os.Stat(r.Dir); {
		case err == nil:
			r.Exists = st.IsDir()
		case !os.IsNotExist(err):
			r.Err = err
		}
		roots = append(roots, r)
	}
	return roots
}

// BaseDir 返回指定根位置的基础目录：优先读取环境变量，其次按平台约定回退。
func BaseDir(k RootKind) string {
	switch k {
	case RootAppData:
		return firstNonEmpty(os.Getenv("APPDATA"), homeJoin("AppData", "Roaming"), safe(os.UserConfigDir))
	case RootLocalAppData:
		return firstNonEmpty(os.Getenv("LOCALAPPDATA"), homeJoin("AppData", "Local"), safe(os.UserCacheDir))
	case RootTemp:
		return firstNonEmpty(os.Getenv("TEMP"), os.Getenv("TMP"), os.TempDir())
	default:
		return ""
	}
}

func safe(fn func() (string, error)) string {
	if d, err := fn(); err == nil && d != "" {
		return d
	}
	return ""
}

func homeJoin(parts ...string) string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return ""
	}
	return filepath.Join(append([]string{h}, parts...)...)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
