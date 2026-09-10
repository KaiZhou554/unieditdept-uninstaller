package i18n

import (
	"reflect"
	"strings"
	"testing"
)

// TestBundlesComplete 校验每种语言的每个文案字段都已填写。
// 反射遍历可以保证以后新增字段时不会漏翻译。
func TestBundlesComplete(t *testing.T) {
	for _, lang := range Order {
		s := Get(lang)
		if s == nil {
			t.Fatalf("%s 缺少文案", lang)
		}
		v := reflect.ValueOf(s).Elem()
		typ := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			name := typ.Field(i).Name
			switch field.Kind() {
			case reflect.String:
				if strings.TrimSpace(field.String()) == "" {
					t.Errorf("%s：字段 %s 未翻译", lang, name)
				}
			case reflect.Slice:
				if field.Len() == 0 {
					t.Errorf("%s：字段 %s 为空", lang, name)
				}
			}
		}
	}
}

// TestHelpEntriesAligned 校验各语言的帮助条目数量一致。
func TestHelpEntriesAligned(t *testing.T) {
	want := len(Get(ZH).HelpEntries)
	for _, lang := range Order {
		if got := len(Get(lang).HelpEntries); got != want {
			t.Errorf("%s 的帮助条目数为 %d，期望 %d", lang, got, want)
		}
	}
	for _, lang := range Order {
		for i, e := range Get(lang).HelpEntries {
			if strings.TrimSpace(e[0]) == "" || strings.TrimSpace(e[1]) == "" {
				t.Errorf("%s 的第 %d 条帮助为空", lang, i)
			}
		}
	}
}

func TestParse(t *testing.T) {
	cases := map[string]Lang{
		"zh":       ZH,
		"ZH":       ZH,
		" zh ":     ZH,
		"cn":       ZH,
		"zh-CN":    ZH,
		"en":       EN,
		"EN":       EN,
		"English":  EN,
		"":         Order[0],
		"whatever": Order[0],
	}
	for in, want := range cases {
		if got := Parse(in); got != want {
			t.Errorf("Parse(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestNextCycles(t *testing.T) {
	seen := map[Lang]bool{}
	l := Order[0]
	for i := 0; i < len(Order); i++ {
		if seen[l] {
			t.Fatalf("Next 出现重复：%s", l)
		}
		seen[l] = true
		l = l.Next()
	}
	if l != Order[0] {
		t.Errorf("Next 应当回到起点，实际 %s", l)
	}
}

func TestLabelIsNative(t *testing.T) {
	if got := ZH.Label(); got != "简体中文" {
		t.Errorf("ZH.Label() = %q", got)
	}
	if got := EN.Label(); got != "English" {
		t.Errorf("EN.Label() = %q", got)
	}
}
