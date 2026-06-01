package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordingRunner struct {
	calls []commandCall
	fail  map[int]bool
}

type commandCall struct {
	name string
	args []string
}

func (r *recordingRunner) Run(name string, args ...string) error {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	if r.fail != nil && r.fail[len(r.calls)] {
		return fmt.Errorf("command failed")
	}
	return nil
}

func TestParseOptionsRequiresBuildDir(t *testing.T) {
	_, err := parseOptions(nil)
	if err == nil || !strings.Contains(err.Error(), "missing -build-dir") {
		t.Fatalf("parseOptions = %v, want missing build dir", err)
	}
}

func TestRunSmokeCommands(t *testing.T) {
	r := &recordingRunner{}
	workDir := t.TempDir()
	buildDir := filepath.Join(workDir, "build")
	prefix := filepath.Join(workDir, "install")
	consumer := filepath.Join(workDir, "consumer")
	consumerBuild := filepath.Join(workDir, "consumer-build")
	err := runSmoke(options{
		BuildDir:  buildDir,
		Consumer:  consumer,
		Prefix:    prefix,
		WorkDir:   workDir,
		CMake:     "cmake",
		Generator: "Ninja",
	}, r)
	if err != nil {
		t.Fatalf("runSmoke: %v", err)
	}
	want := []commandCall{
		{name: "cmake", args: []string{"--install", buildDir, "--prefix", prefix}},
		{name: "cmake", args: []string{
			"-G", "Ninja",
			"-S", consumer,
			"-B", consumerBuild,
			"-DCMAKE_BUILD_TYPE=Release",
			"-DCMAKE_PREFIX_PATH=" + prefix,
		}},
		{name: "cmake", args: []string{"--build", consumerBuild, "-j"}},
		{name: filepath.Join(consumerBuild, "consumer")},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls = %#v, want %#v", r.calls, want)
	}
}

func TestRunSmokeCanSkipRun(t *testing.T) {
	r := &recordingRunner{}
	workDir := t.TempDir()
	err := runSmoke(options{
		BuildDir: "/build",
		Consumer: "/consumer",
		Prefix:   "/install",
		WorkDir:  workDir,
		CMake:    "cmake",
		SkipRun:  true,
	}, r)
	if err != nil {
		t.Fatalf("runSmoke: %v", err)
	}
	if len(r.calls) != 3 {
		t.Fatalf("calls = %#v, want three cmake commands", r.calls)
	}
}

func TestRunSmokeCanExpectConfigureFailure(t *testing.T) {
	r := &recordingRunner{fail: map[int]bool{2: true}}
	workDir := t.TempDir()
	err := runSmoke(options{
		BuildDir:               "/build",
		Consumer:               "/consumer",
		Prefix:                 "/install",
		WorkDir:                workDir,
		CMake:                  "cmake",
		ExpectConfigureFailure: true,
	}, r)
	if err != nil {
		t.Fatalf("runSmoke: %v", err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("calls = %#v, want install and configure", r.calls)
	}
}

func TestRunSmokeExpectedConfigureFailureMustFail(t *testing.T) {
	r := &recordingRunner{}
	err := runSmoke(options{
		BuildDir:               "/build",
		Consumer:               "/consumer",
		Prefix:                 "/install",
		WorkDir:                t.TempDir(),
		CMake:                  "cmake",
		ExpectConfigureFailure: true,
	}, r)
	if err == nil || !strings.Contains(err.Error(), "consumer configure succeeded") {
		t.Fatalf("runSmoke = %v, want configure success error", err)
	}
}
