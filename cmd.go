/*
 This package is used to implement a "line oriented command interpreter", inspired by the python package with
 the same name http://docs.python.org/2/library/cmd.html

 Usage:

	 commander := &Cmd{...}
	 commander.Init()

         cmd := NewCommand(name,Option...)

	 commander.Add(cmd)

	 commander.CmdLoop()
*/
package cmd

import (
	"flag"
	"fmt"
	"github.com/gobs/args"
	"github.com/gobs/pretty"
	"github.com/peterh/liner"
	"io"
	"os"
	"os/exec"
	"path"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//
// This is used to describe a new command
//
type Command struct {
	// command name
	name string
	// command alias
	alias string
	// command description
	help string
	// the function to call to execute the command
	call func(*Command, string) bool
	// list of possible sub commands
	subCommands map[string]*Command
	flags       *flag.FlagSet

	cmdline *Cmd
}

type Option func(command *Command)

func SetCmdAlias(alias string) Option {
	return func(command *Command) {
		command.alias = alias
	}
}

func SetHelp(help string) Option {
	return func(command *Command) {
		command.help = help
	}
}

func SetFlag(flag string, value string, help string) Option {
	return func(command *Command) {
		command.flags.String(flag, value, help)
	}
}

func SetBoolFlag(flag string, value bool, help string) Option {
	return func(command *Command) {
		command.flags.Bool(flag, value, help)
	}
}

func SetCmd(cmd func(command *Command, line string) (stop bool)) Option {
	return func(command *Command) {
		command.call = cmd
	}
}

func NewCommand(name string, opts ...Option) *Command {

	command := &Command{
		name:        name,
		help:        "",
		call:        func(*Command, string) bool { return false },
		subCommands: make(map[string]*Command),
		flags:       flag.NewFlagSet(name, flag.ContinueOnError)}

	for _, opt := range opts {
		opt(command)
	}

	command.flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s -%s", command.alias, command.help+"\n")
		PrintDefaults(command.flags)
	}

	if len(command.alias) == 0 {
		command.alias = command.name
	}

	return command
}

//Prints the default values of all defined flags in the set.
func PrintDefaults(f *flag.FlagSet) {
	f.VisitAll(func(flag *flag.Flag) {
		if reflect.TypeOf(flag.Value).String() == "*flag.boolValue" {
			fmt.Println(fmt.Sprintf("-%s %s", flag.Name, flag.Usage))
		} else {
			fmt.Println(fmt.Sprintf("-%s=%s %s", flag.Name, flag.DefValue, flag.Usage))
		}
	})
}

func (command *Command) GetCmdline() *Cmd {
	return command.cmdline
}

func (command *Command) AddSubCommand(name string, opts ...Option) {

	subcommand := NewCommand(name, opts...)

	if len(subcommand.alias) > 0 && subcommand.alias != subcommand.name {
		command.subCommands[subcommand.alias] = subcommand
	} else {
		command.subCommands[subcommand.name] = subcommand
	}

	subcommand.flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s -%s", command.alias, subcommand.alias, subcommand.help+"\n")
		PrintDefaults(subcommand.flags)
	}

	if len(command.alias) == 0 {
		subcommand.alias = subcommand.name
	}
}

func (command *Command) GetFlag(name string) string {
	flag := command.flags.Lookup(name)

	if flag != nil {
		return flag.Value.String()
	}

	return ""
}

func (command *Command) GetBoolFlag(name string) bool {
	flag := command.flags.Lookup(name)

	if flag != nil {
		val, err := strconv.ParseBool(flag.Value.String())
		if err != nil {
			return false
		}

		return val
	}

	return false
}

func (command *Command) Usage() {
	command.flags.Usage()

	for _, subcommand := range command.subCommands {
		fmt.Println()
		subcommand.flags.Usage()
	}
}

//
// This the the "context" for the command interpreter
//
type Cmd struct {
	// the prompt string
	Prompt string

	// the history file
	HistoryFile string

	// this function is called before starting the command loop
	PreLoop func()

	// this function is called before terminating the command loop
	PostLoop func()

	// this function is called before executing the selected command
	PreCmd func(string)

	// this function is called after a command has been executed
	// return true to terminate the interpreter, false to continue
	PostCmd func(string, bool) bool

	// this function is called if the last typed command was an empty line
	EmptyLine func()

	// this function is called if the command line doesn't match any existing command
	// by default it displays an error message
	Default func(string)

	// this function is called to implement command completion.
	// it should return a list of words that match the input text
	Complete func(string, string, int, int) []string

	// if true, enable shell commands
	EnableShell bool

	// this is the list of available commands indexed by command name
	Commands map[string]*Command

	///////// private stuff /////////////

	readline *liner.State

	commandNames []string

	waitGroup          *sync.WaitGroup
	waitMax, waitCount int

	restartLoop bool
}

