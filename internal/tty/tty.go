// Package tty 负责控制台表格的宽度计算与排版，口径与 Python 版一致：
// 东亚宽字符（East_Asian_Width 为 W/F）按 2 列计。
package tty

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// Human 把字节数写成 84B / 158.5MB 这样的一列宽文本。
func Human(nbytes float64) string {
	n := nbytes
	for _, unit := range []string{"B", "KB", "MB", "GB", "TB"} {
		if n < 1024 || unit == "TB" {
			if unit == "B" {
				return fmt.Sprintf("%.0f%s", n, unit)
			}
			return fmt.Sprintf("%.1f%s", n, unit)
		}
		n /= 1024.0
	}
	return "?"
}

// widthCond 显式锁死两档开关：Python 只把 East_Asian_Width 为 W/F 的字符算 2 列，
// A（如 …、°）算 1 列；而 runewidth 默认会随系统 locale 变，不锁死的话同一份快照
// 在中文环境里会多 pad 一列，两边表格就对不齐。
var widthCond = runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

// WLen 返回文本在等宽控制台里占的列数。
// runewidth 对组合记号返回 0，而 Python 按字符计数得 1，这里补回 1 保持同口径。
func WLen(text string) int {
	total := 0
	for _, r := range text {
		total += runeWidth(r)
	}
	return total
}

func runeWidth(r rune) int {
	if w := widthCond.RuneWidth(r); w > 0 {
		return w
	}
	return 1
}

func Pad(text string, width int) string {
	n := width - WLen(text)
	if n <= 0 {
		return text
	}
	return text + strings.Repeat(" ", n)
}

// Truncate 按列数截断，超出一律以 … 结尾（… 占 1 列）。
func Truncate(text string, width int) string {
	if WLen(text) <= width {
		return text
	}
	var out []rune
	used := 0
	for _, r := range text {
		if used+runeWidth(r) > width-1 {
			break
		}
		out = append(out, r)
		used += runeWidth(r)
	}
	return string(out) + "…"
}

// Table 渲染表头、分隔线与数据行，列宽不足时截断。
func Table(headers []string, rows [][]string, widths []int) string {
	line := func(cells []string) string {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = c
			if i < len(widths) {
				parts[i] = Pad(Truncate(c, widths[i]), widths[i])
			}
		}
		return strings.Join(parts, "  ")
	}
	var sb strings.Builder
	sb.WriteString(line(headers))
	seps := make([]string, len(widths))
	for i, w := range widths {
		seps[i] = strings.Repeat("-", w)
	}
	sb.WriteByte('\n')
	sb.WriteString(strings.Join(seps, "  "))
	for _, row := range rows {
		sb.WriteByte('\n')
		sb.WriteString(line(row))
	}
	return sb.String()
}

var idRange = regexp.MustCompile(`^(\d+)(?:[-:](\d+))?$`)

// ParseIDs 把 "1-5,8,10-12" 解析成编号列表；写法不认识或越界返回 error。
// 分隔符允许逗号与任意空白，区间端点可反写，重复编号只保留一次。
func ParseIDs(spec string, upper int) ([]int, error) {
	var ids []int
	for _, token := range strings.FieldsFunc(strings.TrimSpace(spec), func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}) {
		m := idRange.FindStringSubmatch(token)
		if m == nil {
			return nil, fmt.Errorf("编号写法无法识别: %s（示例: 1-5,8-10,16）", token)
		}
		start, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("编号写法无法识别: %s（示例: 1-5,8-10,16）", token)
		}
		end := start
		if m[2] != "" {
			if end, err = strconv.Atoi(m[2]); err != nil {
				return nil, fmt.Errorf("编号写法无法识别: %s（示例: 1-5,8-10,16）", token)
			}
		}
		if end < start {
			start, end = end, start
		}
		if start < 1 || end > upper {
			return nil, fmt.Errorf("编号 %s 超出范围 1-%d", token, upper)
		}
		for i := start; i <= end; i++ {
			ids = append(ids, i)
		}
	}
	seen := make(map[int]bool, len(ids))
	out := make([]int, 0, len(ids))
	for _, i := range ids {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out, nil
}
