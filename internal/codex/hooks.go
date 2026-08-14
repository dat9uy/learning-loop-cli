package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// hooksDoc is the parsed .codex/hooks.json document. Unknown keys, matcher
// groups, and handlers are preserved verbatim, and numbers round-trip
// without precision loss.
type hooksDoc struct {
	raw map[string]any
}

type groupState int

const (
	groupUnrelated groupState = iota
	groupCurrent
	groupOlder
	groupModified
)

func hooksPath(projectRoot string) string {
	return filepath.Join(projectRoot, HooksDirName, HooksFileName)
}

func loadHooksDoc(projectRoot string) (*hooksDoc, error) {
	path := hooksPath(projectRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, &Error{Code: "E204", Msg: fmt.Sprintf("reading %s: %v", path, err)}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, &Error{Code: "E202", Msg: fmt.Sprintf("malformed %s: %v", path, err)}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, &Error{Code: "E202", Msg: fmt.Sprintf("malformed %s: trailing content", path)}
	}
	if raw == nil {
		raw = map[string]any{}
	}
	if err := validateDoc(raw); err != nil {
		return nil, err
	}
	return &hooksDoc{raw: raw}, nil
}

// validateDoc rejects structurally invalid native configuration so that a
// failed installation can never make it worse.
func validateDoc(raw map[string]any) error {
	hooks, ok := raw["hooks"]
	if !ok {
		return nil
	}
	hm, ok := hooks.(map[string]any)
	if !ok {
		return malformed("hooks must be an object")
	}
	for name, groups := range hm {
		gs, ok := groups.([]any)
		if !ok {
			return malformed(fmt.Sprintf("hooks.%s must be an array", name))
		}
		for _, g := range gs {
			gm, ok := g.(map[string]any)
			if !ok {
				return malformed(fmt.Sprintf("hooks.%s group must be an object", name))
			}
			hs, ok := gm["hooks"].([]any)
			if !ok {
				return malformed(fmt.Sprintf("hooks.%s group must contain a hooks array", name))
			}
			for _, h := range hs {
				if _, ok := h.(map[string]any); !ok {
					return malformed(fmt.Sprintf("hooks.%s handler must be an object", name))
				}
			}
		}
	}
	return nil
}

func malformed(msg string) error {
	return &Error{Code: "E202", Msg: "malformed hooks.json: " + msg}
}

// findLearningLoopGroup returns the index and state of the recognized
// learning-loop SessionStart group, or groupUnrelated when none exists. Any
// modified or unknown learning-loop content wins over a recognized group.
func (d *hooksDoc) findLearningLoopGroup() (int, groupState) {
	found, state := -1, groupUnrelated
	for i, g := range d.sessionStartGroups() {
		gm, _ := g.(map[string]any)
		switch s := classifyGroup(gm); s {
		case groupModified:
			return i, groupModified
		case groupCurrent, groupOlder:
			if found < 0 {
				found, state = i, s
			}
		}
	}
	return found, state
}

func (d *hooksDoc) sessionStartGroups() []any {
	hooks, _ := d.raw["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	groups, _ := hooks["SessionStart"].([]any)
	return groups
}

func (d *hooksDoc) addGroup(g map[string]any) {
	hooks := d.hooksMap()
	groups, _ := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(groups, g)
}

func (d *hooksDoc) replaceGroup(i int, g map[string]any) {
	hooks := d.hooksMap()
	groups := hooks["SessionStart"].([]any)
	groups[i] = g
}

func (d *hooksDoc) removeGroup(i int) {
	hooks := d.hooksMap()
	groups := hooks["SessionStart"].([]any)
	hooks["SessionStart"] = append(groups[:i], groups[i+1:]...)
	if len(hooks["SessionStart"].([]any)) == 0 {
		delete(hooks, "SessionStart")
	}
	if len(hooks) == 0 {
		delete(d.raw, "hooks")
	}
}

func (d *hooksDoc) hooksMap() map[string]any {
	hooks, _ := d.raw["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		d.raw["hooks"] = hooks
	}
	return hooks
}

// classifyGroup recognizes exactly the current and older learning-loop group
// shapes. A group whose handler mentions learning-loop without matching a
// recognized shape is modified or unknown learning-loop content.
func classifyGroup(g map[string]any) groupState {
	switch canonical(g) {
	case canonical(currentGroup()):
		return groupCurrent
	case canonical(olderGroup()):
		return groupOlder
	}
	if mentionsLearningLoop(g) {
		return groupModified
	}
	return groupUnrelated
}

func mentionsLearningLoop(g map[string]any) bool {
	hs, _ := g["hooks"].([]any)
	for _, h := range hs {
		hm, _ := h.(map[string]any)
		if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "learning-loop") {
			return true
		}
	}
	return false
}

func canonical(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func currentGroup() map[string]any {
	return map[string]any{
		"hooks": []any{
			map[string]any{
				"type":          "command",
				"command":       handlerCommand,
				"statusMessage": handlerStatusMessage,
			},
		},
	}
}

func olderGroup() map[string]any {
	return map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": handlerCommand,
			},
		},
	}
}

// writeHooksDoc writes the document atomically, preserving the existing
// file's permissions.
func writeHooksDoc(projectRoot string, doc *hooksDoc) error {
	dir := filepath.Join(projectRoot, HooksDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("creating %s: %v", dir, err)}
	}
	path := hooksPath(projectRoot)
	data, err := json.MarshalIndent(doc.raw, "", "  ")
	if err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("encoding %s: %v", path, err)}
	}
	data = append(data, '\n')
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".hooks.json-*")
	if err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	if err := tmp.Close(); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return &Error{Code: "E204", Msg: fmt.Sprintf("writing %s: %v", path, err)}
	}
	return nil
}
