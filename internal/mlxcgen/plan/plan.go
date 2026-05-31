package plan

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/inventory"
	"gopkg.in/yaml.v3"
)

const (
	defaultManifestPath = "codegen/manifest.yaml"
	SchemaVersion       = 1
)

// HeaderMapping defines a header-derived binding output.
type HeaderMapping struct {
	Name        string   `yaml:"name"`
	Headers     []string `yaml:"headers"`
	Docstring   string   `yaml:"doc"`
	PreIncludes []string `yaml:"pre_includes,omitempty"`
}

// Manifest describes the generator output plan.
type Manifest struct {
	SchemaVersion          int                             `yaml:"schema_version"`
	MLX                    MLXPolicy                       `yaml:"mlx,omitempty"`
	Report                 ReportPolicy                    `yaml:"report,omitempty"`
	GeneratedMarkers       GeneratedMarkerPolicy           `yaml:"generated_markers,omitempty"`
	Headers                []HeaderMapping                 `yaml:"headers"`
	Standalone             []string                        `yaml:"standalone"`
	VariantMappings        map[string]map[string][]Variant `yaml:"variant_mappings,omitempty"`
	AllowedDetailFunctions []string                        `yaml:"allowed_detail_functions,omitempty"`
}

// MLXPolicy records the upstream MLX revision the manifest was reviewed
// against.
type MLXPolicy struct {
	ExpectedGitRef string `yaml:"expected_git_ref,omitempty" json:"expected_git_ref,omitempty"`
}

// ReportPolicy records the report gates expected for this manifest.
type ReportPolicy struct {
	RequireCleanGenerated bool `yaml:"require_clean_generated,omitempty" json:"require_clean_generated,omitempty"`
	RequireAPILock        bool `yaml:"require_api_lock,omitempty" json:"require_api_lock,omitempty"`
	IncludeInventory      bool `yaml:"include_inventory,omitempty" json:"include_inventory,omitempty"`
}

// GeneratedMarkerPolicy records generated marker invariants for this manifest.
type GeneratedMarkerPolicy struct {
	ForbidVolatileData bool `yaml:"forbid_volatile_data,omitempty" json:"forbid_volatile_data,omitempty"`
}

// Variant defines one overload selection rule.
type Variant struct {
	Signature string  `yaml:"signature"`
	Suffix    *string `yaml:"suffix,omitempty"`
	Skip      bool    `yaml:"skip,omitempty"`
}

// Load reads a generator plan manifest.
func Load(r io.Reader) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("parse plan manifest: %w", err)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Default returns the repository generator output plan.
func Default() (Manifest, error) {
	path, err := findDefaultManifest()
	if err != nil {
		return Manifest{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open default plan manifest: %w", err)
	}
	defer f.Close()
	return Load(f)
}

// HeaderMappings returns the current header-derived binding plan.
func HeaderMappings() ([]HeaderMapping, error) {
	m, err := Default()
	if err != nil {
		return nil, err
	}
	return copyHeaderMappings(m.Headers), nil
}

// StandaloneNames returns the current standalone binding plan.
func StandaloneNames() ([]string, error) {
	m, err := Default()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), m.Standalone...), nil
}

// VariantMappings returns the current overload variant selection policy.
func VariantMappings() (map[string]map[string][]Variant, error) {
	m, err := Default()
	if err != nil {
		return nil, err
	}
	return copyVariantMappings(m.VariantMappings), nil
}

// AllowedDetailFunctions returns the current detail namespace allowlist.
func AllowedDetailFunctions() (map[string]bool, error) {
	m, err := Default()
	if err != nil {
		return nil, err
	}
	return m.AllowedDetailFunctionsSet(), nil
}

// GeneratedOutputs returns all files currently produced by the Go generator.
func GeneratedOutputs() ([]string, error) {
	m, err := Default()
	if err != nil {
		return nil, err
	}
	return m.GeneratedOutputs(), nil
}

// GeneratedOutputs returns all files produced by m.
func (m Manifest) GeneratedOutputs() []string {
	var out []string
	for _, hm := range m.Headers {
		out = append(out,
			"mlx/c/"+hm.Name+".h",
			"mlx/c/"+hm.Name+".cpp",
		)
	}
	for _, name := range m.Standalone {
		out = append(out,
			"mlx/c/"+name+".h",
			"mlx/c/"+name+".cpp",
			"mlx/c/private/"+name+".h",
		)
	}
	sort.Strings(out)
	return out
}

// AllowedDetailFunctionsSet returns the detail namespace allowlist as a set.
func (m Manifest) AllowedDetailFunctionsSet() map[string]bool {
	out := map[string]bool{}
	for _, name := range m.AllowedDetailFunctions {
		out[name] = true
	}
	return out
}

