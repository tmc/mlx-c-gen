package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordingRunner struct {
	calls  []commandCall
	fail   map[int]bool
	output map[int]string
}

type commandCall struct {
	name string
	args []string
}

func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	out := ""
	if r.output != nil {
		out = r.output[len(r.calls)]
	}
	if r.fail != nil && r.fail[len(r.calls)] {
		return out, fmt.Errorf("command failed")
	}
	return out, nil
}

func TestParseOptionsRequiresBuildDir(t *testing.T) {
	_, err := parseOptions(nil)
	if err == nil || !strings.Contains(err.Error(), "missing -build-dir") {
		t.Fatalf("parseOptions = %v, want missing build dir", err)
	}
}

func TestParseOptionsRejectsConfigureErrorWithoutExpectedFailure(t *testing.T) {
	_, err := parseOptions([]string{
		"-build-dir", "/build",
		"-expect-configure-error", "missing JACCL",
	})
	if err == nil || !strings.Contains(err.Error(), "requires -expect-configure-failure") {
		t.Fatalf("parseOptions = %v, want configure error dependency", err)
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
	r := &recordingRunner{
		fail:   map[int]bool{2: true},
		output: map[int]string{2: "MLXC package was built without JACCL"},
	}
	workDir := t.TempDir()
	err := runSmoke(options{
		BuildDir:               "/build",
		Consumer:               "/consumer",
		Prefix:                 "/install",
		WorkDir:                workDir,
		CMake:                  "cmake",
		ExpectConfigureFailure: true,
		ExpectConfigureError:   "without JACCL",
	}, r)
	if err != nil {
		t.Fatalf("runSmoke: %v", err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("calls = %#v, want install and configure", r.calls)
	}
}

func TestRunSmokeExpectedConfigureFailureChecksOutput(t *testing.T) {
	r := &recordingRunner{
		fail:   map[int]bool{2: true},
		output: map[int]string{2: "some other configure failure"},
	}
	err := runSmoke(options{
		BuildDir:               "/build",
		Consumer:               "/consumer",
		Prefix:                 "/install",
		WorkDir:                t.TempDir(),
		CMake:                  "cmake",
		ExpectConfigureFailure: true,
		ExpectConfigureError:   "MLXC package was built without JACCL",
	}, r)
	if err == nil || !strings.Contains(err.Error(), "consumer configure output missing") {
		t.Fatalf("runSmoke = %v, want missing configure output error", err)
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
