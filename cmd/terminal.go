package cmd

import (
	"fmt"

	uicli "github.com/alperdrsnn/clime"
)

type cliTerminal struct{}

var terminal cliTerminal

func (cliTerminal) Info(message string) {
	uicli.InfoLine(message)
}

func (t cliTerminal) Infof(format string, args ...any) {
	t.Info(fmt.Sprintf(format, args...))
}

func (cliTerminal) Error(message string) {
	uicli.ErrorLine(message)
}

func (t cliTerminal) Errorf(format string, args ...any) {
	t.Error(fmt.Sprintf(format, args...))
}

func (cliTerminal) Success(message string) {
	uicli.SuccessLine(message)
}

func (t cliTerminal) Successf(format string, args ...any) {
	t.Success(fmt.Sprintf(format, args...))
}

func (cliTerminal) Warning(message string) {
	uicli.WarningLine(message)
}

func (t cliTerminal) Warningf(format string, args ...any) {
	t.Warning(fmt.Sprintf(format, args...))
}