func (cmd *Cmd) readHistoryFile() {
	if len(cmd.HistoryFile) == 0 || cmd.readline == nil {
		// no history file
		return
	}

	filepath := cmd.HistoryFile // start with current directory
	if _, err := os.Stat(filepath); err == nil {

		if f, err := os.Open(filepath); err == nil {
			_, err := cmd.readline.ReadHistory(f)

			if err != nil {
				fmt.Println(err)
			}

			f.Close()
		}

		return
	} else {
		fmt.Println(err)
	}

	filepath = path.Join(os.Getenv("HOME"), filepath) // then check home directory
	if _, err := os.Stat(filepath); err == nil {

		if f, err := os.Open(filepath); err == nil {
			fmt.Println(err)
			cmd.readline.ReadHistory(f)
			f.Close()
		}
	}

	// update HistoryFile with home path
	cmd.HistoryFile = filepath
}

func (cmd *Cmd) writeHistoryFile() {
	if len(cmd.HistoryFile) == 0 || cmd.readline == nil {
		// no history file
		return
	}

	if f, err := os.Create(cmd.HistoryFile); err != nil {
		fmt.Print("Error writing history file: ", err)
	} else {
		cmd.readline.WriteHistory(f)
		f.Close()
	}

}

//
// Initialize the command interpreter context
//
func (cmd *Cmd) Init() {
	if cmd.PreLoop == nil {
		cmd.PreLoop = func() {}
	}
	if cmd.PostLoop == nil {
		cmd.PostLoop = func() { cmd.readline.Close() }
	}
	if cmd.PreCmd == nil {
		cmd.PreCmd = func(string) {}
	}
	if cmd.PostCmd == nil {
		cmd.PostCmd = func(line string, stop bool) bool { return stop }
	}
	if cmd.EmptyLine == nil {
		cmd.EmptyLine = func() {}
	}
	if cmd.Default == nil {
		cmd.Default = func(line string) { fmt.Printf("invalid command: %v\n", line) }
	}

	cmd.readline = liner.NewLiner()

	cmd.Commands = make(map[string]*Command)

	help := NewCommand("help",
		SetHelp(`list available commands`),
		SetCmd(cmd.Help))

	cmd.Add(help)
	//cmd.Add(Command{"echo", `echo input line`, cmd.Echo})
	//cmd.Add(Command{"go", `go cmd: asynchronous execution of cmd, or 'go [--start|--wait]'`, cmd.Go})
}

//
// Add a completer that matches on command names
//
func (cmd *Cmd) AddCommandCompleter() {
	cmd.commandNames = make([]string, 0, len(cmd.Commands))

	for n, _ := range cmd.Commands {
		cmd.commandNames = append(cmd.commandNames, n)
	}

	// sorting for Help()
	sort.Strings(cmd.commandNames)

	cmd.readline.SetCompleter(func(line string) (c []string) {
		for _, n := range cmd.commandNames {
			if strings.HasPrefix(n, strings.ToLower(line)) {
				c = append(c, n)
			}
		}
		return
	})
}

//
// execute shell command
//
func shellExec(command string) {
	args := args.GetArgs(command)
	if len(args) < 1 {
		fmt.Println("No command to exec")
	} else {
		var cmd *exec.Cmd

		if runtime.GOOS == "windows" {
			cmdArgs := []string{"cmd", "/C"}
			cmdArgs = append(cmdArgs, args...)
			cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
		} else {
			cmd = exec.Command(args[0])
			cmd.Args = args
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Println(err)
		}
	}
}

// Add a command to the command interpreter.
// Overrides a command with the same name, if there was one
//
func (cmd *Cmd) Add(command *Command) {
	if len(command.alias) > 0 && command.alias != command.name {
		cmd.Commands[command.alias] = command
	} else {
		cmd.Commands[command.name] = command
	}
}

//
// Default help command.
// It lists all available commands or it displays the help for the specified command
//
func (cmd *Cmd) Help(command *Command, line string) (stop bool) {
	fmt.Println("")

	if len(line) == 0 {
		fmt.Println("Available commands (use 'help <topic>'):")
		fmt.Println("================================================================")

		tp := pretty.NewTabPrinter(8)

		for _, c := range cmd.commandNames {
			tp.Print(c)
		}

		tp.Println()
	} else {

		args := strings.Split(line, " ")

		if len(args) > 1 {

			if c, ok := cmd.Commands[args[0]]; ok {

				cm, ok := c.subCommands[args[1]]
				if ok {
					if len(cm.help) > 0 {
						cm.Usage()
					} else {
						fmt.Println("No help for ", line)
					}
				} else {
					fmt.Println("unknown command")
				}
			}

		} else {

			c, ok := cmd.Commands[line]
			if ok {
				if len(c.help) > 0 {
					c.Usage()
				} else {
					fmt.Println("No help for ", line)
				}
			} else {
				fmt.Println("unknown command")
			}
		}
	}

	fmt.Println("")
	return
}

