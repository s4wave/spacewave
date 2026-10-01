package bldr_project

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/s4wave/spacewave/net/util/labels"
)

// UnmarshalProjectConfig unmarshals a project config from json or yaml.
func UnmarshalProjectConfig(data []byte, conf *ProjectConfig) error {
	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}

	return conf.UnmarshalJSON(jdata)
}

// ValidateProjectID validates a project identifier.
func ValidateProjectID(id string) error {
	if id == "" {
		return ErrEmptyProjectID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "project id")
	}
	return nil
}

// MergeProjectConfigs merges values from a config into another.
// Returns the result of Validate().
func MergeProjectConfigs(dest, src *ProjectConfig) error {
	// Reject a missing destination config.
	if dest == nil {
		return errors.New("destination config cannot be nil")
	}

	// Merge the project id and start configuration.
	if id := src.GetId(); id != "" {
		dest.Id = id
	}

	// Merge the start config, deduplicating the plugin list.
	srcStart := src.GetStart()
	if dest.Start == nil {
		dest.Start = &StartConfig{}
	}
	if srcStart.GetDisableBuild() {
		dest.Start.DisableBuild = true
	}
	dest.Start.Plugins = append(dest.Start.Plugins, srcStart.GetPlugins()...)
	slices.Sort(dest.Start.Plugins)
	dest.Start.Plugins = slices.Compact(dest.Start.Plugins)
	if ws := srcStart.GetLoadWebStartup(); ws != "" {
		dest.Start.LoadWebStartup = ws
	}

	// Merge the manifest configs by id.
	if dest.Manifests == nil {
		dest.Manifests = make(map[string]*ManifestConfig)
	}
	for manifestID, manifest := range src.GetManifests() {
		dest.Manifests[manifestID] = manifest.CloneVT()
	}

	// Merge the build configs by id.
	if dest.Build == nil {
		dest.Build = make(map[string]*BuildConfig)
	}
	for buildID, buildConf := range src.GetBuild() {
		dest.Build[buildID] = buildConf.CloneVT()
	}

	// Merge the remote configs by id.
	if dest.Remotes == nil {
		dest.Remotes = make(map[string]*RemoteConfig)
	}
	for remoteID, remoteConf := range src.GetRemotes() {
		dest.Remotes[remoteID] = remoteConf.CloneVT()
	}

	// Merge the publish configs by id and validate the result.
	if dest.Publish == nil {
		dest.Publish = make(map[string]*PublishConfig)
	}
	for publishID, publishConf := range src.GetPublish() {
		dest.Publish[publishID] = publishConf.CloneVT()
	}

	return dest.Validate()
}

// Validate validates the project configuration.
func (c *ProjectConfig) Validate() error {
	// Validate the project id and start configuration.
	if err := ValidateProjectID(c.GetId()); err != nil {
		return err
	}
	if err := c.GetStart().Validate(); err != nil {
		return errors.Wrap(err, "start")
	}

	// Validate each manifest config and its id.
	for manifestID, manifestConf := range c.GetManifests() {
		if err := manifest.ValidateManifestID(manifestID, false); err != nil {
			return errors.Wrap(err, "manifests: invalid manifest id")
		}
		if err := manifestConf.Validate(); err != nil {
			return errors.Wrapf(err, "manifests[%s]: config invalid", manifestID)
		}
	}

	// Validate each remote config.
	for remoteID, remoteConf := range c.GetRemotes() {
		if err := remoteConf.Validate(); err != nil {
			return errors.Wrapf(err, "remotes[%s]: config invalid", remoteID)
		}
	}

	// Validate each build config.
	for buildID, buildConf := range c.GetBuild() {
		if err := buildConf.Validate(); err != nil {
			return errors.Wrapf(err, "build[%s]: config invalid", buildID)
		}
	}
	return nil
}

// Validate validates the build target configuration.
func (c *BuildConfig) Validate() error {
	if err := c.GetBuildPolicy().Validate(); err != nil {
		return errors.Wrap(err, "build_policy")
	}
	return nil
}

// Validate validates the repository config.
func (c *RemoteConfig) Validate() error {
	// Require an engine id and a valid host config set.
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}

	// Require an object key and a parseable peer id.
	if c.GetObjectKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "remote")
	}
	_, err := c.ParsePeerID()
	if err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer id field.
func (c *RemoteConfig) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// CleanupLinkObjectKeys returns a compacted and sorted copy of the list of
// object keys to link including storeObjKey.
func (c *RemoteConfig) CleanupLinkObjectKeys() (storeObjKey string, linkObjKeys []string) {
	// Collect the store key and the link keys, sorted and compacted.
	storeObjKey = c.GetObjectKey()
	linkObjKeys = append([]string{storeObjKey}, c.GetLinkObjectKeys()...)
	slices.Sort(linkObjKeys)
	linkObjKeys = slices.Compact(linkObjKeys)
	return storeObjKey, linkObjKeys
}

