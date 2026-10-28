package sandbox

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func seccompHelperTestPath(t *testing.T) (string, string, elf.Machine) {
	t.Helper()
	architecture, machine := "x64", elf.EM_X86_64
	if runtime.GOARCH == "arm64" {
		architecture, machine = "arm64", elf.EM_AARCH64
	} else if runtime.GOARCH != "amd64" {
		t.Skip("SRT ships seccomp helpers for amd64 and arm64 only")
	}
	root := t.TempDir()
	helper := filepath.Join(root, "vendor", "seccomp", architecture, "apply-seccomp")
	if err := os.MkdirAll(filepath.Dir(helper), 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "dist", "cli.js"), helper, machine
}

func writeSeccompTestELF(t *testing.T, path string, machine elf.Machine, mode os.FileMode) {
	t.Helper()
	header := make([]byte, 64)
	copy(header, "\x7fELF")
	header[4], header[5], header[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(header[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(header[18:], uint16(machine))
	binary.LittleEndian.PutUint32(header[20:], 1)
	binary.LittleEndian.PutUint16(header[52:], 64)
	if err := os.WriteFile(path, header, mode); err != nil {
		t.Fatal(err)
	}
}

func TestSeccompPreflightRequiresThePackagedArchitecture(t *testing.T) {
	cli, helper, machine := seccompHelperTestPath(t)
	if _, err := validateSRTSeccompHelper(cli); err == nil || !strings.Contains(err.Error(), "reinstall the complete runtime") {
		t.Fatalf("missing packaged helper: %v", err)
	}
	writeSeccompTestELF(t, helper, machine, 0o700)
	if got, err := validateSRTSeccompHelper(cli); err != nil || got != helper {
		t.Fatalf("matching helper = %q, %v", got, err)
	}
	wrong := elf.EM_X86_64
	if machine == wrong {
		wrong = elf.EM_AARCH64
	}
	writeSeccompTestELF(t, helper, wrong, 0o700)
	if _, err := validateSRTSeccompHelper(cli); err == nil || !strings.Contains(err.Error(), "reinstall the matching runtime") {
		t.Fatalf("wrong-architecture helper: %v", err)
	}
}

func TestSeccompPreflightRejectsNonExecutableAndInvalidHelpers(t *testing.T) {
	cli, helper, machine := seccompHelperTestPath(t)
	writeSeccompTestELF(t, helper, machine, 0o600)
	if _, err := validateSRTSeccompHelper(cli); err == nil || !strings.Contains(err.Error(), "regular executable") {
		t.Fatalf("non-executable helper: %v", err)
	}
	if err := os.Chmod(helper, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSRTSeccompHelper(cli); err == nil || !strings.Contains(err.Error(), "read sandbox-runtime seccomp helper") {
		t.Fatalf("script replacing ELF helper: %v", err)
	}
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(helper, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSRTSeccompHelper(cli); err == nil || !strings.Contains(err.Error(), "regular executable") {
		t.Fatalf("directory replacing helper: %v", err)
	}
}

func TestSandboxStatusRequiresNoNewPrivilegesAndActiveFiltering(t *testing.T) {
	for _, status := range []string{
		"",
		"NoNewPrivs:\t0\nSeccomp:\t2\n",
		"NoNewPrivs:\t1\nSeccomp:\t0\n",
		"NoNewPrivs:\t1\nSeccomp:\t1\n",
		"NoNewPrivs:\t1\nSeccomp_filters:\t2\n",
	} {
		if err := checkSandboxStatus(status); err == nil {
			t.Fatalf("accepted missing or weaker process restrictions: %q", status)
		}
	}
	if err := checkSandboxStatus("Name:\tbob-manager\nNoNewPrivs:\t1\nSeccomp:\t2\nSeccomp_filters:\t3\n"); err != nil {
		t.Fatal(err)
	}
}
