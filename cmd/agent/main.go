package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"local-agent/internal/agent"
	"local-agent/internal/config"
	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	workdir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry, workdir)
	client := llm.NewOpenAICompatibleClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	codingAgent := agent.New(client, registry, cfg.MaxSteps)

	fmt.Println("local-agent started")
	fmt.Println("workdir:", workdir)
	fmt.Println("model:", cfg.Model)
	fmt.Println("type exit or quit to stop")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			return
		}

		answer, err := codingAgent.Run(context.Background(), input)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			continue
		}
		fmt.Println(answer)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "input error:", err)
	}
}