// Validate validates the start configuration.
func (c *StartConfig) Validate() error {
	// Validate each plugin id in the start config.
	for _, pluginID := range c.GetPlugins() {
		if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
			return errors.Wrapf(err, "plugins[%s]: invalid plugin id", pluginID)
		}
	}
	if _, err := c.ParseWebStartupPath(); err != nil {
		return err
	}
	return nil
}

// ParseWebStartupPath validates and cleans the web startup path.
// If unset, returns "", nil.
func (c *StartConfig) ParseWebStartupPath() (string, error) {
	// Return empty when the web startup path is unset.
	startupPath := c.GetLoadWebStartup()
	if len(startupPath) == 0 {
		return "", nil
	}

	// Validate the cleaned startup path.
	startupPath = path.Clean(startupPath)
	if startupPath[0] == '/' {
		return "", errors.New("load_web_startup: must be a relative path")
	}
	startupPathExt := path.Ext(startupPath)
	if startupPathExt != ".js" && startupPathExt != ".tsx" && startupPathExt != ".ts" {
		return "", errors.New("load_web_startup: must be a .js, .tsx, or .ts file")
	}
	if strings.HasPrefix(startupPath, "../") {
		return "", errors.New("load_web_startup: must be relative to ./")
	}
	return startupPath, nil
}

// Validate validates the plugin config.
func (c *ManifestConfig) Validate() error {
	if err := c.GetBuilder().Validate(); err != nil {
		return errors.Wrap(err, "builder")
	}
	return nil
}

// DedupeSrcObjectKeys sorts and cleans up the list of source object keys.
//
// Returns a copy of the slice stored in the object.
func (c *PublishConfig) DedupeSrcObjectKeys() []string {
	return dedupeNonEmptyStrings(c.GetSourceObjectKeys())
}

// DedupeManifests sorts and cleans up the list of manifest ids.
//
// Returns a copy of the slice stored in the object.
func (c *PublishConfig) DedupeManifests() []string {
	return dedupeNonEmptyStrings(c.GetManifests())
}

// DedupePlatformIDs sorts and cleans up the list of platform ids.
//
// Returns a copy of the slice stored in the object.
func (c *PublishConfig) DedupePlatformIDs() []string {
	return dedupeNonEmptyStrings(c.GetPlatformIds())
}

// DedupeStrings clones, sorts, compacts, and drops a leading empty entry
// from values.
func DedupeStrings(values []string) []string {
	return dedupeNonEmptyStrings(values)
}

// dedupeNonEmptyStrings clones, sorts, compacts, and drops a leading empty
// entry from values.
func dedupeNonEmptyStrings(values []string) []string {
	// Clone, sort, compact, and drop a leading empty entry.
	values = slices.Clone(values)
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) != 0 && values[0] == "" {
		values = values[1:]
	}
	return values
}

// LoadExtendedProjectConfig loads a project config from an extended module path.
// sourcePath is the root directory of the current project (containing vendor/).
// modulePath is the Go module path to resolve (e.g. "github.com/s4wave/spacewave").
// Returns the loaded config and a list of files that were loaded (for watch tracking).
func LoadExtendedProjectConfig(sourcePath, modulePath string) (*ProjectConfig, []string, error) {
	// Reject an empty module path.
	if modulePath == "" {
		return nil, nil, errors.New("extends: empty module path")
	}

	// Read and unmarshal the vendored bldr.yaml config.
	vendorPath := filepath.Join(sourcePath, "vendor", modulePath)
	configPath := filepath.Join(vendorPath, "bldr.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "read %s", configPath)
	}
	conf := &ProjectConfig{}
	if err := UnmarshalProjectConfig(data, conf); err != nil {
		return nil, nil, errors.Wrapf(err, "unmarshal %s", configPath)
	}
	loadedFiles := []string{configPath}

	// Check for bldr.star in the vendored directory.
	starPath := filepath.Join(vendorPath, "bldr.star")
	if _, serr := os.Stat(starPath); serr == nil {
		loadedFiles = append(loadedFiles, starPath)
	}

	return conf, loadedFiles, nil
}

// Merge merges another config into this config.
func (c *PublishStorageConfig) Merge(ot *PublishStorageConfig) {
	// Merge nothing when either config is nil.
	if c == nil || ot == nil {
		return
	}

	// Merge the transform, transform ref, and timestamp fields.
	if xfrm := ot.GetTransformConf(); !xfrm.GetEmpty() {
		c.TransformConf = xfrm.Clone()
	}
	if xfrmRef := ot.GetTransformConfFromRef(); !xfrmRef.GetEmpty() {
		c.TransformConfFromRef = xfrmRef.Clone()
	}
	if ts := ot.GetTimestamp(); !ts.GetEmpty() {
		c.Timestamp = ts.CloneVT()
	}
}
