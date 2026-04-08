package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	uicli "github.com/alperdrsnn/clime"
	"golang.org/x/term"
)

// SelectConfig configures a selection prompt.
type SelectConfig struct {
	Label   string
	Options []string
	Default int
}

// ErrCancelled is returned when the user cancels a selection prompt.
var ErrCancelled = errors.New("selection cancelled")

// ErrBack is returned when the user wants to go back one level.
var ErrBack = errors.New("selection back")

// ErrInterrupted is returned when the user presses Ctrl+C.
var ErrInterrupted = errors.New("selection interrupted")

// crlf is used instead of bare \n so that output renders correctly
// in raw terminal mode where \n does not imply a carriage return.
const crlf = "\r\n"

// Select shows a single-selection prompt with arrow key navigation.
func Select(config SelectConfig) (int, error) {
	if len(config.Options) == 0 {
		return 0, fmt.Errorf("no options provided")
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return selectFallback(config)
	}
	return selectInteractive(config)
}

// MultiSelect shows a multi-selection prompt with arrow key navigation.
func MultiSelect(config SelectConfig) ([]int, error) {
	if len(config.Options) == 0 {
		return nil, fmt.Errorf("no options provided")
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return multiSelectFallback(config)
	}
	return multiSelectInteractive(config)
}

// --- single select (interactive) ---

func selectInteractive(config SelectConfig) (int, error) {
	current := config.Default
	if current >= len(config.Options) {
		current = 0
	}

	uicli.HideCursor()
	defer uicli.ShowCursor()

	displaySelect(config, current)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return selectFallback(config)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	for {
		b := make([]byte, 4)
		n, err := os.Stdin.Read(b)
		if err != nil {
			return 0, err
		}

		if n == 1 {
			switch b[0] {
			case 13: // Enter
				clearLines(len(config.Options) + 2)
				fmt.Printf("%s %s%s", uicli.Info.Sprint("?"), config.Label, crlf)
				fmt.Printf("  %s %s%s", uicli.Success.Sprint("→"), config.Options[current], crlf)
				return current, nil
			case 3: // Ctrl+C
				clearLines(len(config.Options) + 2)
				return 0, ErrInterrupted
			case 27: // Esc
				clearLines(len(config.Options) + 2)
				return 0, ErrBack
			}
		} else if n >= 3 && b[0] == 27 && b[1] == 91 {
			switch b[2] {
			case 65: // Up
				if current > 0 {
					current--
				} else {
					current = len(config.Options) - 1
				}
				refreshSelect(config, current)
			case 66: // Down
				if current < len(config.Options)-1 {
					current++
				} else {
					current = 0
				}
				refreshSelect(config, current)
			}
		}
	}
}

func displaySelect(config SelectConfig, current int) {
	fmt.Printf("%s %s%s", uicli.Info.Sprint("?"), config.Label, crlf)
	fmt.Printf("%s%s", uicli.Muted.Sprint("(↑/↓ navigate, Enter select, Esc back, Ctrl+C exit)"), crlf)
	for i, option := range config.Options {
		if i == current {
			fmt.Printf("  %s %s%s", uicli.Success.Sprint("→"), uicli.BoldColor.Sprint(option), crlf)
		} else {
			fmt.Printf("    %s%s", option, crlf)
		}
	}
}

func refreshSelect(config SelectConfig, current int) {
	fmt.Printf("\033[%dA\033[J", len(config.Options)+2)
	displaySelect(config, current)
}

// --- single select (fallback for non-terminal) ---

func selectFallback(config SelectConfig) (int, error) {
	fmt.Printf("%s %s\n", uicli.Info.Sprint("?"), config.Label)
	for i, option := range config.Options {
		marker := " "
		if i == config.Default {
			marker = ">"
		}
		fmt.Printf("  %s %d) %s\n", marker, i+1, option)
	}
	fmt.Printf("Select (1-%d): ", len(config.Options))

	input, err := readLine()
	if err != nil {
		return 0, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return config.Default, nil
	}
	sel, err := strconv.Atoi(input)
	if err != nil || sel < 1 || sel > len(config.Options) {
		fmt.Printf("Invalid selection. Please choose a number between 1 and %d\n", len(config.Options))
		return selectFallback(config)
	}
	return sel - 1, nil
}

// --- multi select (interactive) ---

