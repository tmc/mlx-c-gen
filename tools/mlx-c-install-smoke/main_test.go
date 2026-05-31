package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordingRunner struct {
	calls []commandCall
}

type commandCall struct {
	name string
	args []string
}

func (r *recordingRunner) Run(name string, args ...string) error {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
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
		BuildDir: buildDir,
		Consumer: consumer,
		Prefix:   prefix,
		WorkDir:  workDir,
		CMake:    "cmake",
	}, r)
	if err != nil {
		t.Fatalf("runSmoke: %v", err)
	}
	want := []commandCall{
		{name: "cmake", args: []string{"--install", buildDir, "--prefix", prefix}},
		{name: "cmake", args: []string{
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
