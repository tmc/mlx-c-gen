// Command mlx-c-install-smoke verifies an installed MLX C package.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type options struct {
	BuildDir string
	Consumer string
	Prefix   string
	WorkDir  string
	CMake    string
	KeepWork bool
	SkipRun  bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "mlx-c-install-smoke: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	return runSmoke(opts, commandRunner{})
}

func parseOptions(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("mlx-c-install-smoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.BuildDir, "build-dir", "", "CMake build directory to install")
	fs.StringVar(&opts.Consumer, "consumer", "codegen/smoke/cmake-consumer", "CMake consumer source directory")
	fs.StringVar(&opts.Prefix, "prefix", "", "install prefix; defaults to a temporary directory")
	fs.StringVar(&opts.WorkDir, "work-dir", "", "scratch directory for the consumer build")
	fs.StringVar(&opts.CMake, "cmake", "cmake", "cmake executable")
	fs.BoolVar(&opts.KeepWork, "keep-work", false, "keep an automatically-created scratch directory")
	fs.BoolVar(&opts.SkipRun, "skip-run", false, "build the consumer but do not run it")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if opts.BuildDir == "" {
		return options{}, fmt.Errorf("missing -build-dir")
	}
	return opts, nil
}

type runner interface {
	Run(name string, args ...string) error
}

type commandRunner struct{}

func (commandRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w", name, args, err)
	}
	return nil
}

func runSmoke(opts options, r runner) error {
	workDir := opts.WorkDir
	removeWork := false
	if workDir == "" {
		tmp, err := os.MkdirTemp("", "mlx-c-install-smoke-*")
		if err != nil {
			return fmt.Errorf("make work dir: %w", err)
		}
		workDir = tmp
		removeWork = !opts.KeepWork
	}
	if removeWork {
		defer os.RemoveAll(workDir)
	}
	if err := os.MkdirAll(workDir, 0o777); err != nil {
		return fmt.Errorf("make work dir: %w", err)
	}

	prefix := opts.Prefix
	if prefix == "" {
		prefix = filepath.Join(workDir, "install")
	}
	consumerBuild := filepath.Join(workDir, "consumer-build")

	if err := r.Run(opts.CMake, "--install", opts.BuildDir, "--prefix", prefix); err != nil {
		return err
	}
	if err := r.Run(opts.CMake,
		"-S", opts.Consumer,
		"-B", consumerBuild,
		"-DCMAKE_BUILD_TYPE=Release",
		"-DCMAKE_PREFIX_PATH="+prefix,
	); err != nil {
		return err
	}
	if err := r.Run(opts.CMake, "--build", consumerBuild, "-j"); err != nil {
		return err
	}
	if opts.SkipRun {
		return nil
	}
	return r.Run(consumerPath(consumerBuild))
}

func consumerPath(buildDir string) string {
	name := "consumer"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(buildDir, name)
}
