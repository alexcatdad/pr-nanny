package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	winversion "github.com/tc-hib/winres/version"
)

var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func main() {
	out := flag.String("out", "rsrc_windows_amd64.syso", "output Windows resource object")
	root := flag.String("root", "../..", "repository root containing VERSION and build/windows")
	flag.Parse()
	if err := generate(*root, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(root, out string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	versionBytes, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return fmt.Errorf("read VERSION: %w", err)
	}
	appVersion := strings.TrimSuffix(string(versionBytes), "\n")
	parts := semver.FindStringSubmatch(appVersion)
	if parts == nil {
		return fmt.Errorf("VERSION must contain exactly one valid SemVer value")
	}
	var numbers [3]uint64
	for i := range numbers {
		numbers[i], err = strconv.ParseUint(parts[i+1], 10, 16)
		if err != nil {
			return fmt.Errorf("VERSION major, minor, and patch must fit Windows 16-bit version fields")
		}
	}
	fileVersion := fmt.Sprintf("%d.%d.%d.0", numbers[0], numbers[1], numbers[2])

	infoTemplate, err := os.ReadFile(filepath.Join(root, "build", "windows", "info.json"))
	if err != nil {
		return fmt.Errorf("read Windows version info: %w", err)
	}
	infoBytes := bytes.ReplaceAll(infoTemplate, []byte("@FILE_VERSION@"), []byte(fileVersion))
	infoBytes = bytes.ReplaceAll(infoBytes, []byte("@VERSION@"), []byte(appVersion))
	var versionInfo winversion.Info
	if err := json.Unmarshal(infoBytes, &versionInfo); err != nil {
		return fmt.Errorf("parse Windows version info: %w", err)
	}
	versionTable := versionInfo.Table()[winversion.LangDefault]
	if versionTable == nil || (*versionTable)[winversion.ProductVersion] != appVersion {
		return fmt.Errorf("Windows ProductVersion template did not resolve to VERSION")
	}

	manifestTemplate, err := os.ReadFile(filepath.Join(root, "build", "windows", "app.manifest"))
	if err != nil {
		return fmt.Errorf("read Windows application manifest: %w", err)
	}
	manifestBytes := bytes.ReplaceAll(manifestTemplate, []byte("@FILE_VERSION@"), []byte(fileVersion))
	if bytes.Contains(manifestBytes, []byte("@")) {
		return fmt.Errorf("Windows application manifest contains an unresolved template")
	}
	manifest, err := winres.AppManifestFromXML(manifestBytes)
	if err != nil {
		return fmt.Errorf("parse Windows application manifest: %w", err)
	}
	if manifest.Identity.Version != [4]uint16{uint16(numbers[0]), uint16(numbers[1]), uint16(numbers[2]), 0} {
		return fmt.Errorf("Windows application manifest version did not resolve from VERSION")
	}

	iconFile, err := os.Open(filepath.Join(root, "build", "windows", "icon.ico"))
	if err != nil {
		return fmt.Errorf("open Windows icon: %w", err)
	}
	icon, err := winres.LoadICO(iconFile)
	closeErr := iconFile.Close()
	if err != nil {
		return fmt.Errorf("load Windows icon: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close Windows icon: %w", closeErr)
	}
	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.RT_ICON, icon); err != nil {
		return fmt.Errorf("add Windows icon resource: %w", err)
	}
	resources.SetManifest(manifest)
	resources.SetVersionInfo(versionInfo)
	if err := verifyResources(&resources, appVersion, fileVersion, [4]uint16{uint16(numbers[0]), uint16(numbers[1]), uint16(numbers[2]), 0}); err != nil {
		return err
	}

	var object bytes.Buffer
	if err := resources.WriteObject(&object, winres.ArchAMD64); err != nil {
		return fmt.Errorf("generate Windows resources: %w", err)
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	if err := os.WriteFile(out, object.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}

func verifyResources(resources *winres.ResourceSet, appVersion, fileVersion string, numericVersion [4]uint16) error {
	iconCount := 0
	manifestFound := false
	versionFound := false
	resources.Walk(func(typeID, _ winres.Identifier, _ uint16, data []byte) bool {
		switch typeID {
		case winres.RT_ICON:
			if len(data) > 0 {
				iconCount++
			}
		case winres.RT_MANIFEST:
			manifestFound = bytes.Contains(data, []byte(`version="`+fileVersion+`"`))
		case winres.RT_VERSION:
			info, err := winversion.FromBytes(data)
			if err == nil && info.FileVersion == numericVersion && info.ProductVersion == numericVersion {
				table := info.Table()[winversion.LangDefault]
				versionFound = table != nil && (*table)[winversion.ProductVersion] == appVersion
			}
		}
		return true
	})
	if iconCount == 0 || !manifestFound || !versionFound {
		return fmt.Errorf("generated Windows resources failed metadata, manifest, or icon validation")
	}
	return nil
}
