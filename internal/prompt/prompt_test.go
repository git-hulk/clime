package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiSelectKeepsSelectionsAcrossPages(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input")
	require.NoError(t, os.WriteFile(inputPath, []byte("p\n1\nn\n11\nn\n21\nn\np\np\np\n\n"), 0o600))
	input, err := os.Open(inputPath)
	require.NoError(t, err)
	defer input.Close()
	output, err := os.Create(filepath.Join(dir, "output"))
	require.NoError(t, err)
	defer output.Close()
	stdin, stdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = input, output
	t.Cleanup(func() { os.Stdin, os.Stdout = stdin, stdout })
	config := SelectConfig{Label: "Pick skills", PageSize: 10}
	for i := 1; i <= 21; i++ {
		config.Options = append(config.Options, fmt.Sprintf("skill-%02d", i))
	}
	selected, err := MultiSelect(config)
	require.NoError(t, err)
	require.Equal(t, []int{0, 10, 20}, selected)
	data, err := os.ReadFile(output.Name())
	require.NoError(t, err)
	for _, page := range strings.Split(string(data), "Pick skills")[1:] {
		count := strings.Count(page, "skill-")
		require.GreaterOrEqual(t, count, 1)
		require.LessOrEqual(t, count, 10)
	}
	for _, label := range []string{"Page 1/3", "Page 2/3", "Page 3/3"} {
		require.Contains(t, string(data), label)
	}
}
