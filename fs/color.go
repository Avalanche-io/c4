package fs

import (
	"github.com/fatih/color"
)

type ColorFunc func(...interface{}) string

var (
	Bold    ColorFunc
	Red     ColorFunc
	Yellow  ColorFunc
	Green   ColorFunc
	Blue    ColorFunc
	Magenta ColorFunc
	Cyan    ColorFunc
	White   ColorFunc
)

func init() {
	Bold = color.New(color.Bold).SprintFunc()
	Red = color.New(color.FgRed).SprintFunc()
	Yellow = color.New(color.FgYellow).SprintFunc()
	Green = color.New(color.FgGreen).SprintFunc()
	Blue = color.New(color.FgBlue).SprintFunc()
	Magenta = color.New(color.FgMagenta).SprintFunc()
	Cyan = color.New(color.FgCyan).SprintFunc()
	White = color.New(color.FgWhite).SprintFunc()

}
