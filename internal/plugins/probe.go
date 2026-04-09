package plugins

import (
	"fmt"
	"path"
	"strings"

	"github.com/cogentcore/yaegi/interp"
	"github.com/cogentcore/yaegi/stdlib"

	"github.com/idelchi/aura/sdk"
	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// sdkVersionImportPath is the import path for the sdk/version sub-package.
const sdkVersionImportPath = "github.com/idelchi/aura/sdk/version"

// copySDKVersionToGoPath copies the vendored sdk/version/version.go into the
// top-level GOPATH so Yaegi can import it. Returns nil when no vendored SDK
// exists (in-repo test plugins without vendor/).
func copySDKVersionToGoPath(pluginDir, goPath string) error {
	src := folder.New(pluginDir, "vendor", "github.com", "idelchi", "aura", "sdk", "version").WithFile("version.go")

	if !file.New(src.Path()).Exists() {
		return nil
	}

	data, err := file.New(src.Path()).Read()
	if err != nil {
		return fmt.Errorf("reading vendored sdk/version: %w", err)
	}

	dst := folder.New(goPath, "src", "github.com", "idelchi", "aura", "sdk", "version")

	if err := dst.Create(); err != nil {
		return fmt.Errorf("creating sdk/version in GOPATH: %w", err)
	}

	return dst.WithFile("version.go").Write(data, 0o644)
}

// probeSDKVersion imports sdk/version via Yaegi and returns the Version constant.
// Returns "" when the package is not available (no vendored SDK).
func probeSDKVersion(i *interp.Interpreter) string {
	if _, err := i.Eval(fmt.Sprintf(`import %q`, sdkVersionImportPath)); err != nil {
		return ""
	}

	v, err := i.Eval("version.Version")
	if err != nil {
		return ""
	}

	s, ok := v.Interface().(string)
	if !ok {
		return ""
	}

	return s
}

// Capabilities describes what a plugin exports.
type Capabilities struct {
	SDKVersion  string   // empty if no vendored SDK
	Hooks       []string // timing strings: "BeforeChat", etc.
	ToolName    string   // empty if no tool
	CommandName string   // empty if no command
}

// ProbeCapabilities discovers which exports a plugin directory implements
// without creating a full Plugin struct.
// Creates a throwaway Yaegi interpreter — suitable for CLI commands, not hot paths.
func ProbeCapabilities(dir string) (Capabilities, error) {
	modulePath, err := readModulePath(dir)
	if err != nil {
		return Capabilities{}, err
	}

	basePkg := strings.ReplaceAll(path.Base(modulePath), "-", "_")

	gp, err := folder.CreateRandomInDir("", "aura-plugin-probe-*")
	if err != nil {
		return Capabilities{}, fmt.Errorf("creating probe GOPATH: %w", err)
	}

	goPath := gp.Path()
	defer folder.New(goPath).Remove()

	if err := copyPluginToGoPath(dir, folder.New(goPath, "src", modulePath).Path()); err != nil {
		return Capabilities{}, err
	}

	if err := copySDKVersionToGoPath(dir, goPath); err != nil {
		return Capabilities{}, err
	}

	i := interp.New(interp.Options{GoPath: goPath})
	i.Use(stdlib.Symbols)
	i.Use(sdk.Symbols)

	if _, err := i.Eval(fmt.Sprintf(`import %q`, modulePath)); err != nil {
		return Capabilities{}, err
	}

	var caps Capabilities

	caps.SDKVersion = probeSDKVersion(i)

	// Probe hooks.
	for _, def := range KnownTimingNames() {
		symbol := fmt.Sprintf("%s.%s", basePkg, def.Name)
		if _, err := i.Eval(symbol); err == nil {
			caps.Hooks = append(caps.Hooks, def.Timing.String())
		}
	}

	// Probe tool.
	if v, err := i.Eval(basePkg + ".Schema"); err == nil {
		if fn, ok := v.Interface().(func() sdk.ToolSchema); ok {
			caps.ToolName = fn().Name
		}
	}

	// Probe command.
	if v, err := i.Eval(basePkg + ".Command"); err == nil {
		if fn, ok := v.Interface().(func() sdk.CommandSchema); ok {
			caps.CommandName = fn().Name
		}
	}

	return caps, nil
}
