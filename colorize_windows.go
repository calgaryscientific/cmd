package cmd

import (
	"syscall"
	"unsafe"
	"fmt"
	"strconv"
	"bytes"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetStdHandle               = kernel32.NewProc("GetStdHandle")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleTextAttribute    = kernel32.NewProc("SetConsoleTextAttribute")

	// ANSI to Windows color codes
	w_BLACK     = 0
	w_BLUE      = 1
	w_GREEN     = 2
	w_RED       = 4
	w_INTENSITY = 8
	w_CYAN      = w_BLUE | w_GREEN
	w_MAGENTA   = w_BLUE | w_RED
	w_YELLOW    = w_GREEN | w_RED
	w_WHITE     = w_BLUE | w_GREEN | w_RED
	ansi2WIN    = []int{w_BLACK, w_RED, w_GREEN, w_YELLOW, w_BLUE, w_MAGENTA, w_CYAN, w_WHITE}
	
	ansiRESET   = 0
	ansiBOLD    = 1
	ansiBLACK   = 30
	ansiRED     = 31
	ansiGREEN   = 32
	ansiYELLOW  = 33
	ansiBLUE    = 34
	ansiMAGENTA = 35
	ansiCYAN    = 36
	ansiGRAY    = 37
	ansiWHITE   = 37

)

type coord struct {
	x int16
	y int16
}

type smallRect struct {
	left   int16
	top    int16
	right  int16
	bottom int16
}

type consoleScreenBuffer struct {
	size       coord
	cursorPos  coord
	attrs      int32
	window     smallRect
	maxWinSize coord
}

func getConsoleScreenBufferInfo(hCon syscall.Handle) (sb consoleScreenBuffer, err error) {

	rc, _, ec := syscall.Syscall(procGetConsoleScreenBufferInfo.Addr(), 2,
		uintptr(hCon), uintptr(unsafe.Pointer(&sb)), 0)
	if rc == 0 {
		err = syscall.Errno(ec)
	}
	return
}

func setConsoleTextAttribute(hCon syscall.Handle, color int) (err error) {
	rc, _, ec := syscall.Syscall(procSetConsoleTextAttribute.Addr(), 2,
		uintptr(hCon), uintptr(color), 0)
	if rc == 0 {
		err = syscall.Errno(ec)
	}
	return

}


func ColorizeString(text string) {

	handle, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	
	info, err := getConsoleScreenBufferInfo(handle)

	if err != nil {
		fmt.Println(err)
	}

	initialColor := int(info.attrs)

	charArray := bytes.Trim([]byte(text),"\x00")
	
	for i := 0; i < len(charArray); i++  {

		c := charArray[i]

		if c == '\033' || c == 0x1B {
			i++
			c = charArray[i]
       
			if c == '[' {
				i++
				c = charArray[i]

				ansiNumber := make([]byte,0)
				if  charArray[i+1] != 'm' {
					for j := 0; j < 2 && c != 'm'; j++  {
						ansiNumber = append(ansiNumber,c)
						i++
						c = charArray[i]
					}
				} else {
					ansiNumber = append(ansiNumber,c)
					i++
					c = charArray[i]
				}
	 
				ansiColor, _ := strconv.Atoi(string(ansiNumber));
				var winIntensity int
				var winColor int
				
				// Convert ANSI Color to Windows Color
				if (ansiColor == ansiBOLD) {
					winIntensity = w_INTENSITY;
				} else if (ansiColor == ansiRESET) {
					winIntensity = w_BLACK;
					winColor = initialColor;
				} else if (ansiBLACK <= ansiColor && ansiColor <= ansiWHITE) {
					winColor = ansi2WIN[ansiColor - 30];
					winIntensity = w_BLACK;
				} else if (ansiColor == 90) {
					// Special case for gray (it's really white)
					winColor = w_WHITE;
					winIntensity = w_BLACK;
				}
       
				// initialColor & 0xF0 is to keep the background color
				err = setConsoleTextAttribute(handle,winColor | winIntensity | (initialColor & 0xF0))
				if err != nil {
					fmt.Println(err)
				}
			}
		} else {

			fmt.Print(string(c))
		}
	}

	setConsoleTextAttribute(handle,initialColor)

}

