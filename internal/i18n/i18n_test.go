package i18n

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

func resetLang(t *testing.T) {
	t.Helper()
	before := Get()
	t.Cleanup(func() { Set(before) })
}

// 目录完整性：每个键两侧都有文案、两侧不同、格式动词一一对应。
// 动词对不上是这类改造最容易漏的错——翻译把两个 %d 的顺序换了，调用方的参数就得跟着换，
// 而这在界面上表现为「数字串位」，只在切到英文时才看得见。
func TestCatalogComplete(t *testing.T) {
	keys := Keys()
	if len(keys) == 0 {
		t.Fatal("目录是空的")
	}
	for _, k := range keys {
		e, ok := catalog[k]
		if !ok {
			t.Fatalf("键 %s 不在目录里", k)
		}
		if strings.TrimSpace(e.zh) == "" || strings.TrimSpace(e.en) == "" {
			t.Errorf("%s 有一侧是空白: zh=%q en=%q", k, e.zh, e.en)
		}
		if e.zh == e.en {
			t.Errorf("%s 两侧文字相同，等于没翻译", k)
		}
		zv, ev := Verbs(e.zh), Verbs(e.en)
		sort.Strings(zv)
		sort.Strings(ev)
		if strings.Join(zv, " ") != strings.Join(ev, " ") {
			t.Errorf("%s 的格式动词不一致: zh %v / en %v", k, zv, ev)
		}
	}
}

// 带参数的键必须两侧都能被 Sprintf 吃完，不能出现 MISSING/BAD。
func TestCatalogTfVerbsConsumed(t *testing.T) {
	for _, k := range Keys() {
		e := catalog[k]
		vs := Verbs(e.zh)
		if len(vs) == 0 {
			continue
		}
		args := make([]any, len(vs))
		for i, v := range vs {
			args[i] = sampleArg(v)
		}
		Set(En)
		en := Tf(k, args...)
		Set(Zh)
		zh := Tf(k, args...)
		for _, s := range []string{zh, en} {
			if strings.Contains(s, "MISSING") || strings.Contains(s, "BAD ") {
				t.Errorf("%s 参数没被吃完: %q", k, s)
			}
		}
		if !utf8.ValidString(zh) || !utf8.ValidString(en) {
			t.Errorf("%s 渲染出非法字节", k)
		}
	}
}

func sampleArg(verb string) any {
	last := verb[len(verb)-1:]
	switch last {
	case "d", "i", "x", "b":
		return 7
	case "f", "e":
		return 1.5
	case "w":
		return errors.New("boom")
	default:
		return "sample"
	}
}

func TestSetGet(t *testing.T) {
	resetLang(t)
	Set(Zh)
	if Get() != Zh || IsEn() {
		t.Errorf("默认应为中文，实得 %q", Get())
	}
	Set(En)
	if Get() != En || !IsEn() {
		t.Errorf("切英文后实得 %q", Get())
	}
	Set("fr")
	if Get() != En {
		t.Errorf("认不出的值不该改语言，实得 %q", Get())
	}
}

func TestTUnknownKey(t *testing.T) {
	resetLang(t)
	Set(En)
	if got := T("nope.not-a-key"); got != "nope.not-a-key" {
		t.Errorf("缺键应原样回显，实得 %q", got)
	}
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Lang
		ok   bool
	}{
		{"zh", Zh, true}, {"en", En, true}, {"EN", En, true}, {" en ", En, true},
		{"zh-CN", Zh, true}, {"en-US", En, true}, {"中文", Zh, true}, {"English", En, true},
		{"", "", false}, {"fr", "", false},
	} {
		got, ok := Parse(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Parse(%q) = %q,%v 期望 %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// 优先级：--lang > RBM_LANG > 配置文件 > 中文。
func TestSetEnvPrecedence(t *testing.T) {
	resetLang(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "settings.json")
	t.Setenv(SettingsEn, file)
	t.Setenv(EnvVar, "")

	if got := SetEnv(""); got != Zh {
		t.Errorf("什么都没给时应当中文，实得 %q", got)
	}
	if _, err := WriteSettings(En); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	if got := SetEnv(""); got != En {
		t.Errorf("配置文件里的英文没有生效，实得 %q", got)
	}
	t.Setenv(EnvVar, "zh")
	if got := SetEnv(""); got != Zh {
		t.Errorf("环境变量应当压过配置文件，实得 %q", got)
	}
	if got := SetEnv("en"); got != En {
		t.Errorf("--lang 应当压过环境变量，实得 %q", got)
	}
	t.Setenv(EnvVar, "乱写")
	if got := SetEnv(""); got != En {
		t.Errorf("环境变量认不出时应当回落到配置文件，实得 %q", got)
	}
	t.Setenv(SettingsEn, filepath.Join(dir, "nope.json"))
	Set(Zh)
	if got := SetEnv(""); got != Zh {
		t.Errorf("配置文件不存在时应当中文，实得 %q", got)
	}
}

func TestSettingsRoundTripKeepsOtherFields(t *testing.T) {
	resetLang(t)
	file := filepath.Join(t.TempDir(), "sub", "settings.json")
	t.Setenv(SettingsEn, file)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := WriteSettings(En)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if path != file {
		t.Errorf("回显路径不符: %s", path)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("配置不是合法 JSON: %v", err)
	}
	if doc["theme"] != "dark" {
		t.Errorf("其他字段被丢了: %v", doc)
	}
	if doc["language"] != "en" {
		t.Errorf("language = %v，期望 en", doc["language"])
	}
	if v, err := ReadSettings(); err != nil || v != "en" {
		t.Errorf("ReadSettings = %q,%v", v, err)
	}
}

func TestReadSettingsMissingFile(t *testing.T) {
	t.Setenv(SettingsEn, filepath.Join(t.TempDir(), "absent.json"))
	if v, err := ReadSettings(); err != nil || v != "" {
		t.Errorf("文件不存在应当回空串不报错，实得 %q,%v", v, err)
	}
}

func TestSettingsPathFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(SettingsEn, "")
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "Local"))
	t.Setenv("APPDATA", filepath.Join(dir, "Roaming"))
	p, err := SettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Local", "rbm", SettingsName); p != want {
		t.Errorf("LOCALAPPDATA 优先，实得 %s 期望 %s", p, want)
	}
	t.Setenv("LOCALAPPDATA", "   ")
	if p, err := SettingsPath(); err != nil || p != filepath.Join(dir, "Roaming", "rbm", SettingsName) {
		t.Errorf("LOCALAPPDATA 为空时应当退回 APPDATA，实得 %s,%v", p, err)
	}
	t.Setenv(SettingsEn, filepath.Join(dir, "explicit.json"))
	if p, err := SettingsPath(); err != nil || p != filepath.Join(dir, "explicit.json") {
		t.Errorf("RBM_SETTINGS_FILE 应当最优先，实得 %s,%v", p, err)
	}
}

func TestVerbsEscapedPercent(t *testing.T) {
	if got := Verbs("用了 50%% 的空间，共 %d 个"); len(got) != 1 || got[0] != "%d" {
		t.Errorf("%% 是转义，不该算动词，实得 %v", got)
	}
	if got := Verbs("%s 删了 %d 个"); len(got) != 2 {
		t.Errorf("动词漏了: %v", got)
	}
}
