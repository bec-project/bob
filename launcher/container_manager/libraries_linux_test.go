package containermanager

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedLibrariesOfADynamicProgram(t *testing.T) {
	libraries, err := sharedLibraries("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	var loader, libc bool
	for _, library := range libraries {
		if strings.HasPrefix(filepath.Base(library.path), "ld-linux") {
			loader = true
		}
		if filepath.Base(library.path) == "libc.so.6" {
			libc = true
		}
		if !filepath.IsAbs(library.file) {
			t.Fatalf("library %+v has a relative host file", library)
		}
	}
	if !loader || !libc {
		t.Fatalf("libraries %+v lack the loader or libc", libraries)
	}
}
