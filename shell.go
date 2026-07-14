package ivory

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

const prompt = "ivory> "

type command struct {
	name  string
	usage string
	desc  string
	run   func(e *Engine, args []string)
}

var commands []command
var commandMap map[string]command

func init() {
	commands = []command{
		{"status", "status", "show loaded crawlers and their state", cmdStatus},
		{"list", "list", "show available crawlers and stores", cmdList},
		{"start", "start <crawler|all>", "start one or more crawlers", cmdStart},
		{"stop", "stop <crawler|all>", "stop one or more crawlers", cmdStop},
		{"reload", "reload <crawler|all>", "restart a crawler with a fresh instance", cmdReload},
		{"help", "help", "show this message", cmdHelp},
		{"exit", "exit", "stop everything and quit", nil},
	}
	commandMap = make(map[string]command, len(commands))
	for _, c := range commands {
		commandMap[c.name] = c
	}
}

func RunShell(e *Engine) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("ivory shell. type 'help' for commands.")
	fmt.Print(prompt)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			fmt.Print(prompt)
			continue
		}

		name := strings.ToLower(fields[0])
		args := fields[1:]

		if name == "exit" || name == "quit" {
			return
		}

		cmd, ok := commandMap[name]
		if !ok {
			fmt.Printf("unknown command: %s (try 'help')\n", name)
			fmt.Print(prompt)
			continue
		}

		cmd.run(e, args)
		fmt.Print(prompt)
	}
}

func cmdStatus(e *Engine, args []string) {
	status := e.Status()
	for _, name := range sortedKeys(status) {
		state := red("stopped")
		if status[name] {
			state = green("running")
		}
		fmt.Printf("  %-16s %s\n", name, state)
	}
}

func cmdList(e *Engine, args []string) {
	fmt.Println("crawlers:")
	for _, name := range sortedKeys(GetRegisteredCrawlers()) {
		fmt.Printf("  %s\n", name)
	}
	fmt.Println("stores:")
	for _, name := range sortedKeys(GetRegisteredStores()) {
		fmt.Printf("  %s\n", name)
	}
}

func cmdStart(e *Engine, args []string) {
	if len(args) == 0 {
		fmt.Println("usage: start <crawler|all>")
		return
	}
	if isAll(args) {
		e.StartAll()
		return
	}
	for _, name := range args {
		report(e.Start(name))
	}
}

func cmdStop(e *Engine, args []string) {
	if len(args) == 0 {
		fmt.Println("usage: stop <crawler|all>")
		return
	}
	if isAll(args) {
		e.StopAll()
		return
	}
	for _, name := range args {
		report(e.Stop(name))
	}
}

func cmdReload(e *Engine, args []string) {
	if len(args) == 0 {
		fmt.Println("usage: reload <crawler|all>")
		return
	}
	targets := args
	if isAll(args) {
		targets = sortedKeys(e.Status())
	}
	for _, name := range targets {
		report(e.Reload(name))
	}
}

func cmdHelp(e *Engine, args []string) {
	fmt.Println("commands:")
	for _, c := range commands {
		fmt.Printf("  %-22s %s\n", c.usage, c.desc)
	}
}

func isAll(args []string) bool {
	return len(args) == 1 && args[0] == "all"
}

func report(err error) {
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
