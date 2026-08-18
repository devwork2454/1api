package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"1api/internal/profile"
	"1api/internal/provider"
	"1api/internal/secret"
	"1api/internal/tools"
)

func cmdProvider(store *profile.Store, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: 1api provider ls|add|edit|rm|verify|models")
	}
	switch args[0] {
	case "ls", "list":
		return cmdProviderList(store, args[1:])
	case "add":
		return cmdProviderAdd(store, args[1:])
	case "edit":
		return cmdProviderEdit(store, args[1:])
	case "rm", "remove":
		return cmdProviderRemove(store, args[1:])
	case "verify":
		return cmdProviderVerify(store, args[1:])
	case "models":
		return cmdProviderModels(store, args[1:])
	default:
		return fmt.Errorf("unknown provider subcommand %q", args[0])
	}
}

func cmdProviderList(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("provider ls", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ps, err := store.ProviderStore()
	if err != nil {
		return err
	}
	type row struct {
		Name     string `json:"name"`
		Endpoint string `json:"endpoint"`
		Wire     string `json:"wire"`
		Mid      string `json:"mid,omitempty"`
		Low      string `json:"low,omitempty"`
		High     string `json:"high,omitempty"`
		Usable   int    `json:"usable"`
		Secret   string `json:"secret,omitempty"`
		Stale    bool   `json:"needsVerify"`
	}
	var rows []row
	for _, name := range ps.List() {
		r, err := ps.Get(name)
		if err != nil {
			continue
		}
		rows = append(rows, row{
			Name: r.Name, Endpoint: r.Endpoint, Wire: r.Wire,
			Mid: r.Mid, Low: r.Low, High: r.High,
			Usable: len(r.Usable), Secret: secret.Mask(r.Key), Stale: r.NeedsVerify,
		})
	}
	if *asJSON {
		return printJSON(rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tWIRE\tENDPOINT\tMID\tLOW\tHIGH\tUSABLE\tSECRET")
	for _, r := range rows {
		mid, low, high := r.Mid, r.Low, r.High
		if mid == "" {
			mid = "—"
		}
		if low == "" {
			low = "—"
		}
		if high == "" {
			high = "—"
		}
		name := r.Name
		if r.Stale {
			name += " (stale)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", name, r.Wire, r.Endpoint, mid, low, high, r.Usable, r.Secret)
	}
	return w.Flush()
}

func cmdProviderAdd(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("provider add", flag.ContinueOnError)
	name := fs.String("name", "", "provider name")
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	wire := fs.String("wire", "", "openai or anthropic")
	model := fs.String("model", "", "primary/mid model id")
	low := fs.String("low", "", "low-tier model id")
	high := fs.String("high", "", "high-tier model id")
	preset := fs.String("preset", "", "built-in gateway preset: "+provider.PresetNames())
	noVerify := fs.Bool("no-verify", false, "skip connectivity probe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *preset != "" {
		if *endpoint != "" || *wire != "" {
			return fmt.Errorf("--preset cannot be combined with --endpoint/--wire (the preset fills both)")
		}
		p, err := provider.LookupPreset(*preset)
		if err != nil {
			return err
		}
		*endpoint = p.Endpoint
		*wire = p.Wire
	}
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	if err := store.UpsertProviderAndBind(nil, *name, provider.Spec{
		Endpoint: *endpoint,
		Key:      *key,
		Wire:     *wire,
		Model:    *model,
		Low:      *low,
		High:     *high,
	}, *noVerify); err != nil {
		return err
	}
	ps, err := store.ProviderStore()
	if err != nil {
		return err
	}
	r, err := ps.Get(*name)
	if err != nil {
		return err
	}
	fmt.Printf("Added provider %q (mid=%s low=%s high=%s usable=%d)\n", r.Name, r.Mid, r.Low, r.High, len(r.Usable))
	return nil
}

func cmdProviderEdit(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api provider edit <name> [--endpoint --key --model --low --high --wire] [--no-verify]")
	}
	name := args[0]
	ps, err := store.ProviderStore()
	if err != nil {
		return err
	}
	cur, err := ps.Get(name)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("provider edit", flag.ContinueOnError)
	endpoint := fs.String("endpoint", cur.Endpoint, "API base URL")
	key := fs.String("key", cur.Key, "API key")
	wire := fs.String("wire", cur.Wire, "openai or anthropic")
	model := fs.String("model", cur.Mid, "primary/mid model id")
	low := fs.String("low", cur.Low, "low-tier model id")
	high := fs.String("high", cur.High, "high-tier model id")
	noVerify := fs.Bool("no-verify", false, "skip connectivity probe")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := tools.ValidateKey(*key); err != nil {
		return err
	}
	if err := tools.ValidateEndpoint(*endpoint); err != nil {
		return err
	}
	r, err := ps.Upsert(provider.Spec{
		Name: name, Endpoint: *endpoint, Key: *key, Wire: *wire,
		Model: *model, Low: *low, High: *high, SkipVerify: *noVerify,
	}, provider.UpsertOptions{SkipVerify: *noVerify})
	if err != nil {
		return err
	}
	fmt.Printf("Updated provider %q (mid=%s low=%s high=%s usable=%d)\n", r.Name, r.Mid, r.Low, r.High, len(r.Usable))
	return nil
}

func cmdProviderRemove(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api provider rm <name>")
	}
	ps, err := store.ProviderStore()
	if err != nil {
		return err
	}
	if err := ps.Remove(args[0]); err != nil {
		return err
	}
	// Clear tool bindings pointing at this provider.
	for _, t := range tools.All() {
		if store.ActiveProvider(t.Name) == args[0] {
			_ = store.SetToolProvider(t.Name, "")
		}
	}
	fmt.Printf("Removed provider %q\n", args[0])
	return nil
}

func cmdProviderVerify(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api provider verify <name>")
	}
	ps, err := store.ProviderStore()
	if err != nil {
		return err
	}
	r, err := ps.Refresh(args[0], provider.UpsertOptions{})
	if err != nil {
		return err
	}
	fmt.Printf("OK  provider %s\n", r.Name)
	fmt.Printf("  endpoint  %s\n", r.Endpoint)
	fmt.Printf("  mid       %s\n", r.Mid)
	fmt.Printf("  low       %s\n", r.Low)
	fmt.Printf("  high      %s\n", r.High)
	fmt.Printf("  usable    %d model(s)\n", len(r.Usable))
	for _, id := range r.Usable {
		fmt.Printf("    %s\n", id)
	}
	return nil
}

func cmdProviderModels(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: 1api provider models <name>")
	}
	ps, err := store.ProviderStore()
	if err != nil {
		return err
	}
	r, err := ps.Get(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("mid=%s low=%s high=%s\n", r.Mid, r.Low, r.High)
	for _, id := range r.Usable {
		mark := ""
		switch id {
		case r.Mid:
			mark = " [mid]"
		case r.Low:
			mark = " [low]"
		case r.High:
			mark = " [high]"
		}
		tags := make([]string, 0, 3)
		if id == r.Mid {
			tags = append(tags, "mid")
		}
		if id == r.Low {
			tags = append(tags, "low")
		}
		if id == r.High {
			tags = append(tags, "high")
		}
		if len(tags) > 0 {
			mark = " [" + strings.Join(tags, ",") + "]"
		}
		fmt.Printf("%s%s\n", id, mark)
	}
	return nil
}

func cmdUse(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: 1api use <tool> <provider> [--no-verify]")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	noVerify := fs.Bool("no-verify", false, "skip connectivity probe")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	name := args[1]
	if err := store.ApplyProvider(t, name, *noVerify); err != nil {
		return err
	}
	fmt.Printf("Bound %s → provider %q\n", t.Title, name)
	return nil
}
