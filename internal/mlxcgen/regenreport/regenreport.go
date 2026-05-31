package regenreport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ml-explore/mlx-c/internal/mlxcgen/inventory"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/ir"
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

// Options controls a regeneration report run.
type Options struct {
	RepoRoot            string
	MLXSrc              string
	CompileCommandsPath string
	InventoryPath       string
	WorkDir             string
	ASTCacheDir         string
	NoASTCache          bool
	Generator           []string
	NoFormat            bool
	KeepWork            bool
}

// Report records the result of a scratch-tree regeneration.
type Report struct {
	SchemaVersion       int          `json:"schema_version"`
	RepoRoot            string       `json:"repo_root"`
	MLXSrc              string       `json:"mlx_src"`
	MLXRevision         string       `json:"mlx_revision,omitempty"`
	ClangVersion        string       `json:"clang_version,omitempty"`
	CompileCommandsPath string       `json:"compile_commands_path,omitempty"`
	Manifest            ManifestInfo `json:"manifest"`
	Modules             []Module     `json:"modules,omitempty"`
	InventoryPath       string       `json:"inventory_path,omitempty"`
	Inventory           []Inventory  `json:"inventory,omitempty"`
	WorkDir             string       `json:"work_dir"`
	OutputDir           string       `json:"output_dir"`
	MetadataPath        string       `json:"metadata_path"`
	IR                  ir.Result    `json:"ir,omitempty"`
	Diagnostics         []Diagnostic `json:"diagnostics,omitempty"`
	Command             []string     `json:"command"`
	GeneratorOut        string       `json:"generator_output,omitempty"`
	GeneratorErr        string       `json:"generator_error,omitempty"`
	Summary             Summary      `json:"summary"`
	Files               []FileReport `json:"files"`
	GeneratedOnly       []string     `json:"generated_only,omitempty"`
}

// Module records one planned header-derived generator module.
type Module struct {
	Name    string   `json:"name"`
	Headers []string `json:"headers"`
	Outputs []string `json:"outputs"`
}

// ManifestInfo records review policy from the generator manifest.
type ManifestInfo struct {
	SchemaVersion    int                        `json:"schema_version"`
	MLX              plan.MLXPolicy             `json:"mlx,omitempty"`
	Report           plan.ReportPolicy          `json:"report,omitempty"`
	GeneratedMarkers plan.GeneratedMarkerPolicy `json:"generated_markers,omitempty"`
}

// Inventory records one generated-file inventory entry in the report.
type Inventory struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Path   string `json:"path"`
}

// Diagnostic records a generator diagnostic included in metadata.yaml.
type Diagnostic struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
	File    string `json:"file,omitempty" yaml:"file,omitempty"`
	Line    int    `json:"line,omitempty" yaml:"line,omitempty"`
	Col     int    `json:"col,omitempty" yaml:"col,omitempty"`
}

// Summary counts per-file comparison states.
type Summary struct {
	Equal            int `json:"equal"`
	Different        int `json:"different"`
	MissingGenerated int `json:"missing_generated"`
	MissingCheckedIn int `json:"missing_checked_in"`
}

// FileReport records the comparison for one planned generated file.
type FileReport struct {
	Path            string `json:"path"`
	Status          string `json:"status"`
	CheckedBytes    int64  `json:"checked_bytes,omitempty"`
	GeneratedBytes  int64  `json:"generated_bytes,omitempty"`
	CheckedSHA256   string `json:"checked_sha256,omitempty"`
	GeneratedSHA256 string `json:"generated_sha256,omitempty"`
}

