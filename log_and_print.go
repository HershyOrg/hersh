package hersh

import (
	"fmt"

	"github.com/HershyOrg/hersh/mctx"
	"github.com/HershyOrg/hersh/shared"
)

// Log writes a message to the effect log via HershContext.
// This allows users to log from within managed functions.
// The message is logged using the Logger instance associated with the context.
func Log(s string, ctx shared.ManageContext) {
	// Extract the logger from HershContext
	// HershContext is implemented by hctx.HershContext which has a logger field
	if hc, ok := ctx.(*mctx.ManageContext); ok {
		if logger := getLoggerFromContext(hc); logger != nil {
			logger.LogEffect(s)
		}
	}
}

// PrintWithLog prints a message to stdout and logs it via HershContext.
// This combines console output with persistent logging.
func PrintWithLog(s string, ctx shared.ManageContext) {
	fmt.Println(s)
	Log(s, ctx)
}

// getLoggerFromContext extracts the logger from HershContext.
// This is a helper function to access the private logger field.
func getLoggerFromContext(hc *mctx.ManageContext) mctx.Logger {
	// We need to add a public getter method to HershContext
	return hc.GetLogger()
}
