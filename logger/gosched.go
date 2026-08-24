package logger

import "runtime"

// runtimeGosched exists so the async drain loop reads clearly at its call site
// and can be swapped for a backoff strategy if that ever proves necessary.
func runtimeGosched() { runtime.Gosched() }