// Run creates a scratch output tree, runs the generator, and compares outputs.
func Run(opts Options) (*Report, error) {
	if opts.RepoRoot == "" {
		opts.RepoRoot = "."
	}
	if opts.InventoryPath == "" {
		opts.InventoryPath = filepath.Join(opts.RepoRoot, "codegen", "generated-files.txt")
	}
	inventoryPath := repoPath(opts.RepoRoot, opts.InventoryPath)
	if opts.MLXSrc == "" {
		return nil, fmt.Errorf("missing mlx source path")
	}
	if len(opts.Generator) == 0 {
		opts.Generator = []string{"go", "run", "./tools/mlx-c-gen"}
	}
	manifest, err := plan.Default()
	if err != nil {
		return nil, err
	}
	if err := manifest.CheckCMakeMLXRef(opts.RepoRoot); err != nil {
		return nil, err
	}
	inventoryEntries, err := checkInventory(opts.RepoRoot, inventoryPath, manifest)
	if err != nil {
		return nil, err
	}

	workDir := opts.WorkDir
	if workDir == "" {
		tmp, err := os.MkdirTemp("", "mlx-c-regen-*")
		if err != nil {
			return nil, fmt.Errorf("make temp dir: %w", err)
		}
		workDir = tmp
	}
	if !opts.KeepWork && opts.WorkDir == "" {
		defer os.RemoveAll(workDir)
	}
	outputDir := filepath.Join(workDir, "mlx", "c")
	if err := os.MkdirAll(filepath.Join(outputDir, "private"), 0o777); err != nil {
		return nil, fmt.Errorf("make output dir: %w", err)
	}
	metadataPath := filepath.Join(workDir, "metadata.yaml")

	args := generatorArgs(opts, outputDir, metadataPath)
	cmd := exec.Command(opts.Generator[0], args...)
	cmd.Dir = opts.RepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run generator: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	outputs := manifest.GeneratedOutputs()
	modules := reportModules(manifest)
	clangVersion, err := commandOutputLine("clang++", "--version")
	if err != nil {
		clangVersion = ""
	}
	mlxRevision, err := commandOutput("git", "-C", opts.MLXSrc, "rev-parse", "HEAD")
	if err != nil {
		mlxRevision = ""
	}
	report, err := Compare(opts.RepoRoot, outputDir, outputs)
	if err != nil {
		return nil, err
	}
	metadata, err := readMetadata(metadataPath)
	if err != nil {
		return nil, err
	}
	report.SchemaVersion = SchemaVersion
	report.RepoRoot = opts.RepoRoot
	report.MLXSrc = opts.MLXSrc
	report.MLXRevision = mlxRevision
	report.ClangVersion = clangVersion
	report.CompileCommandsPath = opts.CompileCommandsPath
	report.Manifest = reportManifest(manifest)
	report.Modules = modules
	report.InventoryPath = inventoryPath
	report.Inventory = reportInventory(inventoryEntries)
	report.WorkDir = workDir
	report.OutputDir = outputDir
	report.MetadataPath = metadataPath
	report.IR = metadata.IR
	report.Diagnostics = metadata.Diagnostics
	report.Command = append([]string{opts.Generator[0]}, args...)
	report.GeneratorOut = string(out)
	return report, nil
}

func commandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func commandOutputLine(name string, args ...string) (string, error) {
	out, err := commandOutput(name, args...)
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(out, "\n")
	return line, nil
}

func reportModules(manifest plan.Manifest) []Module {
	modules := make([]Module, 0, len(manifest.Headers))
	for _, hm := range manifest.Headers {
		modules = append(modules, Module{
			Name:    hm.Name,
			Headers: append([]string(nil), hm.Headers...),
			Outputs: []string{
				"mlx/c/" + hm.Name + ".cpp",
				"mlx/c/" + hm.Name + ".h",
			},
		})
	}
	return modules
}

func reportManifest(manifest plan.Manifest) ManifestInfo {
	return ManifestInfo{
		SchemaVersion:    manifest.SchemaVersion,
		MLX:              manifest.MLX,
		Report:           manifest.Report,
		GeneratedMarkers: manifest.GeneratedMarkers,
	}
}

