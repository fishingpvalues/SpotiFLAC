//go:build headless

package main

// emit is a no-op in the headless CLI: there is no frontend to publish to.
//
// Calling runtime.EventsEmit here is not merely useless, it kills the
// process. Wails resolves its event bus out of the context it created during
// its own lifecycle, and the CLI has no such context - it calls
// SetContext(context.Background()), which is non-nil and therefore passes any
// nil check, but is not a Wails context. EventsEmit rejects it with
//
//	cannot call 'github.com/wailsapp/wails/v2/pkg/runtime.EventsEmit':
//	An invalid context was passed. This method requires the specific context
//	given in the lifecycle hooks
//
// and exits 1. Since GetSpotifyMetadata emits on every metadata batch, that
// happened on the first one, so `spotiflac-cli --url ...` printed
// {"type":"status","message":"fetching metadata"} and then died - for every
// download, on every invocation. From the caller's side it looked like a
// bare "exit status 1" with no explanation.
func (a *App) emit(event string, data ...interface{}) {}
