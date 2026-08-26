// Package registry is the shared table of top-level jfps command groups.
// Each feature's CLI package (internal/cli/<group>) registers itself here
// from an init() function; cmd/jfps/main.go blank-imports every feature
// package so registration happens automatically at startup. This keeps each
// feature package compilable and testable in complete isolation from the
// others during development.
package registry

// CommandFunc runs one top-level command group with the remaining CLI
// arguments and returns a process exit code.
type CommandFunc func(args []string) int

var (
	commands     = map[string]CommandFunc{}
	commandOrder []string
	commandHelp  = map[string]string{}
)

// Register adds a top-level command group, e.g. Register("access", "Manage
// users, groups, and permissions", Run).
func Register(name, help string, fn CommandFunc) {
	if _, exists := commands[name]; !exists {
		commandOrder = append(commandOrder, name)
	}
	commands[name] = fn
	commandHelp[name] = help
}

// Get returns the command function registered for name, if any.
func Get(name string) (CommandFunc, bool) {
	fn, ok := commands[name]
	return fn, ok
}

// Names returns all registered command group names, in registration order.
func Names() []string {
	return append([]string(nil), commandOrder...)
}

// Help returns the one-line help text for a registered command group.
func Help(name string) string {
	return commandHelp[name]
}