func reportInventory(entries []inventory.Entry) []Inventory {
	out := make([]Inventory, 0, len(entries))
	for _, entry := range entries {
		out = append(out, Inventory{
			Kind:   entry.Kind,
			Target: entry.Target,
			Path:   entry.Path,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func generatorArgs(opts Options, outputDir, metadataPath string) []string {
	args := append([]string{}, opts.Generator[1:]...)
	args = append(args,
		"--mlx-src", opts.MLXSrc,
		"--output-dir", outputDir,
		"--metadata", metadataPath,
	)
	if opts.CompileCommandsPath != "" {
		args = append(args, "--compile-commands", opts.CompileCommandsPath)
	}
	if opts.ASTCacheDir != "" {
		args = append(args, "--ast-cache", opts.ASTCacheDir)
	}
	if opts.NoASTCache {
		args = append(args, "--no-ast-cache")
	}
	if opts.NoFormat {
		args = append(args, "--no-format")
	}
	return args
}

type metadataReport struct {
	IR          ir.Result    `yaml:"ir"`
	Diagnostics []Diagnostic `yaml:"diagnostics"`
}

func readMetadata(path string) (metadataReport, error) {
	var meta metadataReport
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return meta, nil
	}
	if err != nil {
		return meta, fmt.Errorf("read metadata: %w", err)
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("parse metadata: %w", err)
	}
	meta.IR.Sort()
	return meta, nil
}

func readMetadataDiagnostics(path string) ([]Diagnostic, error) {
	meta, err := readMetadata(path)
	if err != nil {
		return nil, err
	}
	return meta.Diagnostics, nil
}

// Compare compares planned generator outputs in repoRoot and outputDir.
func Compare(repoRoot, outputDir string, outputs []string) (*Report, error) {
	report := &Report{}
	outputSet := map[string]bool{}
	for _, path := range outputs {
		outputSet[path] = true
		file, err := compareOne(repoRoot, outputDir, path)
		if err != nil {
			return nil, err
		}
		report.Files = append(report.Files, file)
		switch file.Status {
		case "equal":
			report.Summary.Equal++
		case "different":
			report.Summary.Different++
		case "missing_generated":
			report.Summary.MissingGenerated++
		case "missing_checked_in":
			report.Summary.MissingCheckedIn++
		}
	}
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].Path < report.Files[j].Path
	})
	generatedOnly, err := generatedOnlyFiles(outputDir, outputSet)
	if err != nil {
		return nil, err
	}
	report.GeneratedOnly = generatedOnly
	return report, nil
}

// JSON returns an indented JSON representation of r.
func (r *Report) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(data, '\n'), nil
}

// Clean reports whether all planned generated files match.
func (r *Report) Clean() bool {
	return r.Summary.Different == 0 &&
		r.Summary.MissingGenerated == 0 &&
		r.Summary.MissingCheckedIn == 0 &&
		len(r.GeneratedOnly) == 0
}

func checkInventory(root, path string, manifest plan.Manifest) ([]inventory.Entry, error) {
	if err := inventory.Check(root, path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	entries, err := inventory.Read(f)
	if err != nil {
		return nil, err
	}
	if err := manifest.CheckInventory(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func repoPath(root, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func compareOne(repoRoot, outputDir, path string) (FileReport, error) {
	rel := strings.TrimPrefix(path, "mlx/c/")
	checkedPath := filepath.Join(repoRoot, filepath.FromSlash(path))
	generatedPath := filepath.Join(outputDir, filepath.FromSlash(rel))

	checked, checkedErr := os.ReadFile(checkedPath)
	generated, generatedErr := os.ReadFile(generatedPath)

	file := FileReport{Path: path}
	checkedMissing := os.IsNotExist(checkedErr)
	generatedMissing := os.IsNotExist(generatedErr)
	if checkedErr != nil && !checkedMissing {
		return file, fmt.Errorf("read %s: %w", checkedPath, checkedErr)
	}
	if generatedErr != nil && !generatedMissing {
		return file, fmt.Errorf("read %s: %w", generatedPath, generatedErr)
	}
	if !checkedMissing {
		file.CheckedBytes = int64(len(checked))
		file.CheckedSHA256 = hash(checked)
	}
	if !generatedMissing {
		file.GeneratedBytes = int64(len(generated))
		file.GeneratedSHA256 = hash(generated)
	}
	switch {
	case checkedMissing:
		file.Status = "missing_checked_in"
	case generatedMissing:
		file.Status = "missing_generated"
	default:
		if bytes.Equal(checked, generated) {
			file.Status = "equal"
		} else {
			file.Status = "different"
		}
	}
	return file, nil
}

func generatedOnlyFiles(outputDir string, outputs map[string]bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".h" && ext != ".cpp" {
			return nil
		}
		rel, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}
		inventoryPath := "mlx/c/" + filepath.ToSlash(rel)
		if !outputs[inventoryPath] {
			out = append(out, inventoryPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk generated output: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
