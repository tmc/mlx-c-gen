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
	"github.com/ml-explore/mlx-c/internal/mlxcgen/plan"
)

// Options controls a regeneration report run.
type Options struct {
	RepoRoot            string
	MLXSrc              string
	CompileCommandsPath string
	InventoryPath       string
	WorkDir             string
	Generator           []string
	NoFormat            bool
	KeepWork            bool
}

// Report records the result of a scratch-tree regeneration.
type Report struct {
	RepoRoot            string       `json:"repo_root"`
	MLXSrc              string       `json:"mlx_src"`
	CompileCommandsPath string       `json:"compile_commands_path,omitempty"`
	WorkDir             string       `json:"work_dir"`
	OutputDir           string       `json:"output_dir"`
	MetadataPath        string       `json:"metadata_path"`
	Command             []string     `json:"command"`
	GeneratorOut        string       `json:"generator_output,omitempty"`
	GeneratorErr        string       `json:"generator_error,omitempty"`
	Summary             Summary      `json:"summary"`
	Files               []FileReport `json:"files"`
	GeneratedOnly       []string     `json:"generated_only,omitempty"`
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
	if opts.MLXSrc == "" {
		return nil, fmt.Errorf("missing mlx source path")
	}
	if len(opts.Generator) == 0 {
		opts.Generator = []string{"go", "run", "./tools/mlx-c-gen"}
	}
	if err := checkInventory(opts.InventoryPath); err != nil {
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

	report, err := Compare(opts.RepoRoot, outputDir, plan.GeneratedOutputs())
	if err != nil {
		return nil, err
	}
	report.RepoRoot = opts.RepoRoot
	report.MLXSrc = opts.MLXSrc
	report.CompileCommandsPath = opts.CompileCommandsPath
	report.WorkDir = workDir
	report.OutputDir = outputDir
	report.MetadataPath = metadataPath
	report.Command = append([]string{opts.Generator[0]}, args...)
	report.GeneratorOut = string(out)
	return report, nil
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
	if opts.NoFormat {
		args = append(args, "--no-format")
	}
	return args
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

func checkInventory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	entries, err := inventory.Read(f)
	if err != nil {
		return err
	}
	return plan.CheckInventory(entries)
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
