package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMultiSelectKeepsSelectionsAcrossPages(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input")
	if err := os.WriteFile(inputPath, []byte("p\n1\nn\n11\nn\n21\nn\np\np\np\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.Create(filepath.Join(dir, "output"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stdin, stdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = input, output
	t.Cleanup(func() { os.Stdin, os.Stdout = stdin, stdout })
	config := SelectConfig{Label: "Pick skills", PageSize: 10}
	for i := 1; i <= 21; i++ {
		config.Options = append(config.Options, fmt.Sprintf("skill-%02d", i))
	}
	selected, err := MultiSelect(config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected, []int{0, 10, 20}) {
		t.Fatalf("selected = %v, want [0 10 20]", selected)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range strings.Split(string(data), "Pick skills")[1:] {
		if count := strings.Count(page, "skill-"); count < 1 || count > 10 {
			t.Fatalf("page has %d skills, want 1–10: %s", count, page)
		}
	}
	for _, label := range []string{"Page 1/3", "Page 2/3", "Page 3/3"} {
		if !strings.Contains(string(data), label) {
			t.Fatalf("output missing %q", label)
		}
	}
}