func multiSelectInteractive(config SelectConfig) ([]int, error) {
	current := 0
	selected := make(map[int]bool)

	uicli.HideCursor()
	defer uicli.ShowCursor()

	displayMultiSelect(config, current, selected)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return multiSelectFallback(config)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	for {
		b := make([]byte, 4)
		n, err := os.Stdin.Read(b)
		if err != nil {
			return nil, err
		}

		if n == 1 {
			switch b[0] {
			case 13: // Enter
				clearLines(len(config.Options) + 2)
				var result []int
				for i := range config.Options {
					if selected[i] {
						result = append(result, i)
					}
				}
				fmt.Printf("%s %s%s", uicli.Info.Sprint("?"), config.Label, crlf)
				if len(result) > 0 {
					fmt.Printf("  %s Selected %d option(s)%s", uicli.Success.Sprint("→"), len(result), crlf)
				} else {
					fmt.Printf("  %s No options selected%s", uicli.Warning.Sprint("→"), crlf)
				}
				return result, nil
			case 3: // Ctrl+C
				clearLines(len(config.Options) + 2)
				return nil, ErrInterrupted
			case 27: // Esc
				clearLines(len(config.Options) + 2)
				return nil, ErrBack
			case 32: // Space
				selected[current] = !selected[current]
				refreshMultiSelect(config, current, selected)
			}
		} else if n >= 3 && b[0] == 27 && b[1] == 91 {
			switch b[2] {
			case 65: // Up
				if current > 0 {
					current--
				} else {
					current = len(config.Options) - 1
				}
				refreshMultiSelect(config, current, selected)
			case 66: // Down
				if current < len(config.Options)-1 {
					current++
				} else {
					current = 0
				}
				refreshMultiSelect(config, current, selected)
			}
		}
	}
}

func displayMultiSelect(config SelectConfig, current int, selected map[int]bool) {
	fmt.Printf("%s %s%s", uicli.Info.Sprint("?"), config.Label, crlf)
	fmt.Printf("%s%s", uicli.Muted.Sprint("(↑/↓ navigate, Space select, Enter confirm, Esc back, Ctrl+C exit)"), crlf)
	for i, option := range config.Options {
		marker := "○"
		if selected[i] {
			marker = uicli.Success.Sprint("●")
		}
		if i == current {
			fmt.Printf("  %s %s %s%s", uicli.Success.Sprint("→"), marker, uicli.BoldColor.Sprint(option), crlf)
		} else {
			fmt.Printf("    %s %s%s", marker, option, crlf)
		}
	}
}

func refreshMultiSelect(config SelectConfig, current int, selected map[int]bool) {
	fmt.Printf("\033[%dA\033[J", len(config.Options)+2)
	displayMultiSelect(config, current, selected)
}

// --- multi select (fallback for non-terminal) ---

func multiSelectFallback(config SelectConfig) ([]int, error) {
	selected := make(map[int]bool)
	for {
		fmt.Printf("%s %s (toggle by number, Enter to confirm)\n", uicli.Info.Sprint("?"), config.Label)
		for i, option := range config.Options {
			marker := "○"
			if selected[i] {
				marker = "●"
			}
			fmt.Printf("  %s %d) %s\n", marker, i+1, option)
		}
		fmt.Printf("Toggle (1-%d) or Enter to confirm: ", len(config.Options))

		input, err := readLine()
		if err != nil {
			return nil, err
		}
		input = strings.TrimSpace(input)
		if input == "" {
			var result []int
			for i := range config.Options {
				if selected[i] {
					result = append(result, i)
				}
			}
			return result, nil
		}
		sel, err := strconv.Atoi(input)
		if err != nil || sel < 1 || sel > len(config.Options) {
			continue
		}
		selected[sel-1] = !selected[sel-1]
	}
}

// --- text input ---

// Input shows a simple text input prompt and returns the entered string.
func Input(label string) (string, error) {
	fmt.Printf("%s %s: ", uicli.Info.Sprint("?"), label)
	input, err := readLine()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

// --- helpers ---

func clearLines(n int) {
	fmt.Printf("\033[%dA\033[J", n)
}

func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, _, err := reader.ReadLine()
	if err != nil {
		return "", err
	}
	return strings.TrimRightFunc(string(line), unicode.IsSpace), nil
}
