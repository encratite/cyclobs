package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
)

func plotData(argument string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Fatalf("Failed to serialize results to JSON: %v", err)
	}
	cmd := exec.Command("python", "python/plot.py", argument)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr: %v", err)
	}
	err = cmd.Start()
	if err != nil {
		log.Fatalf("Failed to start Python process: %v", err)
	}
	_, err = stdin.Write(jsonData)
	if err != nil {
		log.Fatalf("Failed to write JSON data to Python process: %v", err)
	}
	stdin.Close()
	output, err := io.ReadAll(stdout)
	if err != nil {
		log.Fatalf("Failed to read data from stdout: %v", err)
	}
	errorOutput, err := io.ReadAll(stderr)
	if err != nil {
		log.Fatalf("Failed to read data from stderr: %v", err)
	}
	_ = cmd.Wait()
	if len(output) > 0 {
		fmt.Print(string(output))
	}
	if len(errorOutput) > 0 {
		fmt.Print(string(errorOutput))
	}
}