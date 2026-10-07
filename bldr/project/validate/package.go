//go:build !js

package bldr_project_validate

import (
	"bytes"
	"io/fs"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// lifecycleScripts are the package.json scripts a package manager runs during
// install.
var lifecycleScripts = []string{"preinstall", "install", "postinstall", "preprepare", "prepare", "postprepare"}

// dependencyFields are the package.json fields that name direct dependencies.
var dependencyFields = []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}

// checkPackage reads package.json and bun.lock, refuses install scripts and
// dependencies not pinned by hash, and records the locked dependencies.
func (v *Validation) checkPackage(fsys fs.FS) error {
	// A project without package.json installs nothing.
	data, err := fs.ReadFile(fsys, "package.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "read package.json")
	}

	// Parse the package manifest.
	var parser fastjson.Parser
	pkg, err := parser.ParseBytes(data)
	if err != nil {
		v.refuse(RefusalKind_REFUSAL_KIND_CONFIG, "package.json does not parse: "+err.Error())
		return nil
	}

	// Refuse scripts and trusted dependencies that run during install.
	for _, script := range lifecycleScripts {
		if pkg.Exists("scripts", script) {
			v.refuse(RefusalKind_REFUSAL_KIND_LIFECYCLE_SCRIPT, "package.json has a "+script+" script, which would run during install.")
		}
	}
	if pkg.Exists("trustedDependencies") {
		v.refuse(RefusalKind_REFUSAL_KIND_LIFECYCLE_SCRIPT, "package.json trusts dependencies to run their install scripts.")
	}

	// Collect the direct dependencies.
	direct := make(map[string]bool)
	for _, field := range dependencyFields {
		if deps := pkg.GetObject(field); deps != nil {
			deps.Visit(func(name []byte, _ *fastjson.Value) {
				direct[string(name)] = true
			})
		}
	}

	// Require bun.lock whenever there is anything to install.
	lock, err := fs.ReadFile(fsys, "bun.lock")
	if errors.Is(err, fs.ErrNotExist) {
		if len(direct) != 0 {
			v.refuse(RefusalKind_REFUSAL_KIND_UNPINNED_DEPENDENCY, "package.json has dependencies but no bun.lock pins them.")
		}
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "read bun.lock")
	}
	v.checkLock(lock, direct)
	return nil
}

// checkLock records each package bun.lock pins and refuses any it does not
// pin by an integrity hash, such as git, file and workspace packages.
func (v *Validation) checkLock(data []byte, direct map[string]bool) {
	// Parse the lockfile, which allows trailing commas.
	var parser fastjson.Parser
	lock, err := parser.ParseBytes(withoutTrailingCommas(data))
	if err != nil {
		v.refuse(RefusalKind_REFUSAL_KIND_UNPINNED_DEPENDENCY, "bun.lock does not parse: "+err.Error())
		return
	}

	// Refuse trusted dependencies recorded in the lockfile.
	if lock.Exists("trustedDependencies") {
		v.refuse(RefusalKind_REFUSAL_KIND_LIFECYCLE_SCRIPT, "bun.lock trusts dependencies to run their install scripts.")
	}

	// Check each locked package.
	packages := lock.GetObject("packages")
	if packages == nil {
		return
	}
	packages.Visit(func(key []byte, entry *fastjson.Value) {
		v.checkLockEntry(string(key), entry.GetArray(), direct)
	})
}

// checkLockEntry records one bun.lock package entry pinned by hash, as
// [ident, registry, info, integrity], and refuses any other entry.
func (v *Validation) checkLockEntry(key string, fields []*fastjson.Value, direct map[string]bool) {
	// Split the ident into the package name and its locked version.
	ident := ""
	if len(fields) != 0 {
		ident = string(fields[0].GetStringBytes())
	}
	at := strings.LastIndexByte(ident, '@')
	if at <= 0 {
		v.refuse(RefusalKind_REFUSAL_KIND_UNPINNED_DEPENDENCY, "bun.lock has an entry for "+key+" without a version.")
		return
	}
	name, version := ident[:at], ident[at+1:]

	// Require an npm integrity hash.
	if len(fields) != 4 || !strings.HasPrefix(string(fields[3].GetStringBytes()), "sha512-") {
		v.refuse(RefusalKind_REFUSAL_KIND_UNPINNED_DEPENDENCY, "bun.lock does not pin "+ident+" by its content hash.")
		return
	}

	// Record the dependency; only the top-level entry of a named package is direct.
	v.Dependencies = append(v.Dependencies, &Dependency{
		Name:    name,
		Version: version,
		Direct:  direct[name] && key == name,
	})
}

// withoutTrailingCommas returns data with each comma that directly precedes a
// closing bracket or brace removed, leaving string contents unchanged.
func withoutTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString, escaped := false, false
	for i, c := range data {
		// Copy string contents verbatim, tracking escapes and the closing quote.
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			out = append(out, c)
			continue
		}

		// Drop a comma whose next token closes the array or object.
		inString = c == '"'
		if c == ',' {
			rest := bytes.TrimLeft(data[i+1:], " \t\r\n")
			if len(rest) != 0 && (rest[0] == ']' || rest[0] == '}') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}
