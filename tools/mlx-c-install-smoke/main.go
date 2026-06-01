// Command mlx-c-install-smoke verifies an installed MLX C package.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type options struct {
	BuildDir               string
	Consumer               string
	Prefix                 string
	WorkDir                string
	CMake                  string
	Generator              string
	KeepWork               bool
	SkipRun                bool
	ExpectConfigureFailure bool
	ExpectConfigureError   string
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
	fs.StringVar(&opts.Generator, "generator", "", "CMake generator for the consumer build")
	fs.BoolVar(&opts.KeepWork, "keep-work", false, "keep an automatically-created scratch directory")
	fs.BoolVar(&opts.SkipRun, "skip-run", false, "build the consumer but do not run it")
	fs.BoolVar(&opts.ExpectConfigureFailure, "expect-configure-failure", false, "expect consumer CMake configure to fail")
	fs.StringVar(&opts.ExpectConfigureError, "expect-configure-error", "", "substring required in an expected configure failure")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if opts.BuildDir == "" {
		return options{}, fmt.Errorf("missing -build-dir")
	}
	if opts.ExpectConfigureError != "" && !opts.ExpectConfigureFailure {
		return options{}, fmt.Errorf("-expect-configure-error requires -expect-configure-failure")
	}
	return opts, nil
}

type runner interface {
	Run(name string, args ...string) (string, error)
}

type commandRunner struct{}

func (commandRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		if _, writeErr := os.Stdout.Write(out); writeErr != nil {
			return string(out), fmt.Errorf("write command output: %w", writeErr)
		}
	}
	if err != nil {
		return string(out), fmt.Errorf("%s %v: %w", name, args, err)
	}
	return string(out), nil
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

	if _, err := r.Run(opts.CMake, "--install", opts.BuildDir, "--prefix", prefix); err != nil {
		return err
	}
	configureArgs := []string{
		"-S", opts.Consumer,
		"-B", consumerBuild,
		"-DCMAKE_BUILD_TYPE=Release",
		"-DCMAKE_PREFIX_PATH=" + prefix,
	}
	if opts.Generator != "" {
		configureArgs = append([]string{"-G", opts.Generator}, configureArgs...)
	}
	configureOut, err := r.Run(opts.CMake, configureArgs...)
	if opts.ExpectConfigureFailure {
		if err == nil {
			return fmt.Errorf("consumer configure succeeded, want failure")
		}
		if opts.ExpectConfigureError != "" && !strings.Contains(configureOut, opts.ExpectConfigureError) {
			return fmt.Errorf("consumer configure output missing %q", opts.ExpectConfigureError)
		}
		return nil
	} else if err != nil {
		return err
	}
	if _, err := r.Run(opts.CMake, "--build", consumerBuild, "-j"); err != nil {
		return err
	}
	if opts.SkipRun {
		return nil
	}
	_, err = r.Run(consumerPath(consumerBuild))
	return err
}

func consumerPath(buildDir string) string {
	name := "consumer"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(buildDir, name)
}
