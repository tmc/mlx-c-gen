package symbols

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/apilock"
)

// TargetLibrary maps one lock target to a built library path.
type TargetLibrary struct {
	Target string
	Path   string
}

// Options controls symbol checking.
type Options struct {
	LockPath string
	NM       string
	Targets  []TargetLibrary
}

// Check verifies built library symbols against the API lock.
func Check(opts Options) error {
	if opts.LockPath == "" {
		return fmt.Errorf("missing lock path")
	}
	if opts.NM == "" {
		opts.NM = "nm"
	}
	data, err := os.ReadFile(opts.LockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", opts.LockPath, err)
	}
	var lock apilock.Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("parse %s: %w", opts.LockPath, err)
	}
	if len(opts.Targets) == 0 {
		return fmt.Errorf("no target libraries provided")
	}

	var problems []string
	for _, tl := range opts.Targets {
		target, ok := lock.Targets[tl.Target]
		if !ok {
			problems = append(problems, fmt.Sprintf("unknown target %q", tl.Target))
			continue
		}
		syms, err := definedSymbols(opts.NM, tl.Path)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		problems = append(problems, checkTarget(tl.Target, target, syms)...)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("symbol check failed:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

func checkTarget(name string, target apilock.Target, syms map[string]bool) []string {
	var problems []string
	for _, fn := range target.Functions {
		if !hasSymbol(syms, fn.Name) {
			problems = append(problems, fmt.Sprintf("%s: missing %s", name, fn.Name))
		}
	}
	for sym := range syms {
		canon := canonicalName(sym)
		switch name {
		case "mlxc":
			if strings.HasPrefix(canon, "mlx_jaccl_") {
				problems = append(problems, fmt.Sprintf("mlxc: forbidden JACCL symbol %s", sym))
			}
		case "jacclc":
			if isPublicCAPISymbol(canon) && !strings.HasPrefix(canon, "mlx_jaccl_") {
				problems = append(problems, fmt.Sprintf("jacclc: forbidden non-JACCL C API symbol %s", sym))
			}
		}
	}
	return problems
}

func definedSymbols(nm, path string) (map[string]bool, error) {
	cmd := exec.Command(nm, "-g", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("run nm on %s: %s", path, msg)
	}
	return ParseNM(out), nil
}

// ParseNM returns the defined symbol names from nm output.
func ParseNM(out []byte) map[string]bool {
	syms := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) > 1 && fields[len(fields)-2] == "U" {
			continue
		}
		name := fields[len(fields)-1]
		if name == "" || strings.HasSuffix(name, ":") {
			continue
		}
		syms[name] = true
	}
	return syms
}

func hasSymbol(syms map[string]bool, want string) bool {
	for _, name := range normalizedNames(want) {
		if syms[name] {
			return true
		}
	}
	for sym := range syms {
		if canonicalName(sym) == want {
			return true
		}
	}
	return false
}

func normalizedNames(name string) []string {
	if strings.HasPrefix(name, "_") {
		return []string{name, strings.TrimPrefix(name, "_")}
	}
	return []string{name, "_" + name}
}

func canonicalName(name string) string {
	if strings.HasPrefix(name, "__mlx_") {
		return strings.TrimPrefix(name, "_")
	}
	if strings.HasPrefix(name, "_mlx_") {
		return strings.TrimPrefix(name, "_")
	}
	return name
}

func isPublicCAPISymbol(name string) bool {
	return strings.HasPrefix(name, "mlx_") || strings.HasPrefix(name, "_mlx_")
}
