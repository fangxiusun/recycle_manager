// Package i18n 提供界面与命令行文案的双语目录：中文为默认，英文可切换。
//
// 只管「说给人看的话」。落盘的东西一律不过滤——state.json 的字段名与取值、
// scan_list.csv / ledger.csv 的表头、分区标签（「其他」「全部」「元数据损坏」「特殊位置」「(根)」）
// 和「已删」这类账本状态都保持中文，否则快照复现与黄金样例比对会随语言变。
package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Lang 是界面语言。
type Lang string

const (
	Zh Lang = "zh"
	En Lang = "en"
)

// EnvVar 是环境变量优先级来源；SettingsEnv 只给测试与高级用户用来改配置文件的落点。
const (
	EnvVar     = "RBM_LANG"
	SettingsEn = "RBM_SETTINGS_FILE"
	// SettingsName 是配置文件名，放在 %LOCALAPPDATA%\rbm 下。
	SettingsName = "settings.json"
	settingsDir  = "rbm"
	langField    = "language"
)

// SetEnv 把语言解析成一次实际生效的值，供启动时调用。
//
// 优先级：命令行 --lang > 环境变量 RBM_LANG > %LOCALAPPDATA%\rbm\settings.json > 中文。
// 认不出的写法逐级往下退，最后落到中文；不会报错打断使用。
func SetEnv(flagVal string) Lang {
	if l, ok := Parse(flagVal); ok {
		Set(l)
		return l
	}
	if l, ok := Parse(os.Getenv(EnvVar)); ok {
		Set(l)
		return l
	}
	if saved, err := ReadSettings(); err == nil {
		if l, ok := Parse(saved); ok {
			Set(l)
			return l
		}
	}
	Set(Zh)
	return Zh
}

// fmt 的动词字母与可选标志。只认这些，免得把 「100% 剩余」 这种写法当成格式动词。
const (
	verbRunes  = "bcdefgGqstTuvwxXo"
	flagRunes  = "+-# 0123456789.*"
	maxFlagLen = 12
)

// Verbs 按出现顺序返回一条文案里的格式动词（%% 视为转义，不计入）。
func Verbs(text string) []string {
	var out []string
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '%' {
			continue
		}
		if i+1 < len(rs) && rs[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(rs) && j-i <= maxFlagLen && strings.ContainsRune(flagRunes, rs[j]) {
			j++
		}
		if j >= len(rs) || !strings.ContainsRune(verbRunes, rs[j]) {
			continue
		}
		out = append(out, string(rs[i:j+1]))
		i = j
	}
	return out
}

// Parse 认 zh/en 及其常见地区写法（zh-CN、en-US、中文、English…）。
func Parse(text string) (Lang, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	switch t {
	case "":
		return "", false
	}
	switch {
	case strings.HasPrefix(t, "zh"), t == "cn", t == "chs", strings.Contains(t, "中文"):
		return Zh, true
	case strings.HasPrefix(t, "en"), t == "usa", strings.Contains(t, "english"):
		return En, true
	}
	return "", false
}

var (
	mu      sync.RWMutex
	current = Zh
)

// Set 切换全局语言；传认不出的值时保持原样。
func Set(l Lang) {
	if _, ok := Parse(string(l)); !ok {
		return
	}
	mu.Lock()
	current = l
	mu.Unlock()
}

// Get 返回当前语言。
func Get() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// IsEn 报告当前是否为英文，供宽度、排版这类「不只文案变」的判断用。
func IsEn() bool { return Get() == En }

// T 取一条文案。目录里没有的键会原样回显键名，这样漏翻译在界面上是可见的。
func T(key string) string {
	e, ok := catalog[key]
	if !ok {
		return key
	}
	if IsEn() {
		return e.en
	}
	return e.zh
}

// Tf 取一条带格式的文案。参数个数与动词对不上时，fmt 会把 %!d(MISSING) 这类标记打出来，
// 目录完整性测试已经保证两边动词一致，所以正常情况下不会走到那一步。
func Tf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

// Noun 给「数字后面那个名词」用：英文要分单复数（1 item / 3 items），中文不分，
// 量词由 zh 参数补上，这样中文输出仍然和改造前逐字一致。
//
// 目录里对应位置要写成一个 %s，两侧动词个数才保持一致；测试按个数校验。
func Noun(n int, zh, one, many string) string {
	if IsEn() {
		if n == 1 {
			return one
		}
		return many
	}
	return zh
}

// Keys 返回目录里全部键，排序后给出，便于测试与文档核对。
func Keys() []string {
	out := make([]string, 0, len(catalog))
	for k := range catalog {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SettingsPath 返回配置文件的落点：RBM_SETTINGS_FILE 优先，其次 %LOCALAPPDATA%\rbm，
// 再次 %APPDATA%\rbm，最后退回用户主目录下的 .rbm。
func SettingsPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(SettingsEn)); p != "" {
		return p, nil
	}
	for _, env := range []string{"LOCALAPPDATA", "APPDATA"} {
		if d := strings.TrimSpace(os.Getenv(env)); d != "" {
			return filepath.Join(d, settingsDir, SettingsName), nil
		}
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".rbm", SettingsName), nil
	}
	return "", errors.New("no writable config dir: LOCALAPPDATA and APPDATA are both empty")
}

// ReadSettings 读配置文件里的 language 字段；文件不存在返回空串而不算错误。
func ReadSettings() (string, error) {
	path, err := SettingsPath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	if v, ok := doc[langField].(string); ok {
		return v, nil
	}
	return "", nil
}

// WriteSettings 把语言写回配置文件，保留文件里其他字段；返回实际落点给日志用。
func WriteSettings(l Lang) (string, error) {
	path, err := SettingsPath()
	if err != nil {
		return "", err
	}
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	doc[langField] = string(l)
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}
