package main

import "fmt"

type WidgetEvent struct {
	Nothing string
}

type WidgetControl interface {
	HandleEvent(e *WidgetEvent)
}

type Button struct{}

func (b *Button) HandleEvent(e *WidgetEvent) {
}

type WindowEvent struct {
	Something int
}

type Window struct {
	Widget WidgetControl
}

func (w *Window) HandleEvent(e *WindowEvent) {
}

func main() {
	window := &Window{
		Widget: &Button{},
	}
	windowevent := &WindowEvent{}
	// The next line uses the signature from the wrong method, resulting in an error.
	// Renaming one of the clashing method names fixes the problem.
	window.HandleEvent(windowevent)
	fmt.Println("OK!")
}

// Output:
// OK!
