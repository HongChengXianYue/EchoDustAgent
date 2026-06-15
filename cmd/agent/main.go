package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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
	tools.RegisterBuiltinsWithOptions(registry, workdir, toolOptions(cfg.Tools))
	client := llm.NewOpenAICompatibleClientWithOptions(cfg.LLM.BaseURL, cfg.APIKey, cfg.LLM.Model, llm.OpenAICompatibleOptions{
		Timeout:           time.Duration(cfg.LLM.RequestTimeoutSeconds) * time.Second,
		ParallelToolCalls: cfg.LLM.ParallelToolCalls,
	})
	codingAgent := agent.NewWithWorkspace(client, registry, cfg.Agent.MaxSteps, workdir)
	codingAgent.SetRenderer(ui.NewInteractiveBlockRendererWithOptions(os.Stdin, os.Stdout, uiOptions(cfg.UI)))
	codingAgent.SetApprover(approval.NewMemoryApprover(approval.NewTerminalApprover(os.Stdin, os.Stdout)))

	fmt.Println("local-agent started")
	fmt.Println("workdir:", workdir)
	fmt.Println("model:", cfg.LLM.Model)
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

func toolOptions(cfg config.ToolsConfig) tools.Options {
	return tools.Options{
		ListMaxEntries:               cfg.ListMaxEntries,
		FindMaxMatches:               cfg.FindMaxMatches,
		ReadFileMaxBytes:             cfg.ReadFileMaxBytes,
		SearchMaxMatches:             cfg.SearchMaxMatches,
		SearchMaxFileBytes:           cfg.SearchMaxFileBytes,
		CommandDefaultTimeoutSeconds: cfg.CommandDefaultTimeoutSeconds,
		CommandMaxTimeoutSeconds:     cfg.CommandMaxTimeoutSeconds,
		CommandOutputMaxBytes:        cfg.CommandOutputMaxBytes,
		ApplyPatchTimeoutSeconds:     cfg.ApplyPatchTimeoutSeconds,
		ApplyPatchOutputMaxBytes:     cfg.ApplyPatchOutputMaxBytes,
		FileChangePreviewLines:       cfg.FileChangePreviewLines,
	}
}

func uiOptions(cfg config.UIConfig) ui.Options {
	return ui.Options{
		SeparatorWidth:             cfg.SeparatorWidth,
		LiveFrameMaxLines:          cfg.LiveFrameMaxLines,
		LiveFrameMaxWidth:          cfg.LiveFrameMaxWidth,
		LiveFrameHeightMargin:      cfg.LiveFrameHeightMargin,
		MaxExpandedLiveToolEvents:  cfg.MaxExpandedLiveToolEvents,
		FullLogDefaultWidth:        cfg.FullLogDefaultWidth,
		FullLogDefaultHeight:       cfg.FullLogDefaultHeight,
		FullLogMinWidth:            cfg.FullLogMinWidth,
		FullLogMinHeight:           cfg.FullLogMinHeight,
		FullLogPollMilliseconds:    cfg.FullLogPollMilliseconds,
		TogglePollMilliseconds:     cfg.TogglePollMilliseconds,
		MarkdownWordWrap:           cfg.MarkdownWordWrap,
		ToolPreviewOutputChars:     cfg.ToolPreviewOutputChars,
		ToolPreviewLongOutputChars: cfg.ToolPreviewLongOutputChars,
		FileChangePreviewChars:     cfg.FileChangePreviewChars,
		ApprovalArgsPreviewChars:   cfg.ApprovalArgsPreviewChars,
	}
}
