package ux

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// StdPrompter implements Prompter using standard input/output
type StdPrompter struct{}

func NewStdPrompter() Prompter {
	return &StdPrompter{}
}

func (p *StdPrompter) Select(message string, options []string) (string, error) {
	fmt.Println(message)
	for i, option := range options {
		fmt.Printf("%d. %s\n", i+1, option)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Enter your choice (number): ")
		if !scanner.Scan() {
			return "", scanner.Err()
		}

		input := strings.TrimSpace(scanner.Text())
		choice, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Please enter a valid number.")
			continue
		}

		if choice < 1 || choice > len(options) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(options))
			continue
		}

		return options[choice-1], nil
	}
}

func (p *StdPrompter) Confirm(message string) (bool, error) {
	fmt.Printf("%s (y/n): ", message)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false, scanner.Err()
	}

	input := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return input == "y" || input == "yes", nil
}

func (p *StdPrompter) Input(message string) (string, error) {
	fmt.Printf("%s: ", message)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", scanner.Err()
	}

	return strings.TrimSpace(scanner.Text()), nil
}
