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

var validTargets = map[string]bool{
	"jacclc": true,
	"mlxc":   true,
}

var validOwnership = map[string]bool{
	"custom_spec_generated": true,
	"handwritten_runtime":   true,
}

// Spec records one custom C API surface.
type Spec struct {
	SchemaVersion int          `yaml:"schema_version" json:"schema_version"`
	Name          string       `yaml:"name" json:"name"`
	Target        string       `yaml:"target" json:"target"`
	Header        string       `yaml:"header" json:"header"`
	Ownership     string       `yaml:"ownership" json:"ownership"`
	Generate      GenerateSpec `yaml:"generate,omitempty" json:"generate,omitempty"`
	Copyright     string       `yaml:"copyright,omitempty" json:"copyright,omitempty"`
	IncludeGuard  string       `yaml:"include_guard,omitempty" json:"include_guard,omitempty"`
	Includes      []string     `yaml:"includes,omitempty" json:"includes,omitempty"`
	Group         Group        `yaml:"group,omitempty" json:"group,omitempty"`
	Items         []Item       `yaml:"items" json:"items"`
}

// GenerateSpec records which custom artifacts are generated from a spec.
type GenerateSpec struct {
	Header bool `yaml:"header,omitempty" json:"header,omitempty"`
}

// Group records optional Doxygen group metadata for a generated header.
type Group struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	Doc   string `yaml:"doc,omitempty" json:"doc,omitempty"`
}

// Item records one custom declaration decision.
type Item struct {
	Kind      string      `yaml:"kind" json:"kind"`
	Name      string      `yaml:"name" json:"name"`
	Action    string      `yaml:"action" json:"action"`
	Reason    string      `yaml:"reason,omitempty" json:"reason,omitempty"`
	Doc       string      `yaml:"doc,omitempty" json:"doc,omitempty"`
	Signature string      `yaml:"signature,omitempty" json:"signature,omitempty"`
	Opaque    bool        `yaml:"opaque,omitempty" json:"opaque,omitempty"`
	Values    []EnumValue `yaml:"values,omitempty" json:"values,omitempty"`
}

