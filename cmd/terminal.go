package cmd

import (
	"fmt"

	uicli "github.com/alperdrsnn/clime"
)

type Terminal struct{}

var terminal Terminal

func (Terminal) Info(message string) {
	fmt.Println(uicli.DimColor.Sprint("ℹ " + message))
}

func (terminal Terminal) Infof(format string, args ...any) {
	terminal.Info(fmt.Sprintf(format, args...))
}

func (Terminal) Error(message string) {
	uicli.ErrorLine(message)
}

func (terminal Terminal) Errorf(format string, args ...any) {
	terminal.Error(fmt.Sprintf(format, args...))
}

func (Terminal) Success(message string) {
	uicli.SuccessLine(message)
}

func (terminal Terminal) Successf(format string, args ...any) {
	terminal.Success(fmt.Sprintf(format, args...))
}

func (Terminal) Warning(message string) {
	uicli.WarningLine(message)
}

func (terminal Terminal) Warningf(format string, args ...any) {
	terminal.Warning(fmt.Sprintf(format, args...))
}
