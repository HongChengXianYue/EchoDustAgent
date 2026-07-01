package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"local-agent/internal/ui"
)

// startupInfo 由 main() 填充，slash handler 读取。用包级变量避免每次
// dispatch 都透传上下文；新增命令时 handler 可以直接读，不用改签名。
var startupInfo ui.StartupInfo

// errExit 是退出 sentinel error，handler 返回它表示用户请求退出。
// dispatchSlash 检测到后通过 shouldExit 返回值通知 main 循环 return。
var errExit = errors.New("exit")

// slashHandler 是 /命令 的执行函数。args 是命令名之后的空白分隔参数
//（已做过 strings.Fields 清洗），不会为 nil 但可能为空切片。
type slashHandler func(args []string) error

// slashCommand 注册表：key 是命令名（不含 /），value 含帮助文本和 handler。
// 新命令只需在这里加一行，dispatchSlash 会自动识别。
var slashCommands = map[string]struct {
	desc    string
	handler slashHandler
}{
	"info":  {desc: "show startup details (workdir, model, mcp tools, log file)", handler: slashInfo},
	"model": {desc: "show or switch the active LLM model", handler: slashModel},
	"exit":  {desc: "exit the agent", handler: slashExit},
	"quit":  {desc: "exit the agent", handler: slashExit},
}

// dispatchSlash 尝试把 input 作为 /命令 处理。
// handled=true 表示 input 已被消费（无论成功/失败），调用者应跳过 agent 执行；
// handled=false 表示 input 不是 /命令，应交给 agent 当普通输入。
// shouldExit=true 表示用户请求退出（/exit 或 /quit），调用者应 return。
func dispatchSlash(input string) (handled bool, shouldExit bool) {
	if !strings.HasPrefix(input, "/") {
		return false, false
	}
	name, args := parseSlash(input)
	cmd, ok := slashCommands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: /%s\n", name)
		printSlashHelp()
		return true, false
	}
	if err := cmd.handler(args); err != nil {
		if errors.Is(err, errExit) {
			return true, true
		}
		fmt.Fprintln(os.Stderr, err)
	}
	return true, false
}

// parseSlash 把 "/cmd arg1 arg2" 拆成 ("cmd", ["arg1", "arg2"])。
// 命令名取第一个空白前的部分并 trim；剩余部分按空白切分为参数。
func parseSlash(input string) (name string, args []string) {
	input = strings.TrimPrefix(input, "/")
	parts := strings.SplitN(input, " ", 2)
	name = strings.TrimSpace(parts[0])
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return name, nil
	}
	return name, strings.Fields(parts[1])
}

// printSlashHelp 列出所有已注册命令，给用户一个可操作的提示。
func printSlashHelp() {
	fmt.Fprintln(os.Stderr, "available commands:")
	for name, cmd := range slashCommands {
		fmt.Fprintf(os.Stderr, "  /%-8s %s\n", name, cmd.desc)
	}
}

func slashInfo(_ []string) error {
	ui.RenderStartupDetails(os.Stdout, startupInfo)
	return nil
}

// slashExit 返回 errExit sentinel error，dispatchSlash 检测到后通知 main 循环退出。
func slashExit(_ []string) error {
	return errExit
}

// slashModel 预留：无参数时打印当前模型；有参数时提示尚未实现。
// 切换模型需要重建 llm.Client 并热替换到 codingAgent，涉及
// agent 生命周期管理，等需求明确后再实现。
func slashModel(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(os.Stdout, "current model: %s\n", startupInfo.Model)
		return nil
	}
	return fmt.Errorf("/model switch not yet implemented (requested: %s)", strings.Join(args, " "))
}

// SlashCommandList 返回按名称排序的 /命令 列表，供 UI 输入框做建议补全。
func SlashCommandList() []ui.CommandSuggestion {
	cmds := make([]ui.CommandSuggestion, 0, len(slashCommands))
	for name, cmd := range slashCommands {
		cmds = append(cmds, ui.CommandSuggestion{Name: name, Desc: cmd.desc})
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}
