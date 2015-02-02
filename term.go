
// +build linux darwin !windows

package cmd

import (
	"os/exec"
	"strconv"
	"strings"
	"fmt"
	"os"
)

var textPadding = 4

func RightJustifyText(text string) string {
	
	_, cols := size()

	col := cols - (len(text) + textPadding)

	fmt.Printf("\033[%dG", col)

	return text
}

func size()(rows,cols int) {
	cmd := exec.Command("stty", "size")

	cmd.Stdin = os.Stdin

	out, _ := cmd.Output()
	
	sz := string(out)
	size := strings.Split(sz," ")

	rows, _ = strconv.Atoi(strings.TrimSpace(size[0]))
	cols, _ = strconv.Atoi(strings.TrimSpace(size[1]))

	return rows, cols
}

/*
	cursor := []string{"/","-","\\","|"}
	go func() {
		for {
			for _,ch := range cursor {
				fmt.Print(ch)
				fmt.Printf("\033[%dG", col)
				time.Sleep(time.Millisecond*100)

			}

		}
	}()
*/
