// rbm：Windows 回收站分批管理工具。
// 带子命令（scan/parts/show/delete/shell）走命令行；不带任何参数直接运行则打开图形界面。
package main

import (
	"os"

	"rbm/internal/cli"
	"rbm/internal/gui"
)

func main() {
	if len(os.Args) > 1 {
		os.Exit(cli.Main(os.Args[1:]))
	}
	os.Exit(gui.Run())
}
