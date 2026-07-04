package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"local-agent/internal/agent"
	"local-agent/internal/approval"
	"local-agent/internal/config"
	"local-agent/internal/llm"
	"local-agent/internal/logs"
	"local-agent/internal/mcp"
	"local-agent/internal/memory"
	"local-agent/internal/session"
	"local-agent/internal/skill"
	"local-agent/internal/tools"
	"local-agent/internal/tui"
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
	logger, err := logs.New(workdir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logger.Close()
	logs.SetDefault(logger)

	registry := tools.NewRegistry()
	tools.RegisterBuiltinsWithOptions(registry, workdir, toolOptions(cfg.Tools))
	var loadedMemory *memory.Set
	if cfg.Memory.Enabled {
		loadedMemory = memory.Load(memory.Options{CWD: workdir, UserDir: cfg.Memory.UserDir})
		registry.Register(memory.NewRecallTool(loadedMemory.Store))
		registry.Register(memory.NewRememberTool(loadedMemory.Store))
		registry.Register(memory.NewForgetTool(loadedMemory.Store))
	}
	var skillRegistry *skill.Registry
	if cfg.Skills.Enabled {
		skillRegistry, err = skill.LoadRegistry(skill.Options{
			CWD:        workdir,
			UserDir:    cfg.Skills.UserDir,
			ProjectDir: cfg.Skills.ProjectDir,
		})
		if err != nil {
			logs.Errorf("skill registry loaded with warnings: %v", err)
		}
	}
	var mcpManager *mcp.Manager
	if cfg.MCP.Enabled {
		mcpManager, err = mcp.Load(context.Background(), mcp.Options{
			Dir:            cfg.MCP.Dir,
			StartTimeout:   time.Duration(cfg.MCP.StartTimeoutSeconds) * time.Second,
			RequestTimeout: time.Duration(cfg.MCP.RequestTimeoutSeconds) * time.Second,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer mcpManager.Close()
		mcpManager.Register(registry)
	}
	client := llm.NewOpenAICompatibleClientWithOptions(cfg.LLM.BaseURL, cfg.APIKey, cfg.LLM.Model, llm.OpenAICompatibleOptions{
		Timeout:           time.Duration(cfg.LLM.RequestTimeoutSeconds) * time.Second,
		WireAPI:           cfg.LLM.WireAPI,
		ParallelToolCalls: cfg.LLM.ParallelToolCalls,
	})
	codingAgent := agent.NewWithWorkspaceAndOptions(client, registry, cfg.Agent.MaxSteps, workdir, agentOptions(cfg.Agent, cfg.Subagents, cfg.Skills, cfg.Context, loadedMemory, skillRegistry))

	startupInfo := ui.StartupInfo{
		Workdir:    workdir,
		Model:      cfg.LLM.Model,
		WireAPI:    cfg.LLM.WireAPI,
		MCPEnabled: mcpManager != nil,
		MCPTools:   mcpToolCount(mcpManager),
		LogFile:    logger.Path(),
	}
	sessions, err := newSessionRuntime(cfg.Session, workdir, codingAgent, &startupInfo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var runStateMu sync.Mutex
	isRunning := false
	setRunning := func(v bool) {
		runStateMu.Lock()
		isRunning = v
		runStateMu.Unlock()
	}
	currentlyRunning := func() bool {
		runStateMu.Lock()
		defer runStateMu.Unlock()
		return isRunning
	}
	slash := newSlashRouter(&startupInfo, sessions, currentlyRunning)

	if isInteractiveTTY(os.Stdin) && isInteractiveTTY(os.Stdout) {
		if err := runTUI(codingAgent, cfg.UI, &startupInfo, slash, sessions, setRunning); err != nil {
			logs.Errorf("tui exited with error: %v", err)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	runClassicUI(codingAgent, cfg.UI, startupInfo, slash, sessions, setRunning)
}

func runTUI(codingAgent *agent.Agent, cfg config.UIConfig, startup *ui.StartupInfo, slash *slashRouter, sessions *sessionRuntime, setRunning func(bool)) (err error) {
	bridge := tui.NewBridge()
	model := tui.NewModel(uiOptions(cfg), *startup, bridge)
	model.SetRunFunc(func(ctx context.Context, input string) error {
		setRunning(true)
		defer setRunning(false)
		_, err := codingAgent.Run(ctx, input)
		return err
	})
	model.SetSlashFunc(slash.DispatchText)
	model.SetSlashCommands(slash.CommandList())
	model.SetResumePickerHandlers(
		func() ([]session.Meta, error) { return sessions.Recent(10) },
		func(sessionID string) (string, error) {
			_, err := sessions.Resume(sessionID)
			return "", err
		},
	)
	model.SetSessionSnapshotSaver(func(snapshot session.UISnapshot) {
		if err := sessions.SaveUISnapshot(snapshot); err != nil {
			logs.Errorf("save session snapshot failed: %v", err)
		}
	})
	codingAgent.SetRenderer(model)
	codingAgent.SetApprover(approval.NewMemoryApprover(tui.NewBubbleApprover(bridge)))
	sessions.SetUI(model)

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	bridge.SetProgram(program)

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(interrupts)
	go func() {
		for sig := range interrupts {
			logs.Errorf("received signal: %v", sig)
			program.Send(tui.SignalMsg{Signal: sig})
		}
	}()

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tui panic: %v", recovered)
		}
	}()

	_, err = program.Run()
	return err
}

func runClassicUI(codingAgent *agent.Agent, cfg config.UIConfig, startup ui.StartupInfo, slash *slashRouter, sessions *sessionRuntime, setRunning func(bool)) {
	renderer := ui.NewInteractiveBlockRendererWithOptions(os.Stdin, os.Stdout, uiOptions(cfg))
	codingAgent.SetRenderer(renderer)
	codingAgent.SetApprover(approval.NewMemoryApprover(approval.NewTerminalApprover(os.Stdin, os.Stdout)))
	ui.RenderStartupBanner(os.Stdout, startup)

	var runMu sync.Mutex
	var running bool
	var cancelCurrent context.CancelFunc
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	go func() {
		for sig := range interrupts {
			logs.Errorf("received signal: %v", sig)
			renderer.ReleaseTerminal()

			runMu.Lock()
			cancel := cancelCurrent
			active := running
			cancelCurrent = nil
			runMu.Unlock()

			if cancel != nil {
				cancel()
			}
			if !active {
				fmt.Fprintln(os.Stderr)
				os.Exit(130)
			}
		}
	}()

	prompt := ui.NewPrompt(os.Stdin, os.Stdout)
	prompt.SetCommands(slash.CommandList())
	for {
		line, ok := prompt.ReadLine("› ")
		if !ok {
			break
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if handled, shouldExit := slash.Dispatch(input); handled {
			if shouldExit {
				return
			}
			continue
		}

		runCtx, cancel := context.WithCancel(context.Background())
		runMu.Lock()
		running = true
		cancelCurrent = cancel
		runMu.Unlock()
		setRunning(true)

		if _, err := codingAgent.Run(runCtx, input); err != nil {
			logs.Errorf("agent run failed: input=%q err=%v", input, err)
			if !errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "run failed:", err)
			}
		}
		if runErr := runCtx.Err(); runErr == nil || !errors.Is(runErr, context.Canceled) {
			if err := sessions.SaveConversationOnly(); err != nil {
				logs.Errorf("save session failed: %v", err)
			}
		}
		cancel()
		runMu.Lock()
		running = false
		cancelCurrent = nil
		runMu.Unlock()
		setRunning(false)
	}
}

func isInteractiveTTY(file *os.File) bool {
	if file == nil {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
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

func mcpToolCount(manager *mcp.Manager) int {
	if manager == nil {
		return 0
	}
	return len(manager.Tools())
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

func agentOptions(agentCfg config.AgentConfig, subagentsCfg config.SubagentsConfig, skillsCfg config.SkillsConfig, contextCfg config.ContextConfig, loadedMemory *memory.Set, skillRegistry *skill.Registry) agent.Options {
	memoryBlock := ""
	if loadedMemory != nil {
		memoryBlock = loadedMemory.Block()
	}
	return agent.Options{
		MaxParallelToolCalls: agentCfg.MaxParallelToolCalls,
		StepBudget: agent.StepBudgetOptions{
			AdaptiveEnabled:  agentCfg.AdaptiveMaxStepsEnabled,
			MaxExtensions:    agentCfg.MaxStepExtensions,
			ExtensionSize:    agentCfg.StepExtensionSize,
			AbsoluteMaxSteps: agentCfg.AbsoluteMaxSteps,
		},
		SystemPromptSuffix: memoryBlock,
		Context: agent.ContextOptions{
			WindowTokens:             contextCfg.WindowTokens,
			PruneToolResultMaxBytes:  contextCfg.PruneToolResultMaxBytes,
			PruneKeepRecentMessages:  contextCfg.PruneKeepRecentMessages,
			CompactEnabled:           contextCfg.CompactEnabled,
			CompactRatioPercent:      contextCfg.CompactRatioPercent,
			CompactForceRatioPercent: contextCfg.CompactForceRatioPercent,
			CompactTargetPercent:     contextCfg.CompactTargetPercent,
			CompactKeepTailTokens:    contextCfg.CompactKeepTailTokens,
			CompactMinMessages:       contextCfg.CompactMinMessages,
		},
		Subagents: agent.SubagentOptions{
			Enabled:       subagentsCfg.Enabled,
			MaxConcurrent: subagentsCfg.MaxConcurrent,
			MaxSteps:      subagentsCfg.MaxSteps,
			StepBudget: agent.StepBudgetOptions{
				AdaptiveEnabled:  subagentsCfg.AdaptiveMaxStepsEnabled,
				MaxExtensions:    subagentsCfg.MaxStepExtensions,
				ExtensionSize:    subagentsCfg.StepExtensionSize,
				AbsoluteMaxSteps: subagentsCfg.AbsoluteMaxSteps,
			},
			ResultMaxBytes: subagentsCfg.ResultMaxBytes,
		},
		Skills: agent.SkillOptions{
			Enabled:  skillsCfg.Enabled,
			TopK:     skillsCfg.TopK,
			MinScore: skillsCfg.MinScore,
			Registry: skillRegistry,
		},
	}
}