// EnumValue records one custom enum constant.
type EnumValue struct {
	Name  string `yaml:"name" json:"name"`
	Value int    `yaml:"value" json:"value"`
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

// GeneratedHeaders returns generated header paths declared by specs.
func GeneratedHeaders(specs []Spec) []string {
	var headers []string
	for _, spec := range specs {
		if spec.Generate.Header {
			headers = append(headers, filepath.ToSlash(spec.Header))
		}
	}
	sort.Strings(headers)
	return headers
}

// CheckLock verifies specs exactly account for their locked headers.
func CheckLock(lock *apilock.Lock, specs []Spec) error {
	var problems []string
	for _, spec := range specs {
		if err := spec.validate(); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", spec.Name, err))
			continue
		}
		target, ok := lock.Targets[spec.Target]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: unknown target %q", spec.Name, spec.Target))
			continue
		}
		want := lockedItems(target, spec.Header)
		got := specItems(spec)
		for key := range want {
			if _, ok := got[key]; !ok {
				problems = append(problems, fmt.Sprintf("%s: missing custom spec item %s", spec.Name, key))
			}
		}
		for key := range got {
			if _, ok := want[key]; !ok {
				problems = append(problems, fmt.Sprintf("%s: custom spec item %s is not in API lock", spec.Name, key))
			}
		}
		for key, item := range got {
			locked, ok := want[key]
			if !ok {
				continue
			}
			problems = append(problems, compareLockedItem(spec.Name, item, locked)...)
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
	} else if !validTargets[s.Target] {
		problems = append(problems, "unknown target "+s.Target)
	}
	if s.Header == "" {
		problems = append(problems, "missing header")
	} else if !validHeaderPath(s.Header) {
		problems = append(problems, fmt.Sprintf("header %q must be under mlx/c", s.Header))
	}
	if s.Ownership == "" {
		problems = append(problems, "missing ownership")
	} else if !validOwnership[s.Ownership] {
		problems = append(problems, "unknown ownership "+s.Ownership)
	}
	if s.Generate.Header {
		if s.Copyright == "" {
			problems = append(problems, "missing copyright")
		}
		if s.IncludeGuard == "" {
			problems = append(problems, "missing include_guard")
		} else if !validCIdentifier(s.IncludeGuard) {
			problems = append(problems, "include_guard "+s.IncludeGuard+" is not a valid C identifier")
		}
		if s.Group.Name == "" {
			problems = append(problems, "missing group name")
		} else if !validCIdentifier(s.Group.Name) {
			problems = append(problems, "group name "+s.Group.Name+" is not a valid C identifier")
		}
		if s.Group.Title == "" {
			problems = append(problems, "missing group title")
		} else if !validDocText(s.Group.Title) {
			problems = append(problems, "group title contains invalid comment text")
		}
		if s.Group.Doc == "" {
			problems = append(problems, "missing group doc")
		} else if !validDocText(s.Group.Doc) {
			problems = append(problems, "group doc contains invalid comment text")
		}
		for i, include := range s.Includes {
			if !validInclude(include) {
				problems = append(problems, fmt.Sprintf("includes[%d] %q is not a valid header include", i, include))
			}
		}
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
		} else if !validCIdentifier(item.Name) {
			problems = append(problems, prefix+": name "+item.Name+" is not a valid C identifier")
		}
		if item.Action == "" {
			problems = append(problems, prefix+": missing action")
		} else if !validActions[item.Action] {
			problems = append(problems, prefix+": unknown action "+item.Action)
		}
		if item.Reason == "" {
			problems = append(problems, prefix+": missing reason")
		}
		if s.Generate.Header && item.Doc == "" {
			problems = append(problems, prefix+": missing doc")
		} else if s.Generate.Header && !validDocText(item.Doc) {
			problems = append(problems, prefix+": doc contains invalid comment text")
		}
		switch item.Kind {
		case "enum":
			if item.Signature != "" {
				problems = append(problems, prefix+": enum must not have signature")
			}
			if len(item.Values) == 0 {
				problems = append(problems, prefix+": missing enum values")
			}
			for j, value := range item.Values {
				if value.Name == "" {
					problems = append(problems, fmt.Sprintf("%s.values[%d]: missing name", prefix, j))
				} else if !validCIdentifier(value.Name) {
					problems = append(problems, fmt.Sprintf("%s.values[%d]: name %s is not a valid C identifier", prefix, j, value.Name))
				}
			}
		case "function":
			if item.Signature == "" {
				problems = append(problems, prefix+": missing signature")
			}
			if len(item.Values) > 0 {
				problems = append(problems, prefix+": function must not have enum values")
			}
		default:
			if item.Signature != "" {
				problems = append(problems, prefix+": "+item.Kind+" must not have signature")
			}
			if len(item.Values) > 0 {
				problems = append(problems, prefix+": "+item.Kind+" must not have enum values")
			}
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

func validHeaderPath(header string) bool {
	rel, ok := strings.CutPrefix(header, "mlx/c/")
	if !ok || rel == "" {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	return clean != "." &&
		clean != ".." &&
		!strings.HasPrefix(clean, ".."+string(os.PathSeparator)) &&
		!filepath.IsAbs(clean) &&
		filepath.ToSlash(clean) == rel
}

func validInclude(include string) bool {
	if include == "" || strings.TrimSpace(include) != include {
		return false
	}
	if strings.ContainsAny(include, "\"'<>\\") || strings.ContainsAny(include, " \t\r\n") {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(include))
	return clean != "." &&
		clean != ".." &&
		!strings.HasPrefix(clean, ".."+string(os.PathSeparator)) &&
		!filepath.IsAbs(clean) &&
		filepath.ToSlash(clean) == include
}

func validCIdentifier(name string) bool {
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return name != ""
}

func validDocText(text string) bool {
	return !strings.Contains(text, "*/")
}

type lockedItem struct {
	Kind      string
	Name      string
	Signature string
	Opaque    bool
	Values    []EnumValue
}

func lockedItems(target apilock.Target, header string) map[string]lockedItem {
	out := map[string]lockedItem{}
	for _, decl := range target.Macros {
		if decl.Header == header {
			out["macro:"+decl.Name] = lockedItem{
				Kind: "macro",
				Name: decl.Name,
			}
		}
	}
	for _, decl := range target.Typedefs {
		if decl.Header == header {
			out["typedef:"+decl.Name] = lockedItem{
				Kind: "typedef",
				Name: decl.Name,
			}
		}
	}
	for _, decl := range target.Structs {
		if decl.Header == header {
			out["struct:"+decl.Name] = lockedItem{
				Kind:   "struct",
				Name:   decl.Name,
				Opaque: decl.Opaque,
			}
		}
	}
	for _, decl := range target.Enums {
		if decl.Header == header {
			item := lockedItem{
				Kind: "enum",
				Name: decl.Name,
			}
			for _, value := range decl.Values {
				item.Values = append(item.Values, EnumValue{
					Name:  value.Name,
					Value: value.Value,
				})
			}
			out["enum:"+decl.Name] = item
		}
	}
	for _, decl := range target.Functions {
		if decl.Header == header {
			out["function:"+decl.Name] = lockedItem{
				Kind:      "function",
				Name:      decl.Name,
				Signature: decl.Signature,
			}
		}
	}
	return out
}

func specItems(spec Spec) map[string]Item {
	out := map[string]Item{}
	for _, item := range spec.Items {
		out[item.Kind+":"+item.Name] = item
	}
	return out
}

func compareLockedItem(specName string, item Item, locked lockedItem) []string {
	var problems []string
	key := item.Kind + ":" + item.Name
	switch item.Kind {
	case "enum":
		if !equalEnumValues(item.Values, locked.Values) {
			problems = append(problems, fmt.Sprintf("%s: custom spec item %s enum values differ from API lock", specName, key))
		}
	case "function":
		if item.Signature != locked.Signature {
			problems = append(problems, fmt.Sprintf("%s: custom spec item %s signature = %q, want %q", specName, key, item.Signature, locked.Signature))
		}
	case "struct":
		if item.Opaque != locked.Opaque {
			problems = append(problems, fmt.Sprintf("%s: custom spec item %s opaque = %v, want %v", specName, key, item.Opaque, locked.Opaque))
		}
	}
	return problems
}

func equalEnumValues(a, b []EnumValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
}
