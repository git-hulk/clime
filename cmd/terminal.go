package cmd

import (
	"fmt"

	uicli "github.com/alperdrsnn/clime"
)

type Terminal struct{}

var terminal Terminal

func (Terminal) Info(message string) {
	uicli.InfoLine(message)
}

func (t Terminal) Infof(format string, args ...any) {
	t.Info(fmt.Sprintf(format, args...))
}

func (Terminal) Error(message string) {
	uicli.ErrorLine(message)
}

func (t Terminal) Errorf(format string, args ...any) {
	t.Error(fmt.Sprintf(format, args...))
}

func (Terminal) Success(message string) {
	uicli.SuccessLine(message)
}

func (t Terminal) Successf(format string, args ...any) {
	t.Success(fmt.Sprintf(format, args...))
}

func (Terminal) Warning(message string) {
	uicli.WarningLine(message)
}

func (t Terminal) Warningf(format string, args ...any) {
	t.Warning(fmt.Sprintf(format, args...))
}
