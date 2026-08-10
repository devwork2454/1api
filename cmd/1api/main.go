// Command 1api detects the Codex, Claude Code, OpenCode, and Pi CLIs and
// switches their endpoint + credentials between saved profiles.
package main

import (
	"fmt"
	"os"

	"1api/internal/profile"
	"1api/internal/tools"
	"1api/internal/tui"
)

// version is set at build time via -ldflags (see .goreleaser.yaml).
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if ee, ok := err.(*exitError); ok {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "1api: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "update":
			return cmdUpdate()
		case "uninstall":
			return cmdUninstall()
		case "version", "-v", "--version":
			fmt.Println("1api " + version)
			return nil
		case "help", "-h", "--help":
			printUsage()
			return nil
		}
	}

	store, err := profile.Open()
	if err != nil {
		return err
	}
	// Capture the pristine config of every detected tool on first sight.
	for _, t := range tools.All() {
		if err := store.EnsureDefault(t); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not snapshot %s: %v\n", t.Name, err)
		}
	}
	if err := store.MigrateProvidersOnce(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: provider migration: %v\n", err)
	}

	if len(args) == 0 {
		return tui.Run(store, version)
	}

	switch args[0] {
	case "status", "st":
		return cmdStatus(store, args[1:])
	case "ls":
		return cmdList(store, args[1:])
	case "switch":
		return cmdSwitch(store, args[1:])
	case "use":
		return cmdUse(store, args[1:])
	case "run":
		return cmdRun(store, args[1:])
	case "alias":
		return cmdAlias(args[1:])
	case "restore":
		return cmdSwitch(store, append([]string{argAt(args, 1)}, profile.DefaultName))
	case "undo":
		return cmdUndo(store, args[1:])
	case "prune":
		return cmdPrune(store, args[1:])
	case "save":
		return cmdSave(store, args[1:])
	case "refresh":
		return cmdRefresh(store, args[1:])
	case "models":
		return cmdModels(args[1:])
	case "verify":
		return cmdVerify(args[1:])
	case "provider":
		return cmdProvider(store, args[1:])
	case "add":
		return cmdAdd(store, args[1:])
	case "edit":
		return cmdEdit(store, args[1:])
	case "rename", "mv":
		return cmdRename(store, args[1:])
	case "cp":
		return cmdDuplicate(store, args[1:])
	case "rm":
		return cmdRemove(store, args[1:])
	case "completion":
		return cmdCompletion(args[1:])
	case "__profiles": // hidden: feeds shell completion
		return cmdProfiles(store, args[1:])
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func argAt(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func printUsage() {
	fmt.Print(`1api — detect and switch AI tool endpoints + credentials

Usage:
  1api                     interactive menu
  1api status              show each tool's active profile, endpoint and auth (--json)
  1api ls <tool>           list saved profiles for a tool (--json)
  1api save <tool> [name]  snapshot current live config as a profile
                             (omit name to auto-name after the logged-in account)
  1api refresh <tool>      capture in-session changes (model, effort) into active profile
  1api models <tool>       list models from an API (--key, --endpoint)
  1api verify <tool>       probe live endpoint (list + chat) and show mid/low/high
                              (--key --endpoint --model; defaults from live config)
  1api provider ls         list central providers (--json)
  1api provider add        add a provider once (--name --key [--endpoint --wire --model --low --high] [--no-verify])
  1api provider edit <n>   edit a provider (--endpoint --key --model --low --high --wire) [--no-verify]
  1api provider rm <n>     delete a provider
  1api provider verify <n> re-probe usable models and refresh mid/low/high
  1api provider models <n> show usable models and tier assignment
  1api use <tool> <prov>   bind a tool to a central provider and apply it [--no-verify]
  1api add <tool>          add provider + bind tool (--name --key [--endpoint --model])
                              verifies connectivity before write; --no-verify to skip
  1api edit <tool> <p>     change a profile's endpoint/key/model/name
                              OpenCode verifies connectivity before write; --no-verify to skip
  1api rename <tool> <o> <n>  rename a saved profile
  1api cp <tool> <src> <dst>  duplicate a saved profile
  1api switch <tool> <p>   apply a saved profile (backs up current first)
  1api run <tool> <p>      start tool with profile in a temp HOME (session only)
                             codex/opencode; --keep retains the sandbox dir
  1api alias <tool> <p>    print a shell function wrapping 1api run
                             [--name NAME] [--shell bash|zsh|fish]
  1api restore <tool>      revert to the auto-captured default
  1api undo <tool>         revert to the most recent pre-switch backup
  1api prune <tool>        delete old backups, keeping the newest (--keep N)
  1api rm <tool> <p>       delete a saved profile
  1api completion <shell>  print a bash/zsh/fish completion script
  1api update              upgrade from Gitee releases (GitHub fallback)
                             (override URL with CHARON_UPDATE_URL)
  1api uninstall           remove the installed 1api binary

Tools: codex, claude, opencode, pi
Session run: codex, opencode
`)
}
