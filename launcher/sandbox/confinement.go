package sandbox

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	confinementProbeCommand = "--confinement-probe"
	confinementProbeSuccess = "bob confinement verified\n"
)

// validateSRTSeccompHelper requires the packaged helper instead of SRT's optional global lookup
// and permissive missing-helper fallback. Bob supports the two architectures SRT ships helpers for.
func validateSRTSeccompHelper(srtCLI string) (string, error) {
	var architecture string
	var machine elf.Machine
	switch runtime.GOARCH {
	case "amd64":
		architecture, machine = "x64", elf.EM_X86_64
	case "arm64":
		architecture, machine = "arm64", elf.EM_AARCH64
	default:
		return "", fmt.Errorf("sandbox-runtime confinement is unsupported on %s; use Linux amd64 or arm64", runtime.GOARCH)
	}
	helper := filepath.Join(filepath.Dir(filepath.Dir(srtCLI)), "vendor", "seccomp", architecture, "apply-seccomp")
	info, err := os.Stat(helper)
	if err != nil {
		return "", fmt.Errorf("sandbox-runtime requires its %s seccomp helper at %s; reinstall the complete runtime: %w", architecture, helper, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("sandbox-runtime seccomp helper must be a regular executable file: %s", helper)
	}
	binary, err := elf.Open(helper)
	if err != nil {
		return "", fmt.Errorf("read sandbox-runtime seccomp helper %s: %w", helper, err)
	}
	defer binary.Close()
	if binary.Class != elf.ELFCLASS64 || binary.Data != elf.ELFDATA2LSB || binary.Machine != machine ||
		(binary.Type != elf.ET_EXEC && binary.Type != elf.ET_DYN) {
		return "", fmt.Errorf("sandbox-runtime seccomp helper %s is not a Linux %s executable; reinstall the matching runtime", helper, runtime.GOARCH)
	}
	return helper, nil
}

// checkSandboxStatus checks inherited process restrictions, not a particular filter count:
// unrelated outer filters must not be mistaken for SRT's protections, which the probe tests.
func checkSandboxStatus(status string) error {
	fields := make(map[string]string)
	for line := range strings.SplitSeq(status, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok {
			fields[name] = strings.TrimSpace(value)
		}
	}
	for _, required := range []struct{ name, value string }{{"NoNewPrivs", "1"}, {"Seccomp", "2"}} {
		if fields[required.name] != required.value {
			return fmt.Errorf("%s is %q, expected %s; refusing to start OpenCode without sandbox confinement", required.name, fields[required.name], required.value)
		}
	}
	return nil
}