func (cmd *Cmd) Echo(line string) (stop bool) {
	fmt.Println(line)
	return
}

func (cmd *Cmd) Go(line string) (stop bool) {
	if strings.HasPrefix(line, "-") {
		// should be --start or --wait

		args := args.ParseArgs(line)

		if _, ok := args.Options["start"]; ok {
			cmd.waitGroup = new(sync.WaitGroup)
			cmd.waitCount = 0
			cmd.waitMax = 0

			if len(args.Arguments) > 0 {
				cmd.waitMax, _ = strconv.Atoi(args.Arguments[0])
			}

			return
		}

		if _, ok := args.Options["wait"]; ok {
			if cmd.waitGroup == nil {
				fmt.Println("nothing to wait on")
			} else {
				cmd.waitGroup.Wait()
				cmd.waitGroup = nil
			}

			return
		}
	}

	if strings.HasPrefix(line, "go ") {
		fmt.Println("Don't go go me!")
	} else {
		if cmd.waitGroup == nil {
			go cmd.OneCmd(line)
		} else {
			if cmd.waitMax > 0 {
				if cmd.waitCount >= cmd.waitMax {
					cmd.waitGroup.Wait()
					cmd.waitCount = 0
				}
			}

			cmd.waitCount++
			cmd.waitGroup.Add(1)

			go func() {
				defer cmd.waitGroup.Done()
				cmd.OneCmd(line)
			}()
		}
	}

	return
}

// Simple processing of quotes strings in command line args
// This just treats a quotes string as a indivisible piece
func processQuotes(line string) (args []string) {

	lastQuote := rune(0)

	f := func(ch rune) bool {
		switch {
		case ch == lastQuote:
			lastQuote = rune(0)
			return false
		case lastQuote != rune(0):
			return false
		case unicode.In(ch, unicode.Quotation_Mark):
			lastQuote = ch
			return false
		default:
			return unicode.IsSpace(ch)
		}
	}

	args = strings.FieldsFunc(line, f)

	for i, arg := range args {
		args[i] = strings.Replace(arg, "\"", "", -1)
	}

	return args
}

//
// This method executes one command
//
func (cmd *Cmd) OneCmd(line string) (stop bool) {

	if cmd.EnableShell && strings.HasPrefix(line, "!") {
		shellExec(line[1:])
		return
	}

	parts := strings.SplitN(line, " ", 2)
	cname := parts[0]

	command, ok := cmd.Commands[cname]

	if ok {
		var params string

		if len(parts) > 1 {
			splitLine := strings.SplitN(parts[1], " ", 2)
			subcmd := splitLine[0]

			subcommand, ok := command.subCommands[subcmd]
			if ok {
				if len(splitLine) > 2 {

					params = strings.TrimSpace(splitLine[1])
				}

				//args := strings.Split(line, " ")

				args := processQuotes(line)

				subcommand.flags.Parse(args[2:])

				subcommand.cmdline = cmd
				stop = subcommand.call(subcommand, params)

				subcommand.flags.VisitAll(func(flag *flag.Flag) {
					flag.Value.Set(flag.DefValue)
				})
				return
			}

			params = strings.TrimSpace(parts[1])
		}

		args := processQuotes(line)
		command.flags.Parse(args[1:])
		command.cmdline = cmd
		stop = command.call(command, params)

		command.flags.VisitAll(func(flag *flag.Flag) {
			flag.Value.Set(flag.DefValue)
		})

	} else {
		cmd.Default(line)
	}

	return
}

//
// This is the command interpreter entry point.
// It displays a prompt, waits for a command and executes it until the selected command returns true
//
func (cmd *Cmd) CmdLoop() {
	if len(cmd.Prompt) == 0 {
		cmd.Prompt = "> "
	}

	cmd.AddCommandCompleter()

	cmd.PreLoop()

	cmd.readHistoryFile()

	// loop until ReadLine returns nil (signalling EOF)
	for {
		result, err := cmd.readline.Prompt(cmd.Prompt)
		if err != nil {

			if err == io.EOF {
				break
			}

			fmt.Println(err)
			continue
		}

		line := strings.TrimSpace(result)
		if line == "" {
			cmd.EmptyLine()
			continue
		}

		cmd.readline.AppendHistory(result) // allow user to recall this line

		cmd.PreCmd(line)

		stop := cmd.OneCmd(line)
		stop = cmd.PostCmd(line, stop)

		if stop {
			break
		}
	}

	cmd.writeHistoryFile()

	cmd.PostLoop()
}

func (cmd *Cmd) SetRestartLoop(restart bool) {
	cmd.restartLoop = restart
}

func (cmd *Cmd) GetRestartLoop() bool {
	return cmd.restartLoop
}

func (cmd *Cmd) RestartLoop() {
	cmd.PreLoop = func() {}
	cmd.restartLoop = false
	cmd.CmdLoop()
}
