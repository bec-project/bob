package containermanager

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The sandbox does not see the host's /usr: of the system's files it gets only what the dynamic
// loader needs to start OpenCode, which is the loader itself and the libraries OpenCode names,
// with the libraries those name.

// libraryDirs are where the loader looks for a library without ld.so.cache, which the sandbox
// does not have, in its order.
var libraryDirs = []string{"/lib64", "/usr/lib64", "/lib", "/usr/lib"}

// sharedLibrary is a file the loader needs: where it looks for it, and the host file it is.
type sharedLibrary struct {
	path string
	file string
}

// sharedLibraries lists the loader and every library executable needs, transitively. A static
// executable, such as bob-manager, needs none.
func sharedLibraries(executable string) ([]sharedLibrary, error) {
	var libraries []sharedLibrary
	seen := map[string]bool{}
	pending := []string{}
	add := func(path string) error {
		if seen[path] {
			return nil
		}
		seen[path] = true
		file, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		libraries = append(libraries, sharedLibrary{path: path, file: file})
		pending = append(pending, file)
		return nil
	}
	interpreter, needed, err := elfDependencies(executable)
	if err != nil {
		return nil, err
	}
	if interpreter != "" {
		if err := add(interpreter); err != nil {
			return nil, fmt.Errorf("OpenCode's loader %s: %w", interpreter, err)
		}
	}
	for {
		for _, name := range needed {
			path, err := findLibrary(name)
			if err != nil {
				return nil, err
			}
			if err := add(path); err != nil {
				return nil, err
			}
		}
		if len(pending) == 0 {
			return libraries, nil
		}
		next := pending[0]
		pending = pending[1:]
		if _, needed, err = elfDependencies(next); err != nil {
			return nil, err
		}
	}
}

func elfDependencies(path string) (string, []string, error) {
	file, err := elf.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()
	interpreter := ""
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP {
			data := make([]byte, program.Filesz)
			if _, err := program.ReadAt(data, 0); err != nil {
				return "", nil, err
			}
			interpreter = string(bytes.TrimRight(data, "\x00"))
		}
	}
	needed, err := file.ImportedLibraries()
	if err != nil && !errors.Is(err, elf.ErrNoSymbols) {
		return "", nil, err
	}
	return interpreter, needed, nil
}

func findLibrary(name string) (string, error) {
	for _, directory := range libraryDirs {
		path := filepath.Join(directory, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("library %s, which OpenCode needs, is in none of %v", name, libraryDirs)
}
