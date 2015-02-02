package cmd

import (
	"fmt"
	"syscall"
)

var textPadding = 4

func RightJustifyText(text string) string {

	_, cols, c_row, _ := size()

	col := cols - (len(text) + textPadding)

	handle, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)

	err := setConsoleCursorPosition(handle, coord{x: int16(c_row), y: int16(col)})

	if err != nil {
		fmt.Println(err)
	}

	return text
}

func size() (rows, cols, c_row, c_col int) {
	handle, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)

	info, err := getConsoleScreenBufferInfo(handle)

	if err != nil {
		fmt.Println(err)
	}

	size := coord(info.size)
	cursorPos := coord(info.cursorPos)

	cols = int(size.x)
	rows = int(size.y)
	c_col = int(cursorPos.x)
	c_row = int(cursorPos.y)

	return
}
