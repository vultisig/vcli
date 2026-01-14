package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// promptPassword prompts the user for a password interactively.
// If the password flag was provided, it returns that instead.
// This handles special characters like ! that shells may interpret.
func promptPassword(flagPassword string, prompt string) (string, error) {
	if flagPassword != "" {
		return flagPassword, nil
	}

	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("password required but stdin is not a terminal. Use --password flag")
	}

	fmt.Print(prompt)
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // newline after password input
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	return string(passwordBytes), nil
}

// promptPasswordWithConfirm prompts for password twice and confirms they match.
func promptPasswordWithConfirm(flagPassword string) (string, error) {
	if flagPassword != "" {
		return flagPassword, nil
	}

	password, err := promptPassword("", "Enter password: ")
	if err != nil {
		return "", err
	}

	confirm, err := promptPassword("", "Confirm password: ")
	if err != nil {
		return "", err
	}

	if password != confirm {
		return "", fmt.Errorf("passwords do not match")
	}

	return password, nil
}

// promptYesNo prompts for a yes/no confirmation.
func promptYesNo(prompt string, defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)

	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}

	fmt.Print(prompt + suffix)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultYes
	}

	return input == "y" || input == "yes"
}
