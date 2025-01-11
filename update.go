package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type Updater struct{}

func ThisBinaryPath() (string, error) {
	binPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("error getting executable path: %v", err)
	}
	return filepath.Clean(binPath), nil
}

func ThisBinaryAsBytes() ([]byte, error) {
	binPath, err := ThisBinaryPath()
	if err != nil {
		return nil, fmt.Errorf("error getting executable path: %v", err)
	}
	binary, err := os.ReadFile(binPath)
	if err != nil {
		return nil, fmt.Errorf("error reading binary: %v", err)
	}
	return binary, nil
}

func ThisBinaryVersion() (string, error) {
	binary, err := ThisBinaryAsBytes()
	if err != nil {
		return "", fmt.Errorf("error getting binary: %v", err)
	}
	return CalculateChecksum(string(binary)).String(), nil
}
