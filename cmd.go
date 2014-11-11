/*
 This package is used to implement a "line oriented command interpreter", inspired by the python package with
 the same name http://docs.python.org/2/library/cmd.html

 Usage:

	 commander := &Cmd{...}
	 commander.Init()

         cmd := NewCommand(name,option...)

	 commander.Add(cmd)

	 commander.CmdLoop()
*/
package cmd

import (
	"github.com/calgaryscientific/args"
	"github.com/calgaryscientific/pretty"
	"github.com/calgaryscientific/readline"

	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"runtime"
	"flag"
)


//
// This is used to describe a new command
//
type Command struct {
	// command name
	name string
	// command description
	help string
	// the function to call to execute the command
	call func(*Command, string) bool
	// list of possible sub commands
	subCommands map[string]*Command
	flags *flag.FlagSet

}

type option func(command *Command)

func SetHelp(help string) option {
	return func(command* Command) {
		command.help = help
	}
}

func SetFlag(flag string, value string, help string) option {
	return func(command *Command) {
		command.flags.String(flag,value,help)
	}
}

func SetCmd(cmd func(command *Command, line string)(stop bool)) option {
	return func(command* Command) {
		command.call = cmd
	}
}

func NewCommand(name string, opts ...option) (*Command){

	command := &Command{
		name: name,
		help: "",
		call:func(*Command, string) bool { return false },
		subCommands: make(map[string]*Command),
		flags: flag.NewFlagSet(name,flag.ContinueOnError) }

	
	for _, opt := range opts {
		opt(command)
	}

	command.flags.Usage = func() {
		fmt.Fprintf(os.Stderr,"%s -%s", command.name, command.help + "\n")
		command.flags.PrintDefaults()
	}

	
	return command
}

func (command *Command) AddSubCommand(name string, opts ...option) {

	subcommand := NewCommand(name,opts...)
	command.subCommands[name] = subcommand
	
	subcommand.flags.Usage = func() {
		fmt.Fprintf(os.Stderr,"%s %s -%s", command.name, subcommand.name, subcommand.help + "\n")
		subcommand.flags.PrintDefaults()
	}

}

func (command* Command) GetFlag(name string)(string) {
	flag := command.flags.Lookup(name)

	if flag != nil {
		return flag.Value.String()
	}

	return ""
}

func (command *Command) Usage() {
	command.flags.Usage()

	for _, subcommand := range command.subCommands {
		fmt.Println()
		subcommand.flags.Usage()
	}
}


//
// The context for command completion
//
type Completer struct {
	// the list of words to match on
	Words []string
	// the list of current matches
	Matches []string
}

//
// Return a word matching the prefix
// If there are multiple matches, index selects which one to pick
//
func (c *Completer) Complete(prefix string, index int) string {
	if index == 0 {
		c.Matches = c.Matches[:0]

		for _, w := range c.Words {
			if strings.HasPrefix(w, prefix) {
				c.Matches = append(c.Matches, w)
			}
		}
	}

	if index < len(c.Matches) {
		return c.Matches[index]
	} else {
		return ""
	}
}

//
// Create a Completer and initialize with list of words
//
func NewCompleter(words []string) (c *Completer) {
	c = new(Completer)
	c.Words = words
	c.Matches = make([]string, 0, len(c.Words))
	return
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
	completer    *Completer
	commandNames []string

	waitGroup          *sync.WaitGroup
	waitMax, waitCount int
}

func (cmd *Cmd) readHistoryFile() {
	if len(cmd.HistoryFile) == 0 {
		// no history file
		return
	}

	filepath := cmd.HistoryFile // start with current directory
	if _, err := os.Stat(filepath); err == nil {
		if err := readline.ReadHistoryFile(filepath); err != nil {
			fmt.Println(err)
		}

		return
	}

	filepath = path.Join(os.Getenv("HOME"), filepath) // then check home directory
	if _, err := os.Stat(filepath); err == nil {
		if err := readline.ReadHistoryFile(filepath); err != nil {
			fmt.Println(err)
		}
	}

	// update HistoryFile with home path
	cmd.HistoryFile = filepath
}

func (cmd *Cmd) writeHistoryFile() {
	if len(cmd.HistoryFile) == 0 {
		// no history file
		return
	}

	if err := readline.WriteHistoryFile(cmd.HistoryFile); err != nil {
		fmt.Println(err)
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
		cmd.PostLoop = func() {}
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

	cmd.completer = NewCompleter(cmd.commandNames)
	//readline.SetCompletionEntryFunction(completer.Complete)

	readline.SetAttemptedCompletionFunction(cmd.attemptedCompletion)
}

func (cmd *Cmd) attemptedCompletion(text string, start, end int) []string {
	if start == 0 { // this is the command to match
		return readline.CompletionMatches(text, cmd.completer.Complete)
	} else if cmd.Complete != nil {
		return cmd.Complete(text, readline.GetLineBuffer(), start, end)
	} else {
		return nil
	}
}

//
// execute shell command
//
func shellExec(command string) {
	args := args.GetArgs(command)
	if len(args) < 1 {
		fmt.Println("No command to exec")
	} else {
		var cmd *exec.Cmd;

		if runtime.GOOS == "windows" {
			cmdArgs := []string{"cmd","/C"}
			cmdArgs = append(cmdArgs,args...)
			cmd = exec.Command(cmdArgs[0],cmdArgs[1:]...)
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
	cmd.Commands[command.name] = command
}

//
// Default help command.
// It lists all available commands or it displays the help for the specified command
//
func (cmd *Cmd) Help(command* Command,line string) (stop bool) {
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

		

		args := strings.Split(line," ")

		
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

				args := strings.Split(line," ")
				subcommand.flags.Parse(args[2:])

				stop = subcommand.call(subcommand,params)

				subcommand.flags.VisitAll(func(flag *flag.Flag) {
					flag.Value.Set(flag.DefValue)
				})
				return
			}
						

			params = strings.TrimSpace(parts[1])
		}

		args := strings.Split(line," ")
		command.flags.Parse(args[1:])
		stop = command.call(command,params)

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
		result := readline.ReadLine(&cmd.Prompt)
		if result == nil {
			//break
			continue
		}

		line := strings.TrimSpace(*result)
		if line == "" {
			cmd.EmptyLine()
			continue
		}

		readline.AddHistory(*result) // allow user to recall this line

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
