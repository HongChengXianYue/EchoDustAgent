package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"local-agent/internal/ui"
)

// startupInfo 由 main() 填充，slash handler 读取。用包级变量避免每次
// dispatch 都透传上下文；新增命令时 handler 可以直接读，不用改签名。
var startupInfo ui.StartupInfo

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
}

// dispatchSlash 尝试把 input 作为 /命令 处理。
// 返回 true 表示 input 已被消费（无论成功/失败），调用者应跳过 agent 执行；
// 返回 false 表示 input 不是 /命令，应交给 agent 当普通输入。
func dispatchSlash(input string) (handled bool) {
	if !strings.HasPrefix(input, "/") {
		return false
	}
	name, args := parseSlash(input)
	cmd, ok := slashCommands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: /%s\n", name)
		printSlashHelp()
		return true
	}
	if err := cmd.handler(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return true
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
