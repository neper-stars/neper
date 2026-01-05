package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
	colorReset  = "\033[0m"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
}

func run() error {
	fmt.Printf("%s=== Neper Release Script ===%s\n\n", colorBlue, colorReset)

	// Show last tag
	lastTag, err := getLastTag()
	if err != nil {
		lastTag = "No previous tags"
	}
	fmt.Printf("Last tag: %s%s%s\n\n", colorGreen, lastTag, colorReset)

	// Ask for new version
	version, err := prompt("Enter new version (e.g., v1.2.0): ")
	if err != nil {
		return err
	}

	if !isValidVersion(version) {
		return fmt.Errorf("version must be in format v1.2.3")
	}

	// Check if changie batch was already run for this version
	changieVersionFile := fmt.Sprintf(".changes/%s.md", version)
	if _, err := os.Stat(changieVersionFile); err == nil {
		fmt.Printf("%s=== Changie already processed for %s (found %s), skipping ===%s\n\n",
			colorYellow, version, changieVersionFile, colorReset)
	} else {
		// Show dry-run preview of changelog content
		fmt.Printf("\n%s=== Changelog preview (dry-run) ===%s\n\n", colorBlue, colorReset)
		if err := runCommand("changie", "batch", version, "--dry-run"); err != nil {
			return fmt.Errorf("changie batch dry-run failed: %w", err)
		}

		// Confirm before making changes
		fmt.Println()
		confirmChangie, err := confirm("Do you want to proceed with this changelog?")
		if err != nil {
			return err
		}
		if !confirmChangie {
			fmt.Printf("%sAborted.%s\n", colorYellow, colorReset)
			return nil
		}

		// Run changie batch
		fmt.Printf("\n%s=== Running changie batch %s ===%s\n\n", colorBlue, version, colorReset)
		if err := runCommand("changie", "batch", version); err != nil {
			return fmt.Errorf("changie batch failed: %w", err)
		}

		// Run changie merge
		fmt.Printf("\n%s=== Running changie merge ===%s\n\n", colorBlue, colorReset)
		if err := runCommand("changie", "merge"); err != nil {
			return fmt.Errorf("changie merge failed: %w", err)
		}
	}

	// Show files to be staged
	fmt.Printf("\n%s=== Files to be staged ===%s\n\n", colorBlue, colorReset)
	if err := runCommand("git", "status", "--short"); err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}

	// Confirm staging
	fmt.Println()
	confirmAdd, err := confirm("Do you want to stage all these files?")
	if err != nil {
		return err
	}

	if !confirmAdd {
		fmt.Printf("%sAborted. You can manually stage files and run:%s\n", colorYellow, colorReset)
		fmt.Printf("  git add <files>\n")
		fmt.Printf("  git commit -m \"Release %s\"\n", version)
		fmt.Printf("  git tag %s\n", version)
		fmt.Printf("  git push && git push --tags\n")
		return nil
	}

	// Stage files
	fmt.Printf("\n%s=== Staging files ===%s\n\n", colorBlue, colorReset)
	if err := runCommand("git", "add", "."); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Create commit
	fmt.Printf("\n%s=== Creating commit ===%s\n\n", colorBlue, colorReset)
	if err := runCommand("git", "commit", "-m", fmt.Sprintf("Release %s", version)); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Create tag
	fmt.Printf("\n%s=== Creating tag %s ===%s\n\n", colorBlue, version, colorReset)
	if err := runCommand("git", "tag", version); err != nil {
		return fmt.Errorf("git tag failed: %w", err)
	}

	// Confirm push
	fmt.Println()
	confirmPush, err := confirm("Do you want to push the commit and tag now?")
	if err != nil {
		return err
	}

	if !confirmPush {
		fmt.Printf("%sTag created locally. To push later, run:%s\n", colorYellow, colorReset)
		fmt.Printf("  git push && git push --tags\n")
		return nil
	}

	// Push
	fmt.Printf("\n%s=== Pushing to remote ===%s\n\n", colorBlue, colorReset)
	if err := runCommand("git", "push"); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	if err := runCommand("git", "push", "--tags"); err != nil {
		return fmt.Errorf("git push --tags failed: %w", err)
	}

	fmt.Printf("\n%s=== Release %s complete! ===%s\n", colorGreen, version, colorReset)
	return nil
}

func getLastTag() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func isValidVersion(version string) bool {
	match, _ := regexp.MatchString(`^v\d+\.\d+\.\d+$`, version)
	return match
}

func prompt(message string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(message)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func confirm(message string) (bool, error) {
	input, err := prompt(fmt.Sprintf("%s (y/N): ", message))
	if err != nil {
		return false, err
	}
	return strings.ToLower(input) == "y" || strings.ToLower(input) == "yes", nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
