package main

import (
	"github.com/gobs/cmd"

	"fmt"
	"strings"
)

var (
	words = []string{"one", "two", "three", "four"}
)

func CompletionFunction(text string, line string, start, end int) []string {
	// for the "ls" command we let readline show real file names
	if strings.HasPrefix(line, "ls ") {
		return nil
	}

	// for all other commands, we pick from our list of completion words
	matches := make([]string, 0, len(words))

	for _, w := range words {
		if strings.HasPrefix(w, text) {
			matches = append(matches, w)
		}
	}

	return matches
}

func Exit(command *cmd.Command,line string) (stop bool) {
	fmt.Println("goodbye!")
	return true
}

func main() {
	commander := &cmd.Cmd{HistoryFile: ".rlhistory", Complete: CompletionFunction, EnableShell: true}
	commander.Init()

	text := fmt.Sprintf("%c[%dm%s\033[0m", 0x1B, 31,"red bold")
	cmd.ColorizeString(text)
	fmt.Println()

	list := cmd.NewCommand(
		"ls",
		cmd.SetHelp(`list stuff`),
		cmd.SetFlag("number","","only list this number of things"),
		cmd.SetCmd(func(command *cmd.Command,line string) (stop bool) {

			if num := command.GetFlag("number"); len(num) > 0 {
				fmt.Println("list only",num, "stuff")
			} else {
				fmt.Println("listing stuff")
			}
			return
		}))


	

	list.AddSubCommand("dir",
		cmd.SetHelp("list directories only"),
		cmd.SetFlag("h","","print human readable size of directory"),
		cmd.SetCmd(func(command* cmd.Command,line string) (stop bool) {
			fmt.Println("listing directories")
			return
		}))
	
	commander.Add(list)

	commander.Add(cmd.NewCommand(
		">",
		cmd.SetHelp(`Set prompt`),
		cmd.SetCmd(func(command *cmd.Command,line string) (stop bool) {
			commander.Prompt = line
			return
		})))

	commander.Add(cmd.NewCommand(
		"exit",
		cmd.SetHelp(`terminate example`),
		cmd.SetCmd(Exit)))

	commander.CmdLoop()
}
