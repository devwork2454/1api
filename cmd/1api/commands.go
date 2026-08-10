package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"1api/internal/models"
	"1api/internal/profile"
	"1api/internal/provider"
	"1api/internal/secret"
	"1api/internal/tools"
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
	Provider string `json:"provider,omitempty"`
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
			r.Provider = store.ActiveProvider(t.Name)
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
	fmt.Fprintln(w, "TOOL\tACTIVE\tPROVIDER\tAUTH\tENDPOINT\tMODEL\tEFFORT\tSECRET")
	for _, r := range rows {
		if !r.Detected {
			fmt.Fprintf(w, "%s\t—\t—\t(not detected)\t\t\t\t\n", r.Title)
			continue
		}
		active := r.Active
		if active == "" {
			active = "—"
		}
		if r.Modified {
			active += " (modified)" // live config changed since the last switch
		}
		prov := r.Provider
		if prov == "" {
			prov = "—"
		}
		model, effort := r.Model, r.Effort
		if model == "" {
			model = "—"
		}
		if effort == "" {
			effort = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Title, active, prov, r.AuthMode, r.Endpoint, model, effort, r.Secret)
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
		return fmt.Errorf("usage: 1api ls <tool> [--json]")
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
		return fmt.Errorf("usage: 1api switch <tool> <profile>")
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
		return fmt.Errorf("usage: 1api save <tool> [name] [--label ..] [--note ..]")
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
		return fmt.Errorf("usage: 1api refresh <tool>")
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
		return fmt.Errorf("usage: 1api undo <tool>")
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
		return fmt.Errorf("usage: 1api prune <tool> [--keep N]")
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

func cmdModels(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api models <tool> --key <key> [--endpoint <url>] [--no-probe]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	noProbe := fs.Bool("no-probe", false, "list only (skip per-model chat probe)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	ep := t.ResolveEndpoint(*endpoint)
	var list []string
	if *noProbe {
		list, err = models.Fetch(models.Provider(t.Provider), ep, *key)
	} else {
		list, err = models.FilterReachable(models.Provider(t.Provider), ep, *key, models.FilterOptions{})
	}
	if err != nil {
		return err
	}
	for _, m := range list {
		fmt.Println(m)
	}
	return nil
}

func cmdVerify(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api verify <tool> [--endpoint --key --model]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL (default: live config)")
	key := fs.String("key", "", "API key (default: live config)")
	model := fs.String("model", "", "primary model id (default: live config)")
	asJSON := fs.Bool("json", false, "machine-readable JSON output")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	ep, k, primary := strings.TrimSpace(*endpoint), strings.TrimSpace(*key), strings.TrimSpace(*model)
	if (ep == "" || k == "" || primary == "") && t.Describe != nil {
		info, derr := t.Describe()
		if derr != nil {
			return derr
		}
		if ep == "" {
			ep = info.Endpoint
		}
		if k == "" {
			k = info.Secret
		}
		if primary == "" {
			primary = info.Model
		}
	}
	if strings.Contains(ep, "(default)") {
		ep = t.DefaultEndpoint
	}
	if err := tools.ValidateEndpoint(ep); err != nil {
		return err
	}
	if err := tools.ValidateKey(k); err != nil {
		return err
	}
	ep = t.ResolveEndpoint(ep)

	if t.Name == "opencode" {
		tiers, reach, verr := tools.VerifyOpenCodeAuth(ep, k, primary, nil)
		if verr != nil {
			return verr
		}
		if *asJSON {
			return printJSON(map[string]any{
				"ok":        true,
				"endpoint":  ep,
				"primary":   primary,
				"mid":       tiers.Mid,
				"low":       tiers.Low,
				"high":      tiers.High,
				"reachable": len(reach),
				"models":    reach,
			})
		}
		fmt.Printf("OK  %s\n", t.Title)
		fmt.Printf("  endpoint  %s\n", ep)
		fmt.Printf("  mid       %s\n", tiers.Mid)
		fmt.Printf("  low       %s\n", tiers.Low)
		fmt.Printf("  high      %s\n", tiers.High)
		fmt.Printf("  usable    %d model(s)\n", len(reach))
		for _, id := range reach {
			fmt.Printf("    %s\n", id)
		}
		return nil
	}

	reach, err := models.FilterReachable(models.Provider(t.Provider), ep, k, models.FilterOptions{})
	if err != nil {
		return err
	}
	if primary != "" {
		found := false
		for _, id := range reach {
			if id == primary {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("primary model %q is not usable", primary)
		}
	}
	if *asJSON {
		return printJSON(map[string]any{
			"ok":        true,
			"endpoint":  ep,
			"primary":   primary,
			"reachable": len(reach),
			"models":    reach,
		})
	}
	fmt.Printf("OK  %s\n", t.Title)
	fmt.Printf("  endpoint  %s\n", ep)
	fmt.Printf("  usable    %d model(s)\n", len(reach))
	for _, id := range reach {
		fmt.Printf("    %s\n", id)
	}
	return nil
}

func cmdAdd(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api add <tool> --name <p> --key <k> [--endpoint <url>] [--model <m>] [--no-verify]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	model := fs.String("model", "", "model id")
	name := fs.String("name", "", "profile name")
	noVerify := fs.Bool("no-verify", false, "skip live connectivity probe (OpenCode)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if t.ApplyAuth == nil {
		return fmt.Errorf("%s does not support add", t.Title)
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	ep := t.ResolveEndpoint(*endpoint)
	if err := store.UpsertProviderAndBind(t, *name, provider.Spec{
		Endpoint: ep,
		Key:      *key,
		Wire:     t.Provider,
		Model:    *model,
	}, *noVerify); err != nil {
		return err
	}
	if err := store.SaveWithSpec(t, *name, profile.Spec{
		Endpoint: ep,
		Key:      *key,
		Model:    *model,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: provider bound but profile snapshot: %v\n", err)
	} else {
		_ = store.SetActiveName(t.Name, *name)
	}
	fmt.Printf("Added provider %q and bound %s (%s · %s)\n", *name, t.Title, ep, *model)
	return nil
}

func cmdEdit(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: 1api edit <tool> <profile> [--endpoint --key --model --name] [--no-verify]")
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
	// Flags default to the current values, so an unset flag leaves that field unchanged.
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	endpoint := fs.String("endpoint", sp.Endpoint, "API base URL")
	key := fs.String("key", sp.Key, "API key")
	model := fs.String("model", sp.Model, "model id")
	newName := fs.String("name", "", "rename the profile")
	noVerify := fs.Bool("no-verify", false, "skip live connectivity probe (OpenCode)")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	target := name
	if *newName != "" {
		target = *newName
	}
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	if err := store.EditProfile(t, name, target, profile.Spec{
		Endpoint:   *endpoint,
		Key:        *key,
		Model:      *model,
		SkipVerify: *noVerify,
	}); err != nil {
		return err
	}
	fmt.Printf("Updated %s profile %q (%s · %s)\n", t.Title, target, t.ResolveEndpoint(*endpoint), *model)
	return nil
}

func cmdRename(store *profile.Store, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: 1api rename <tool> <old> <new>")
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
		return fmt.Errorf("usage: 1api cp <tool> <src> <dst>")
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
		return fmt.Errorf("usage: 1api rm <tool> <profile>")
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
		return fmt.Errorf("usage: 1api completion [bash|zsh|fish]")
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

// cmdUninstall removes the running 1api binary.
func cmdUninstall() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate running binary: %w", err)
	}
	fmt.Printf("Removing 1api binary at %s ...\n", exe)
	if err := os.Remove(exe); err != nil {
		return fmt.Errorf("failed to remove binary: %w. Try running with sudo if needed", err)
	}
	fmt.Println("1API binary uninstalled successfully.")
	fmt.Println("Note: Your profile configurations at ~/.config/1api remain intact.")
	fmt.Println("To completely remove them, run: rm -rf ~/.config/1api")
	return nil
}

const (
	defaultUpdateInstallURL = "https://github.com/devwork2454/1api/releases/latest/download/install.sh"
	// Gitee has no stable /latest/download redirect; resolve tag via API first.
	giteeOwnerRepo       = "wbff/1api"
	giteeReleasesAPI     = "https://gitee.com/api/v5/repos/" + giteeOwnerRepo + "/releases/latest"
	giteeReleaseDownload = "https://gitee.com/" + giteeOwnerRepo + "/releases/download"
)

// updateInstallURL returns the primary install.sh URL for `1api update`.
// Override with CHARON_UPDATE_URL for mirrors or local testing.
func updateInstallURL() string {
	if u := strings.TrimSpace(os.Getenv("CHARON_UPDATE_URL")); u != "" {
		return u
	}
	return defaultUpdateInstallURL
}

// giteeInstallFetchScript is a POSIX fragment: resolve latest Gitee release tag
// and curl that tag's install.sh. Gitee does not reliably support GitHub-style
// .../releases/latest/download/install.sh.
func giteeInstallFetchScript() string {
	// sed extracts "tag_name":"v…" without jq; fails closed if missing.
	return `tag=$(curl -fsSL ` + shellQuote(giteeReleasesAPI) + ` | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1) && ` +
		`test -n "$tag" && curl -fsSL ` + shellQuote(giteeReleaseDownload) + `/"$tag"/install.sh`
}

// cmdUpdate upgrades the binary via online install.sh: GitHub first, then Gitee.
func cmdUpdate() error {
	if u := strings.TrimSpace(os.Getenv("CHARON_UPDATE_URL")); u != "" {
		fmt.Printf("Checking for updates and upgrading 1api from\n  %s\n", u)
		// #nosec G204 -- explicit operator override.
		cmd := exec.Command("sh", "-c", "curl -fsSL "+shellQuote(u)+" | sh")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
		return nil
	}

	fmt.Printf("Checking for updates and upgrading 1api from\n  %s\n", defaultUpdateInstallURL)
	// #nosec G204 -- fixed GitHub release URL.
	cmd := exec.Command("sh", "-c", "curl -fsSL "+shellQuote(defaultUpdateInstallURL)+" | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	ghErr := cmd.Run()
	if ghErr == nil {
		return nil
	}
	fmt.Printf("GitHub update failed: %v\nTrying Gitee mirror %s …\n", ghErr, giteeOwnerRepo)

	// #nosec G204 -- fixed Gitee API + release download hosts.
	cmd = exec.Command("sh", "-c", giteeInstallFetchScript()+" | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update failed on GitHub and Gitee: %w", err)
	}
	return nil
}

// shellQuote wraps s in single quotes for a POSIX shell, escaping embedded quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
