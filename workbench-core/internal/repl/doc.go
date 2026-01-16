// Package repl provides a Read-Eval-Print Loop for workbench.
//
// The REPL allows users to interact with workbench runs through a command-line
// interface with readline support (history, editing, completion).
//
// # Features
//
//   - Line editing with arrow keys
//   - Command history (up/down arrows)
//   - Tab completion (future)
//   - Graceful interrupt handling (Ctrl+C)
//
// # Usage Pattern
//
//	rl, err := repl.NewReadline()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer rl.Close()
//
//	for {
//	    line, err := rl.ReadLine("workbench> ")
//	    if err == io.EOF {
//	        break // User pressed Ctrl+D
//	    }
//	    // Process command...
//	}
//
// # Readline Integration
//
// The package wraps the chzyer/readline library to provide a consistent
// interface for interactive input across different terminal environments.
package repl
