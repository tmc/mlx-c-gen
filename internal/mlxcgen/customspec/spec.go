package customspec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/apilock"
	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

var validKinds = map[string]bool{
	"enum":     true,
	"function": true,
	"macro":    true,
	"struct":   true,
	"typedef":  true,
}

var validActions = map[string]bool{
	"custom_spec": true,
	"handwritten": true,
}

// Spec records one custom C API surface.
type Spec struct {
	SchemaVersion int    `yaml:"schema_version" json:"schema_version"`
	Name          string `yaml:"name" json:"name"`
	Target        string `yaml:"target" json:"target"`
	Header        string `yaml:"header" json:"header"`
	Ownership     string `yaml:"ownership" json:"ownership"`
	Items         []Item `yaml:"items" json:"items"`
}

// Item records one custom declaration decision.
type Item struct {
	Kind   string `yaml:"kind" json:"kind"`
	Name   string `yaml:"name" json:"name"`
	Action string `yaml:"action" json:"action"`
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// Load reads one custom spec.
func Load(r io.Reader) (Spec, error) {
	var spec Spec
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("parse custom spec: %w", err)
	}
	if err := spec.validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// LoadFile reads a custom spec file.
func LoadFile(path string) (Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return Spec{}, fmt.Errorf("open custom spec: %w", err)
	}
	defer f.Close()
	spec, err := Load(f)
	if err != nil {
		return Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	return spec, nil
}

// LoadDir reads all custom spec files from dir. An empty dir means no specs.
func LoadDir(dir string) ([]Spec, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read custom spec dir: %w", err)
	}
	var specs []Spec
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		spec, err := LoadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	sortSpecs(specs)
	return specs, nil
}

// CheckLock verifies specs exactly account for their locked headers.
func CheckLock(lock *apilock.Lock, specs []Spec) error {
	var problems []string
	for _, spec := range specs {
		target, ok := lock.Targets[spec.Target]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: unknown target %q", spec.Name, spec.Target))
			continue
		}
		want := lockedItems(target, spec.Header)
		got := specItems(spec)
		for key := range want {
			if !got[key] {
				problems = append(problems, fmt.Sprintf("%s: missing custom spec item %s", spec.Name, key))
			}
		}
		for key := range got {
			if !want[key] {
				problems = append(problems, fmt.Sprintf("%s: custom spec item %s is not in API lock", spec.Name, key))
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("custom spec check failed:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

func (s Spec) validate() error {
	var problems []string
	if s.SchemaVersion != SchemaVersion {
		problems = append(problems, fmt.Sprintf("schema_version = %d, want %d", s.SchemaVersion, SchemaVersion))
	}
	if s.Name == "" {
		problems = append(problems, "missing name")
	}
	if s.Target == "" {
		problems = append(problems, "missing target")
	}
	if s.Header == "" {
		problems = append(problems, "missing header")
	}
	if s.Ownership == "" {
		problems = append(problems, "missing ownership")
	}
	seen := map[string]bool{}
	for i, item := range s.Items {
		prefix := fmt.Sprintf("items[%d]", i)
		if item.Kind == "" {
			problems = append(problems, prefix+": missing kind")
		} else if !validKinds[item.Kind] {
			problems = append(problems, prefix+": unknown kind "+item.Kind)
		}
		if item.Name == "" {
			problems = append(problems, prefix+": missing name")
		}
		if item.Action == "" {
			problems = append(problems, prefix+": missing action")
		} else if !validActions[item.Action] {
			problems = append(problems, prefix+": unknown action "+item.Action)
		}
		key := item.Kind + ":" + item.Name
		if seen[key] {
			problems = append(problems, prefix+": duplicate "+key)
		}
		seen[key] = true
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("invalid custom spec:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

func lockedItems(target apilock.Target, header string) map[string]bool {
	out := map[string]bool{}
	for _, decl := range target.Macros {
		if decl.Header == header {
			out["macro:"+decl.Name] = true
		}
	}
	for _, decl := range target.Typedefs {
		if decl.Header == header {
			out["typedef:"+decl.Name] = true
		}
	}
	for _, decl := range target.Structs {
		if decl.Header == header {
			out["struct:"+decl.Name] = true
		}
	}
	for _, decl := range target.Enums {
		if decl.Header == header {
			out["enum:"+decl.Name] = true
		}
	}
	for _, decl := range target.Functions {
		if decl.Header == header {
			out["function:"+decl.Name] = true
		}
	}
	return out
}

func specItems(spec Spec) map[string]bool {
	out := map[string]bool{}
	for _, item := range spec.Items {
		out[item.Kind+":"+item.Name] = true
	}
	return out
}

func sortSpecs(specs []Spec) {
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Target != specs[j].Target {
			return specs[i].Target < specs[j].Target
		}
		if specs[i].Header != specs[j].Header {
			return specs[i].Header < specs[j].Header
		}
		return specs[i].Name < specs[j].Name
	})
	for i := range specs {
		sort.Slice(specs[i].Items, func(a, b int) bool {
			if specs[i].Items[a].Kind != specs[i].Items[b].Kind {
				return specs[i].Items[a].Kind < specs[i].Items[b].Kind
			}
			return specs[i].Items[a].Name < specs[i].Items[b].Name
		})
	}
}
