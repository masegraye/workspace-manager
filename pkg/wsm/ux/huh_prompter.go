package ux

import (
	"github.com/charmbracelet/huh"
)

// HuhPrompter implements MultiSelectPrompter using charmbracelet/huh
type HuhPrompter struct{}

func NewHuhPrompter() MultiSelectPrompter {
	return &HuhPrompter{}
}

func (p *HuhPrompter) Select(message string, options []string) (string, error) {
	var result string

	err := huh.NewSelect[string]().
		Title(message).
		Options(huh.NewOptions(options...)...).
		Value(&result).
		Run()

	return result, err
}

func (p *HuhPrompter) Confirm(message string) (bool, error) {
	var result bool

	err := huh.NewConfirm().
		Title(message).
		Value(&result).
		Run()

	return result, err
}

func (p *HuhPrompter) Input(message string) (string, error) {
	var result string

	err := huh.NewInput().
		Title(message).
		Value(&result).
		Run()

	return result, err
}

// MultiSelect provides multi-selection functionality
func (p *HuhPrompter) MultiSelect(message string, options []string) ([]string, error) {
	var result []string

	err := huh.NewMultiSelect[string]().
		Title(message).
		Options(huh.NewOptions(options...)...).
		Value(&result).
		Run()

	return result, err
}
