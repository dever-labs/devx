package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runClone(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("clone", flag.ExitOnError)
	noSetup := fs.Bool("no-setup", false, "Skip running devx setup after cloning")
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: devx clone <repo> [directory]")
	}

	repoArg := fs.Arg(0)
	repoURL := expandRepoArg(repoArg)

	// Determine target directory.
	dir := fs.Arg(1)
	if dir == "" {
		dir = repoBaseName(repoURL)
	}

	// Clone.
	fmt.Printf("Cloning %s into %s...\n", repoURL, dir)
	cloneCmd := exec.Command("git", "clone", repoURL, dir)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	// Change into the cloned directory.
	if err := os.Chdir(abs); err != nil {
		return fmt.Errorf("could not enter %s: %w", abs, err)
	}

	fmt.Printf("Cloned into %s\n", abs)

	if *noSetup {
		return nil
	}

	// Run devx setup if devx.yaml exists.
	if _, err := os.Stat("devx.yaml"); err != nil {
		fmt.Println()
		fmt.Println("No devx.yaml found — skipping setup.")
		fmt.Println("Run  devx setup  to configure your environment.")
		return nil
	}

	fmt.Println()
	fmt.Println("Found devx.yaml — running setup...")
	fmt.Println()
	return runSetup(context.Background(), nil)
}

// expandRepoArg turns shorthand like "dever-labs/devx" into a full GitHub URL.
func expandRepoArg(arg string) string {
	// Already a full URL.
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "git@") ||
		strings.HasPrefix(arg, "https://") {
		return arg
	}
	// owner/repo shorthand → GitHub HTTPS URL.
	if strings.Count(arg, "/") == 1 {
		return "https://github.com/" + arg
	}
	return arg
}

// repoBaseName extracts a directory name from a clone URL.
func repoBaseName(url string) string {
	// Strip trailing .git and take the last path segment.
	url = strings.TrimSuffix(url, ".git")
	if idx := strings.LastIndexAny(url, "/:"); idx >= 0 {
		return url[idx+1:]
	}
	return url
}
