package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/zyedidia/tcell"
)

// TermMessage sends a message to the user in the terminal. This usually occurs before
// micro has been fully initialized -- ie if there is an error in the syntax highlighting
// regular expressions
// The function must be called when the screen is not initialized
// This will write the message, and wait for the user
// to press and key to continue
func TermMessage(msg ...interface{}) {
	screenWasNil := screen == nil
	if !screenWasNil {
		screen.Fini()
	}

	fmt.Println(msg...)
	fmt.Print("\nPress enter to continue")

	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	if !screenWasNil {
		InitScreen()
	}
}

// TermError sends an error to the user in the terminal. Like TermMessage except formatted
// as an error
func TermError(filename string, lineNum int, err string) {
	TermMessage(filename + ", " + strconv.Itoa(lineNum) + ": " + err)
}

// Messenger is an object that makes it easy to send messages to the user
// and get input from the user
type Messenger struct {
	// Are we currently prompting the user?
	hasPrompt bool
	// Is there a message to print
	hasMessage bool

	// Message to print
	message string
	// The user's response to a prompt
	response string
	// style to use when drawing the message
	style tcell.Style

	// We have to keep track of the cursor for prompting
	cursorx int

	// This map stores the history for all the different kinds of uses Prompt has
	// It's a map of history type -> history array
	history    map[string][]string
	historyNum int

	// Is the current message a message from the gutter
	gutterMessage bool
}

// Message sends a message to the user
func (m *Messenger) Message(msg ...interface{}) {
	buf := new(bytes.Buffer)
	fmt.Fprint(buf, msg...)
	m.message = buf.String()
	m.style = defStyle

	if _, ok := colorscheme["message"]; ok {
		m.style = colorscheme["message"]
	}
	m.hasMessage = true
}

// Error sends an error message to the user
func (m *Messenger) Error(msg ...interface{}) {
	buf := new(bytes.Buffer)
	fmt.Fprint(buf, msg...)
	m.message = buf.String()
	m.style = defStyle.
		Foreground(tcell.ColorBlack).
		Background(tcell.ColorMaroon)

	if _, ok := colorscheme["error-message"]; ok {
		m.style = colorscheme["error-message"]
	}
	m.hasMessage = true
}

// YesNoPrompt asks the user a yes or no question (waits for y or n) and returns the result
func (m *Messenger) YesNoPrompt(prompt string) (bool, bool) {
	m.Message(prompt)

	_, h := screen.Size()
	for {
		m.Clear()
		m.Display()
		screen.ShowCursor(Count(m.message), h-1)
		screen.Show()
		event := <-events

		switch e := event.(type) {
		case *tcell.EventKey:
			switch e.Key() {
			case tcell.KeyRune:
				if e.Rune() == 'y' {
					return true, false
				} else if e.Rune() == 'n' {
					return false, false
				}
			case tcell.KeyCtrlC, tcell.KeyCtrlQ, tcell.KeyEscape:
				return false, true
			}
		}
	}
}

type Completion int

const (
	NoCompletion Completion = iota
	FileCompletion
)

// Prompt sends the user a message and waits for a response to be typed in
// This function blocks the main loop while waiting for input
func (m *Messenger) Prompt(prompt, historyType string, completionType Completion) (string, bool) {
	m.hasPrompt = true
	m.Message(prompt)
	if _, ok := m.history[historyType]; !ok {
		m.history[historyType] = []string{""}
	} else {
		m.history[historyType] = append(m.history[historyType], "")
	}
	m.historyNum = len(m.history[historyType]) - 1

	response, canceled := "", true

	for m.hasPrompt {
		m.Clear()
		m.Display()

		event := <-events

		switch e := event.(type) {
		case *tcell.EventKey:
			switch e.Key() {
			case tcell.KeyCtrlQ, tcell.KeyCtrlC, tcell.KeyEscape:
				// Cancel
				m.hasPrompt = false
			case tcell.KeyEnter:
				// User is done entering their response
				m.hasPrompt = false
				response, canceled = m.response, false
				m.history[historyType][len(m.history[historyType])-1] = response
			case tcell.KeyTab:
				if completionType == FileCompletion {
					args := strings.Split(m.response, " ")
					currentArg := args[len(args)-1]
					dirs := strings.Split(currentArg, "/")
					var files []os.FileInfo
					var err error
					if len(dirs) > 1 {
						files, err = ioutil.ReadDir(strings.Join(dirs[:len(dirs)-1], "/"))
					} else {
						files, err = ioutil.ReadDir(".")
					}
					if err != nil {
						continue
					}
					var suggestions []string
					for _, f := range files {
						name := f.Name()
						if f.IsDir() {
							name += "/"
						}
						if strings.HasPrefix(name, dirs[len(dirs)-1]) {
							suggestions = append(suggestions, name)
						}
					}
					if len(suggestions) == 1 {
						if len(dirs) > 1 {
							currentArg = strings.Join(dirs[:len(dirs)-1], "/") + "/" + suggestions[0]
						} else {
							currentArg = suggestions[0]
						}
						if len(args) > 1 {
							currentArg = " " + currentArg
						}
						m.response = strings.Join(args[:len(args)-1], " ") + currentArg
						m.cursorx = Count(m.response)
					}
				}
			}
		}

		m.HandleEvent(event, m.history[historyType])
	}

	m.Reset()
	return response, canceled
}

// HandleEvent handles an event for the prompter
func (m *Messenger) HandleEvent(event tcell.Event, history []string) {
	switch e := event.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyUp:
			if m.historyNum > 0 {
				m.historyNum--
				m.response = history[m.historyNum]
				m.cursorx = Count(m.response)
			}
		case tcell.KeyDown:
			if m.historyNum < len(history)-1 {
				m.historyNum++
				m.response = history[m.historyNum]
				m.cursorx = Count(m.response)
			}
		case tcell.KeyLeft:
			if m.cursorx > 0 {
				m.cursorx--
			}
		case tcell.KeyRight:
			if m.cursorx < Count(m.response) {
				m.cursorx++
			}
		case tcell.KeyBackspace2, tcell.KeyBackspace:
			if m.cursorx > 0 {
				m.response = string([]rune(m.response)[:m.cursorx-1]) + string(m.response[m.cursorx:])
				m.cursorx--
			}
		case tcell.KeyRune:
			m.response = Insert(m.response, m.cursorx, string(e.Rune()))
			m.cursorx++
		}
		history[m.historyNum] = m.response
	}
}

// Reset resets the messenger's cursor, message and response
func (m *Messenger) Reset() {
	m.cursorx = 0
	m.message = ""
	m.response = ""
}

// Clear clears the line at the bottom of the editor
func (m *Messenger) Clear() {
	w, h := screen.Size()
	for x := 0; x < w; x++ {
		screen.SetContent(x, h-1, ' ', nil, defStyle)
	}
}

// Display displays messages or prompts
func (m *Messenger) Display() {
	_, h := screen.Size()
	if m.hasMessage {
		runes := []rune(m.message + m.response)
		for x := 0; x < len(runes); x++ {
			screen.SetContent(x, h-1, runes[x], nil, m.style)
		}
	}
	if m.hasPrompt {
		screen.ShowCursor(Count(m.message)+m.cursorx, h-1)
		screen.Show()
	}
}

// A GutterMessage is a message displayed on the side of the editor
type GutterMessage struct {
	lineNum int
	msg     string
	kind    int
}

// These are the different types of messages
const (
	// GutterInfo represents a simple info message
	GutterInfo = iota
	// GutterWarning represents a compiler warning
	GutterWarning
	// GutterError represents a compiler error
	GutterError
)