// CheckInventory verifies that generated inventory entries match the plan.
func CheckInventory(entries []inventory.Entry) error {
	outputs, err := GeneratedOutputs()
	if err != nil {
		return err
	}
	planned := map[string]bool{}
	for _, out := range outputs {
		planned[out] = true
	}
	entriesByPath := map[string]inventory.Entry{}
	for _, entry := range entries {
		entriesByPath[entry.Path] = entry
	}

	var problems []string
	for path := range planned {
		entry, ok := entriesByPath[path]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is generated by plan but missing from inventory", path))
			continue
		}
		if entry.Kind != "generated_header_api" && entry.Kind != "generated_support" {
			problems = append(problems, fmt.Sprintf("%s inventory kind = %s, want generated_header_api or generated_support", path, entry.Kind))
		}
	}
	for _, entry := range entries {
		if entry.Kind != "generated_header_api" && entry.Kind != "generated_support" {
			continue
		}
		if !planned[entry.Path] {
			problems = append(problems, fmt.Sprintf("%s is %s but is not generated by the Go plan", entry.Path, entry.Kind))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("plan check failed:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

func (m Manifest) validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("plan manifest schema_version = %d, want %d", m.SchemaVersion, SchemaVersion)
	}
	if m.MLX.ExpectedGitRef == "" {
		return fmt.Errorf("plan manifest has empty mlx expected_git_ref")
	}
	if len(m.Headers) == 0 {
		return fmt.Errorf("plan manifest has no header mappings")
	}
	if len(m.Standalone) == 0 {
		return fmt.Errorf("plan manifest has no standalone generators")
	}
	headerNames := map[string]bool{}
	for _, hm := range m.Headers {
		if hm.Name == "" {
			return fmt.Errorf("plan manifest has header mapping with empty name")
		}
		if headerNames[hm.Name] {
			return fmt.Errorf("plan manifest has duplicate header mapping %q", hm.Name)
		}
		headerNames[hm.Name] = true
		if len(hm.Headers) == 0 {
			return fmt.Errorf("plan manifest header mapping %q has no headers", hm.Name)
		}
		for _, header := range hm.Headers {
			if header == "" {
				return fmt.Errorf("plan manifest header mapping %q has empty header", hm.Name)
			}
		}
		for _, header := range hm.PreIncludes {
			if header == "" {
				return fmt.Errorf("plan manifest header mapping %q has empty pre-include", hm.Name)
			}
		}
	}
	standaloneNames := map[string]bool{}
	for _, name := range m.Standalone {
		if name == "" {
			return fmt.Errorf("plan manifest has empty standalone generator")
		}
		if standaloneNames[name] {
			return fmt.Errorf("plan manifest has duplicate standalone generator %q", name)
		}
		standaloneNames[name] = true
	}
	for namespace, funcs := range m.VariantMappings {
		if namespace == "" {
			return fmt.Errorf("plan manifest has variant mapping with empty namespace")
		}
		if len(funcs) == 0 {
			return fmt.Errorf("plan manifest variant namespace %q has no functions", namespace)
		}
		for name, variants := range funcs {
			if name == "" {
				return fmt.Errorf("plan manifest variant namespace %q has empty function name", namespace)
			}
			if len(variants) == 0 {
				return fmt.Errorf("plan manifest variant mapping %q.%s has no entries", namespace, name)
			}
			seenSignatures := map[string]bool{}
			for _, variant := range variants {
				if variant.Signature == "" {
					return fmt.Errorf("plan manifest variant mapping %q.%s has empty signature", namespace, name)
				}
				if seenSignatures[variant.Signature] {
					return fmt.Errorf("plan manifest variant mapping %q.%s has duplicate signature %q", namespace, name, variant.Signature)
				}
				seenSignatures[variant.Signature] = true
				if variant.Skip == (variant.Suffix != nil) {
					return fmt.Errorf("plan manifest variant mapping %q.%s signature %q must set exactly one of suffix or skip", namespace, name, variant.Signature)
				}
			}
		}
	}
	allowedDetail := map[string]bool{}
	for _, name := range m.AllowedDetailFunctions {
		if name == "" {
			return fmt.Errorf("plan manifest has empty allowed detail function")
		}
		if allowedDetail[name] {
			return fmt.Errorf("plan manifest has duplicate allowed detail function %q", name)
		}
		allowedDetail[name] = true
	}
	return nil
}

func copyHeaderMappings(in []HeaderMapping) []HeaderMapping {
	out := make([]HeaderMapping, len(in))
	for i, hm := range in {
		out[i] = HeaderMapping{
			Name:        hm.Name,
			Headers:     append([]string(nil), hm.Headers...),
			Docstring:   hm.Docstring,
			PreIncludes: append([]string(nil), hm.PreIncludes...),
		}
	}
	return out
}

func copyVariantMappings(in map[string]map[string][]Variant) map[string]map[string][]Variant {
	out := make(map[string]map[string][]Variant, len(in))
	for namespace, funcs := range in {
		out[namespace] = make(map[string][]Variant, len(funcs))
		for name, variants := range funcs {
			out[namespace][name] = copyVariants(variants)
		}
	}
	return out
}

func copyVariants(in []Variant) []Variant {
	out := make([]Variant, len(in))
	for i, variant := range in {
		out[i] = Variant{
			Signature: variant.Signature,
			Skip:      variant.Skip,
		}
		if variant.Suffix != nil {
			s := *variant.Suffix
			out[i].Suffix = &s
		}
	}
	return out
}

func findDefaultManifest() (string, error) {
	if path, ok := findUp(".", defaultManifestPath); ok {
		return path, nil
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		if path, ok := findUp(filepath.Dir(file), defaultManifestPath); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("find default plan manifest %s", defaultManifestPath)
}

func findUp(start, rel string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(dir, rel)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", false
		}
		dir = next
	}
}
