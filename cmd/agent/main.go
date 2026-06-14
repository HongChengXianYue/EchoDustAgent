package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"local-agent/internal/agent"
	"local-agent/internal/approval"
	"local-agent/internal/config"
	"local-agent/internal/llm"
	"local-agent/internal/tools"
	"local-agent/internal/ui"
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
	codingAgent := agent.NewWithWorkspace(client, registry, cfg.MaxSteps, workdir)
	codingAgent.SetRenderer(ui.NewInteractiveBlockRenderer(os.Stdin, os.Stdout))
	codingAgent.SetApprover(approval.NewMemoryApprover(approval.NewTerminalApprover(os.Stdin, os.Stdout)))

	fmt.Println("local-agent started")
	fmt.Println("workdir:", workdir)
	fmt.Println("model:", cfg.Model)
	fmt.Println("type exit or quit to stop")

	prompt := ui.NewPrompt(os.Stdin, os.Stdout)
	for {
		line, ok := prompt.ReadLine("› ")
		if !ok {
			break
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			return
		}

		_, _ = codingAgent.Run(context.Background(), input)
	}
}
