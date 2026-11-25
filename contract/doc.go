// Package contract parses and runs certification-style CONTRACT.md files.
//
// A contract is a Markdown file of numbered assertions. Each assertion is a
// level-3 heading (its name, e.g. "F-01"), one line of prose, a console block
// ("$ command" lines, then the expected output) and an "exit: N" line.
// Placeholders such as {ID}, {PORT}, {TIME} and {KEY} bind on first match and
// apply consistently across later steps. --json output is compared
// structurally; plain text only where the contract gives it.
package contract
