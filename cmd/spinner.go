// SPDX-License-Identifier: Apache-2.0
package cmd

import (
	"fmt"
	"os"
	"time"
)

// spinner shows an animated progress indicator on stderr while a blocking
// operation runs. It is a no-op when stderr is not an interactive terminal.
type spinner struct {
	msg  string
	stop chan struct{}
	done chan struct{}
}

// newSpinner creates a new spinner with the given status message.
// Call Start() to begin the animation, Stop() to end it.
func newSpinner(msg string) *spinner {
	return &spinner{
		msg:  msg,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// isTTY reports whether f is connected to an interactive terminal.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// Start begins the spinner animation in a background goroutine.
// It writes only to stderr and is safe to use alongside stdout output.
func (s *spinner) Start() {
	go func() {
		defer close(s.done)

		if !isTTY(os.Stderr) {
			// Non-interactive (CI, piped output): skip animation, just block.
			<-s.stop
			return
		}

		var frames []string
		if useEmoji() {
			frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		} else {
			frames = []string{"|", "/", "-", "\\"}
		}

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		for {
			select {
			case <-s.stop:
				fmt.Fprintf(os.Stderr, "\r\033[K") // erase the spinner line
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], s.msg)
				i++
			}
		}
	}()
}

// Stop halts the spinner and erases the spinner line.
func (s *spinner) Stop() {
	close(s.stop)
	<-s.done
}
