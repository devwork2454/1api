package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"charon/internal/models"
	"charon/internal/profile"
	"charon/internal/secret"
	"charon/internal/tools"
)

// requireTool resolves a tool-name argument, erroring with the supported names.
func requireTool(name string) (*tools.Tool, error) {
	if t := tools.Find(name); t != nil {
		return t, nil
	}
	var names []string
	for _, t := range tools.All() {
		names = append(names, t.Name)
	}
	if name == "" {
		return nil, fmt.Errorf("missing tool name (want %s)", strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("unknown tool %q (want %s)", name, strings.Join(names, ", "))
}

// splitTool returns the first non-flag arg (the tool) and the remaining args.
func splitTool(args []string) (tool string, rest []string) {
	for _, a := range args {
		if tool == "" && !strings.HasPrefix(a, "-") {
			tool = a
			continue
		}
		rest = append(rest, a)
	}
	return tool, rest
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

type statusRow struct {
	Tool     string `json:"tool"`
	Title    string `json:"title"`
	Detected bool   `json:"detected"`
	Active   string `json:"active,omitempty"`
	AuthMode string `json:"authMode,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Account  string `json:"account,omitempty"`
	Secret   string `json:"secret,omitempty"` // masked; never the raw value
	Modified bool   `json:"modified"`
}

func cmdStatus(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "machine-readable JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var rows []statusRow
	for _, t := range tools.All() {
		r := statusRow{Tool: t.Name, Title: t.Title}
		if t.Detected != nil && t.Detected() {
			info, _ := t.Describe()
			r.Detected = true
			r.Active = store.Active(t.Name)
			r.AuthMode = info.AuthMode
			r.Endpoint = info.Endpoint
			r.Model = info.Model
			r.Effort = info.Effort
			r.Account = info.Account
			r.Secret = secret.Mask(info.Secret)
			r.Modified, _ = store.Drift(t)
		}
		rows = append(rows, r)
	}

	if *asJSON {
		return printJSON(rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tACTIVE\tAUTH\tENDPOINT\tMODEL\tEFFORT\tSECRET")
	for _, r := range rows {
		if !r.Detected {
			fmt.Fprintf(w, "%s\t—\t(not detected)\t\t\t\t\n", r.Title)
			continue
		}
		active := r.Active
		if active == "" {
			active = "—"
		}
		if r.Modified {
			active += " (modified)" // live config changed since the last switch
		}
		model, effort := r.Model, r.Effort
		if model == "" {
			model = "—"
		}
		if effort == "" {
			effort = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Title, active, r.AuthMode, r.Endpoint, model, effort, r.Secret)
	}
	return w.Flush()
}

type profileRow struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Active   bool   `json:"active"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Account  string `json:"account,omitempty"`
}

func cmdList(store *profile.Store, args []string) error {
	toolName, rest := splitTool(args)
	if toolName == "" {
		return fmt.Errorf("usage: charon ls <tool> [--json]")
	}
	t, err := requireTool(toolName)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "machine-readable JSON output")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	active := store.Active(t.Name)
	var rows []profileRow
	for _, name := range store.List(t.Name) {
		m, _ := store.LoadManifest(t.Name, name)
		r := profileRow{Name: name, Label: m.Label, Account: m.Account, Active: name == active}
		sp, hasSpec := store.GetSpec(t.Name, name)
		if hasSpec {
			r.Endpoint = t.ResolveEndpoint(sp.Endpoint)
			r.Model = sp.Model
		}
		// ProfileModelEffort reads the profile's own captured config, which is more
		// accurate than the Add-time spec (e.g. reflects a later /model or /effort).
		if snapModel, snapEffort := store.ProfileModelEffort(t, name); snapModel != "" || snapEffort != "" {
			if snapModel != "" {
				r.Model = snapModel
			}
			r.Effort = snapEffort
		}
		rows = append(rows, r)
	}

	if *asJSON {
		return printJSON(rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tLABEL\tMODEL\tEFFORT")
	for _, r := range rows {
		marker := "  "
		if r.Active {
			marker = "* "
		}
		model, effort := r.Model, r.Effort
		if model == "" {
			model = "—"
		}
		if effort == "" {
			effort = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", marker, r.Name, r.Label, model, effort)
	}
	return w.Flush()
}

func cmdSwitch(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon switch <tool> <profile>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	backup, err := store.Apply(t, args[1])
	if err != nil {
		return err
	}
	fmt.Printf("Switched %s → %s\n(backup: %s)\n", t.Title, args[1], backup)
	return nil
}

func cmdSave(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon save <tool> [name] [--label ..] [--note ..]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	// A bare name (not a flag) is optional; without it, name after the logged-in account.
	rest := args[1:]
	name := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		name, rest = rest[0], rest[1:]
	}
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	label := fs.String("label", "", "human-friendly label")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if name == "" {
		saved, err := store.SaveCurrentAccount(t)
		if err != nil {
			return err
		}
		if err := store.SetActiveName(t.Name, saved); err != nil {
			return err
		}
		fmt.Printf("Backed up current %s account as profile %q (now active)\n", t.Title, saved)
		return nil
	}
	if err := store.Save(t, name, *label, *note); err != nil {
		return err
	}
	fmt.Printf("Saved current %s config as profile %q\n", t.Title, name)
	return nil
}

func cmdRefresh(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon refresh <tool>")
	}
	tool, err := requireTool(args[0])
	if err != nil {
		return err
	}
	active := store.Active(tool.Name)
	if active == "" {
		return fmt.Errorf("no active profile for %s", tool.Title)
	}
	if err := store.Refresh(tool); err != nil {
		return err
	}
	fmt.Printf("Captured current %s config into profile %q\n", tool.Title, active)
	return nil
}

func cmdUndo(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon undo <tool>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	restored, err := store.Undo(t)
	if err != nil {
		return err
	}
	fmt.Printf("Reverted %s to its most recent backup\n(restored: %s)\n", t.Title, restored)
	return nil
}

func cmdPrune(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon prune <tool> [--keep N]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	keep := fs.Int("keep", -1, "backups to retain (default 10)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	removed, err := store.PruneBackups(t.Name, *keep)
	if err != nil {
		return err
	}
	fmt.Printf("Pruned %d old %s backup(s)\n", removed, t.Title)
	return nil
}

func listProvider(t *tools.Tool, endpoint, wire string) models.Provider {
	if tools.ResolveWire(t, endpoint, wire) == tools.WireAnthropic {
		return models.Anthropic
	}
	return models.OpenAI
}

func discoverModels(t *tools.Tool, endpoint, key, wire string) []string {
	ep := t.ResolveEndpoint(endpoint)
	list, err := models.Fetch(listProvider(t, ep, wire), ep, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not list models: %v\n", err)
		return nil
	}
	return list
}

func noteWireHint(t *tools.Tool, endpoint, wire string) {
	if hint := tools.WireHintFor(t, endpoint, wire); hint != "" {
		fmt.Fprintln(os.Stderr, "note: "+hint)
	}
}

// resolveAPIKey prefers an explicit --key; otherwise reads --key-env (e.g. GEMINI_API_KEY).
func resolveAPIKey(key, keyEnv string) (string, error) {
	key = strings.TrimSpace(key)
	if key != "" {
		return key, nil
	}
	envName := strings.TrimSpace(keyEnv)
	if envName == "" {
		return "", fmt.Errorf("--key or --key-env is required")
	}
	val := strings.TrimSpace(os.Getenv(envName))
	if val == "" {
		return "", fmt.Errorf("environment variable %s is empty or unset", envName)
	}
	return val, nil
}

func cmdModels(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon models <tool> (--key <k>|--key-env <NAME>) [--endpoint <url>] [--wire openai|anthropic]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	keyEnv := fs.String("key-env", "", "read API key from this environment variable (e.g. GEMINI_API_KEY)")
	wireFlag := fs.String("wire", "", "protocol: openai, anthropic, or auto (OpenCode)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	resolvedKey, err := resolveAPIKey(*key, *keyEnv)
	if err != nil {
		return err
	}
	*key = resolvedKey
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	wire, err := tools.NormalizeWire(*wireFlag)
	if err != nil {
		return err
	}
	ep := t.ResolveEndpoint(*endpoint)
	noteWireHint(t, ep, wire)
	list, err := models.Fetch(listProvider(t, ep, wire), ep, *key)
	if err != nil {
		return err
	}
	for _, m := range list {
		fmt.Println(m)
	}
	return nil
}

func cmdAdd(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon add <tool> --name <p> (--key <k>|--key-env <NAME>) [--endpoint <url>] [--model <m>] [--wire openai|anthropic]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	keyEnv := fs.String("key-env", "", "read API key from this environment variable (e.g. GEMINI_API_KEY)")
	model := fs.String("model", "", "model id")
	name := fs.String("name", "", "profile name")
	wireFlag := fs.String("wire", "", "protocol: openai, anthropic, or auto (OpenCode)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if t.ApplyAuth == nil {
		return fmt.Errorf("%s does not support add", t.Title)
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	resolvedKey, err := resolveAPIKey(*key, *keyEnv)
	if err != nil {
		return err
	}
	*key = resolvedKey
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	wire, err := tools.NormalizeWire(*wireFlag)
	if err != nil {
		return err
	}
	ep := t.ResolveEndpoint(*endpoint)
	if t.Name == "opencode" && tools.ResolveWire(t, ep, wire) == tools.WireAnthropic {
		ep = tools.NormalizeAnthropicBaseURL(ep)
	}
	noteWireHint(t, ep, wire)
	allModels := discoverModels(t, ep, *key, wire)
	if err := store.AddProfile(t, *name, profile.Spec{Endpoint: ep, Key: *key, Model: *model, Wire: wire}, allModels...); err != nil {
		return err
	}
	resolved := tools.ResolveWire(t, ep, wire)
	fmt.Printf("Added and activated %s profile %q (%s · %s · wire=%s)\n", t.Title, *name, ep, *model, resolved)
	return nil
}

func cmdEdit(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon edit <tool> <profile> [--endpoint --key --model --name --wire]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	name := args[1]
	sp, ok := store.GetSpec(t.Name, name)
	if !ok {
		return fmt.Errorf("profile %q has no editable endpoint/key (captured or default profile)", name)
	}
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	endpoint := fs.String("endpoint", sp.Endpoint, "API base URL")
	key := fs.String("key", sp.Key, "API key")
	keyEnv := fs.String("key-env", "", "read API key from this environment variable (e.g. GEMINI_API_KEY)")
	model := fs.String("model", sp.Model, "model id")
	newName := fs.String("name", "", "rename the profile")
	wireFlag := fs.String("wire", sp.Wire, "protocol: openai, anthropic, or auto (OpenCode)")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	target := name
	if *newName != "" {
		target = *newName
	}
	if strings.TrimSpace(*keyEnv) != "" {
		resolvedKey, err := resolveAPIKey("", *keyEnv)
		if err != nil {
			return err
		}
		*key = resolvedKey
	}
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	wire, err := tools.NormalizeWire(*wireFlag)
	if err != nil {
		return err
	}
	ep := t.ResolveEndpoint(*endpoint)
	if t.Name == "opencode" && tools.ResolveWire(t, ep, wire) == tools.WireAnthropic {
		ep = tools.NormalizeAnthropicBaseURL(ep)
	}
	noteWireHint(t, ep, wire)
	allModels := discoverModels(t, ep, *key, wire)
	if err := store.EditProfile(t, name, target, profile.Spec{Endpoint: ep, Key: *key, Model: *model, Wire: wire}, allModels...); err != nil {
		return err
	}
	resolved := tools.ResolveWire(t, ep, wire)
	fmt.Printf("Updated %s profile %q (%s · %s · wire=%s)\n", t.Title, target, ep, *model, resolved)
	return nil
}

func cmdRename(store *profile.Store, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: charon rename <tool> <old> <new>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	if err := store.Rename(t.Name, args[1], args[2]); err != nil {
		return err
	}
	fmt.Printf("Renamed %s profile %q → %q\n", t.Title, args[1], args[2])
	return nil
}

func cmdDuplicate(store *profile.Store, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: charon cp <tool> <src> <dst>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	if err := store.Duplicate(t.Name, args[1], args[2]); err != nil {
		return err
	}
	fmt.Printf("Copied %s profile %q → %q\n", t.Title, args[1], args[2])
	return nil
}

func cmdRemove(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon rm <tool> <profile>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	if store.Active(t.Name) == args[1] {
		if _, err := store.Apply(t, profile.DefaultName); err != nil {
			return err
		}
	}
	if err := store.Remove(t.Name, args[1]); err != nil {
		return err
	}
	fmt.Printf("Removed profile %q for %s\n", args[1], t.Title)
	return nil
}

func cmdCompletion(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon completion [bash|zsh|fish]")
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("unsupported shell %q (want bash, zsh, or fish)", args[0])
	}
	return nil
}

// cmdProfiles prints one profile name per line for a tool; used by shell completion.
func cmdProfiles(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return nil
	}
	t := tools.Find(args[0])
	if t == nil {
		return nil
	}
	for _, name := range store.List(t.Name) {
		fmt.Println(name)
	}
	return nil
}

// cmdUninstall removes the running charon binary.
func cmdUninstall() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate running binary: %w", err)
	}
	fmt.Printf("Removing charon binary at %s ...\n", exe)
	if err := os.Remove(exe); err != nil {
		return fmt.Errorf("failed to remove binary: %w. Try running with sudo if needed", err)
	}
	fmt.Println("Charon binary uninstalled successfully.")
	fmt.Println("Note: Your profile configurations at ~/.config/charon remain intact.")
	fmt.Println("To completely remove them, run: rm -rf ~/.config/charon")
	return nil
}

// defaultUpdateInstallURL is this fork's release install script. Upstream
// mingtheanlay/charon is intentionally not used so `charon update` matches the
// binary built from this repository's releases.
const defaultUpdateInstallURL = "https://github.com/devwork2454/charon/releases/latest/download/install.sh"

// updateInstallURL returns the install.sh URL used by `charon update`.
// Override with CHARON_UPDATE_URL for mirrors or local testing.
func updateInstallURL() string {
	if u := strings.TrimSpace(os.Getenv("CHARON_UPDATE_URL")); u != "" {
		return u
	}
	return defaultUpdateInstallURL
}

// cmdUpdate runs the online install.sh script to upgrade the binary from this
// fork's GitHub releases (or CHARON_UPDATE_URL when set).
func cmdUpdate() error {
	url := updateInstallURL()
	fmt.Printf("Checking for updates and upgrading charon from\n  %s\n", url)
	// #nosec G204 -- URL is either our constant or an explicit operator override.
	cmd := exec.Command("sh", "-c", "curl -fsSL "+shellQuote(url)+" | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	return nil
}

// shellQuote wraps s in single quotes for a POSIX shell, escaping embedded quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
