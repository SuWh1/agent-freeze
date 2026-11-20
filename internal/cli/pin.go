package cli

import (
	"fmt"

	"github.com/DataMata2/agent-freeze/internal/auto"
	"github.com/DataMata2/agent-freeze/internal/freezer"
)

// pinResult is one target's outcome in pin and unpin --json.
type pinResult struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Pinned bool   `json:"pinned"`
	Error  string `json:"error,omitempty"`
}

// pinCmd sets the pin state of the selected groups in the auto config, the same file the TUI writes.
func pinCmd(pin bool, sels []string, api freezer.API, o *output) error {
	verb := "unpin"
	if pin {
		verb = "pin"
	}
	if len(sels) == 0 {
		return usagef("%s: give at least one selector", verb)
	}
	groups, err := api.List()
	if err != nil {
		return err
	}
	targets, err := selectTargets(groups, sels)
	if err != nil {
		return err
	}
	saveErr := setPins(auto.DefaultPath(), targets, pin)
	results := make([]pinResult, 0, len(targets))
	for _, g := range targets {
		r := pinResult{ID: g.ID, Key: g.Key, Name: g.Name, OK: saveErr == nil, Pinned: pin && saveErr == nil}
		if saveErr != nil {
			r.Error = saveErr.Error()
			o.code = 1
		}
		results = append(results, r)
	}
	o.pins(pin, results)
	return nil
}

// setPins loads the config at path, pins or unpins every target and saves it back.
func setPins(path string, targets []freezer.GroupView, pin bool) error {
	cfg, err := auto.LoadConfig(path)
	if err != nil {
		return err
	}
	for _, g := range targets {
		cfg = cfg.SetPinned(g, pin)
	}
	return auto.SaveConfig(path, cfg)
}

func (o *output) pins(pin bool, results []pinResult) {
	if o.json {
		o.writeJSON(struct {
			Results []pinResult `json:"results"`
		}{results})
		return
	}
	for _, r := range results {
		switch {
		case !r.OK:
			fmt.Fprintf(o.stdout, "failed %s (%s): %s\n", r.Name, r.ID, r.Error)
		case pin:
			fmt.Fprintf(o.stdout, "pinned %s (%s)\n", r.Name, r.ID)
		default:
			fmt.Fprintf(o.stdout, "unpinned %s (%s)\n", r.Name, r.ID)
		}
	}
}
