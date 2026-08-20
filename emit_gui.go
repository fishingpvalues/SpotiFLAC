//go:build !headless

package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// emit publishes a Wails event to the frontend.
//
// Every call used to be an inline runtime.EventsEmit(a.ctx, ...). That is
// correct here and fatal in the headless CLI, which is why this is split by
// build tag - see emit_headless.go.
func (a *App) emit(event string, data ...interface{}) {
	runtime.EventsEmit(a.ctx, event, data...)
}
