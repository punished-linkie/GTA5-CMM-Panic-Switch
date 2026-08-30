package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"time"
)

func goSafe(fn interface{}, args ...interface{}) {
	go func() {
		defer recoverLog()

		// Convert args to reflect.Value slice
		valFn := reflect.ValueOf(fn)
		valArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			valArgs[i] = reflect.ValueOf(arg)
		}

		// Execute the function with its parameters
		valFn.Call(valArgs)
	}()
}

func recoverLog() {
	if r := recover(); r != nil {
		// Open or create a crash log file
		f, err := os.OpenFile("crash.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("Failed to open crash log: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		// Write panic and stack trace
		stack := debug.Stack()
		_, _ = fmt.Fprintf(f, "\nCRASH TIME: %s\n", time.Now().Format(time.RFC850))
		_, _ = fmt.Fprintf(f, "CRASH PANIC: %v\n%s\n", r, stack)

		// Redact sensitive info and dump state
		state.DuckToken = "****"
		config, _ := json.Marshal(state)
		_, _ = fmt.Fprintf(f, "CONFIG: %s\n\n\n", string(config))

		fmt.Fprintf(os.Stderr, "Program crashed. Details written to crash.log\n")

		// Force the entire application to exit completely
		os.Exit(1)
	}
}
