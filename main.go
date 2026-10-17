// rbm：Windows 回收站分批管理工具。
// 带子命令（scan/parts/show/delete/shell）走命令行；只剩语言选项或完全没有参数时打开图形界面。
package main

import (
	"os"

	"rbm/internal/cli"
	"rbm/internal/gui"
	"rbm/internal/i18n"
)

func main() {
	lang, rest := cli.SplitLangArgs(os.Args[1:])
	i18n.SetEnv(lang)
	if len(rest) == 0 {
		os.Exit(gui.Run())
	}
	os.Exit(cli.Main(rest))
}
