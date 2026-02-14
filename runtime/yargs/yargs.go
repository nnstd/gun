package yargs

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	jsvalue "github.com/nnstd/gun/runtime/jsvalue"
)

// Yargs implements a fluent command-line argument parser compatible with the
// yargs npm package API.
type Yargs struct {
	args           []string
	commands       []*command
	options        map[string]*option
	demandMin      int
	demandMsg      string
	helpEnabled    bool
	currentCommand *command // set during builder phase
}

type command struct {
	pattern     string
	description string
	positionals []string
	builder     any
	handler     any
}

type option struct {
	name         string
	alias        string
	optType      string
	defaultVal   any
	description  string
	demandOption bool
}

// Argv wraps parsed command-line arguments for typed access in handler callbacks.
type Argv struct {
	inner *jsvalue.JSValue
}

// NewArgv creates an Argv from a JSValue object.
func NewArgv(v *jsvalue.JSValue) *Argv {
	return &Argv{inner: v}
}

// Get returns the value of a named argument.
func (a *Argv) Get(name string) *jsvalue.JSValue {
	return a.inner.Get(name)
}

// String returns the string representation of the underlying value.
func (a *Argv) String() string {
	return a.inner.String()
}

// Bool returns the boolean value of the underlying value.
func (a *Argv) Bool() bool {
	return a.inner.Bool()
}

// Default creates a new Yargs instance from the given args.
// This is the default export of the yargs package.
func Default(args []string) *Yargs {
	return &Yargs{
		args:    args,
		options: make(map[string]*option),
	}
}

// Command defines a command with a pattern, description, builder, and handler.
func (y *Yargs) Command(pattern string, desc string, builder any, handler any) *Yargs {
	cmd := &command{
		pattern:     pattern,
		description: desc,
		builder:     builder,
		handler:     handler,
	}
	// Extract positional names from pattern like "greet <name>"
	re := regexp.MustCompile(`<(\w+)>`)
	matches := re.FindAllStringSubmatch(pattern, -1)
	for _, m := range matches {
		cmd.positionals = append(cmd.positionals, m[1])
	}
	y.commands = append(y.commands, cmd)
	return y
}

// Positional defines a positional argument for the current command.
func (y *Yargs) Positional(name string, opts ...any) *Yargs {
	// Positional metadata is captured during builder phase.
	// The actual parsing uses the command pattern.
	return y
}

// Option defines a named option.
func (y *Yargs) Option(name string, opts any) *Yargs {
	o := &option{name: name}
	if m, ok := opts.(map[string]*jsvalue.JSValue); ok {
		if v, exists := m["alias"]; exists {
			o.alias = v.String()
		}
		if v, exists := m["type"]; exists {
			o.optType = v.String()
		}
		if v, exists := m["default"]; exists {
			switch v.Type() {
			case jsvalue.TypeBoolean:
				o.defaultVal = v.Bool()
			case jsvalue.TypeNumber:
				o.defaultVal = v.Number()
			case jsvalue.TypeString:
				o.defaultVal = v.String()
			}
		}
		if v, exists := m["describe"]; exists {
			o.description = v.String()
		}
	}
	y.options[name] = o
	return y
}

// DemandCommand requires at least min commands.
func (y *Yargs) DemandCommand(min int, msg ...any) *Yargs {
	y.demandMin = min
	if len(msg) > 0 {
		y.demandMsg = fmt.Sprint(msg[0])
	}
	return y
}

// Help enables the --help flag.
func (y *Yargs) Help() *Yargs {
	y.helpEnabled = true
	return y
}

// Parse parses the arguments and executes the matched command.
func (y *Yargs) Parse() {
	// Check for --help or -h
	for _, a := range y.args {
		if a == "--help" || a == "-h" {
			y.printHelp()
			os.Exit(0)
		}
	}

	if len(y.args) == 0 {
		if y.demandMin > 0 {
			fmt.Fprintln(os.Stderr, y.demandMsg)
			y.printHelp()
			os.Exit(1)
		}
		return
	}

	cmdName := y.args[0]
	var matched *command
	for _, cmd := range y.commands {
		// Match command name (first word of pattern)
		parts := strings.Fields(cmd.pattern)
		if len(parts) > 0 && parts[0] == cmdName {
			matched = cmd
			break
		}
	}

	if matched == nil {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmdName)
		y.printHelp()
		os.Exit(1)
	}

	// Run builder if present (for positional definitions)
	// The builder receives a JSValue wrapper; positionals are already
	// extracted from the command pattern so the builder is informational only.
	if matched.builder != nil {
		y.currentCommand = matched
		switch fn := matched.builder.(type) {
		case func(*Yargs) *Yargs:
			fn(y)
		default:
			// Builder callbacks from transpiled code receive *jsvalue.JSValue;
			// skip calling them since positionals come from the command pattern.
			_ = fn
		}
		y.currentCommand = nil
	}

	// Parse remaining args after command name
	argv := jsvalue.NewObject()
	remaining := y.args[1:]

	// Apply defaults
	for name, opt := range y.options {
		if opt.defaultVal != nil {
			argv.Set(name, jsvalue.From(opt.defaultVal))
		}
	}

	posIdx := 0
	i := 0
	for i < len(remaining) {
		arg := remaining[i]
		if strings.HasPrefix(arg, "--") {
			key := arg[2:]
			resolved := y.resolveOption(key)
			if resolved != nil && resolved.optType == "boolean" {
				argv.Set(resolved.name, jsvalue.NewBool(true))
			} else if i+1 < len(remaining) {
				name := key
				if resolved != nil {
					name = resolved.name
				}
				argv.Set(name, jsvalue.NewString(remaining[i+1]))
				i++
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) == 2 {
			alias := string(arg[1])
			resolved := y.resolveAlias(alias)
			if resolved != nil && resolved.optType == "boolean" {
				argv.Set(resolved.name, jsvalue.NewBool(true))
			} else if i+1 < len(remaining) {
				name := alias
				if resolved != nil {
					name = resolved.name
				}
				argv.Set(name, jsvalue.NewString(remaining[i+1]))
				i++
			}
		} else {
			// Positional argument
			if posIdx < len(matched.positionals) {
				argv.Set(matched.positionals[posIdx], jsvalue.NewString(arg))
				posIdx++
			}
		}
		i++
	}

	// Execute handler
	if matched.handler != nil {
		argvWrapped := NewArgv(argv)
		switch fn := matched.handler.(type) {
		case func(*Argv):
			fn(argvWrapped)
		case func(*jsvalue.JSValue):
			fn(argv)
		case func(*jsvalue.JSValue) *jsvalue.JSValue:
			fn(argv)
		}
	}
}

func (y *Yargs) resolveOption(name string) *option {
	if o, ok := y.options[name]; ok {
		return o
	}
	return nil
}

func (y *Yargs) resolveAlias(alias string) *option {
	for _, o := range y.options {
		if o.alias == alias {
			return o
		}
	}
	return nil
}

func (y *Yargs) printHelp() {
	fmt.Fprintf(os.Stderr, "Commands:\n")
	for _, cmd := range y.commands {
		fmt.Fprintf(os.Stderr, "  %-30s %s\n", cmd.pattern, cmd.description)
	}
	if len(y.options) > 0 {
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		for _, o := range y.options {
			flag := "--" + o.name
			if o.alias != "" {
				flag += ", -" + o.alias
			}
			fmt.Fprintf(os.Stderr, "  %-30s %s\n", flag, o.description)
		}
	}
	fmt.Fprintf(os.Stderr, "  %-30s %s\n", "--help, -h", "Show help")
}
