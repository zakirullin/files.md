// Converts [[wikilinks]] to standard markdown [Link Name](/path/Link%20Name.md).
// Scans all .md files in a directory first to resolve link targets by filename.
//
// Usage: go run ./cmd/tomdlinks <dir>
//
//	go run ./cmd/tomdlinks --dry-run <dir>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	dryRun := false
	dir := "."

	for _, arg := range os.Args[1:] {
		if arg == "--dry-run" {
			dryRun = true
		} else {
			dir = arg
		}
	}

	// Scan all .md files and build a map: display name -> relative path
	candidates := map[string]string{}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		name := strings.TrimSuffix(filepath.Base(rel), ".md")
		candidates[name] = rel
		return nil
	})

	wikiRE := regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		changed := false

		result := wikiRE.ReplaceAllStringFunc(content, func(match string) string {
			name := match[2 : len(match)-2]
			target, ok := candidates[name]
			if !ok {
				return match
			}
			// Escape parens too - an unescaped ) in a path closes the
			// markdown link early (mirrors web's encodeLinkPath).
			url := "/" + strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29").Replace(target)
			changed = true
			return fmt.Sprintf("[%s](%s)", name, url)
		})

		if !changed {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		if dryRun {
			fmt.Printf("would update: %s\n", rel)
			return nil
		}

		err = os.WriteFile(path, []byte(result), info.Mode())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %s\n", rel, err)
			return nil
		}
		fmt.Printf("updated: %s\n", rel)
		return nil
	})
}
