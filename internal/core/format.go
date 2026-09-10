// Package core 提供与界面无关的领域逻辑：扫描、聚合与删除。
package core

import (
	"fmt"
	"strconv"
	"strings"
)

// HumanSize 把字节数格式化为易读字符串（以 1024 为进制）。
func HumanSize(b int64) string {
	const unit = 1024
	if b < 0 {
		return "-" + HumanSize(-b)
	}
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	units := [...]string{"KB", "MB", "GB", "TB", "PB"}
	value := float64(b)
	idx := -1
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f %s", value, units[idx])
	case value >= 10:
		return fmt.Sprintf("%.1f %s", value, units[idx])
	default:
		return fmt.Sprintf("%.2f %s", value, units[idx])
	}
}

// HumanCount 为整数添加千分位分隔符。
func HumanCount(n int) string {
	if n < 0 {
		return "-" + HumanCount(-n)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var sb strings.Builder
	lead := len(s) % 3
	if lead == 0 {
		lead = 3
	}
	sb.WriteString(s[:lead])
	for i := lead; i < len(s); i += 3 {
		sb.WriteByte(',')
		sb.WriteString(s[i : i+3])
	}
	return sb.String()
}
