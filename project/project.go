package bldr_project

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/bifrost/util/labels"
	manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
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
	if dest == nil {
		return errors.New("destination config cannot be nil")
	}

	if id := src.GetId(); id != "" {
		dest.Id = id
	}

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

	if dest.Manifests == nil {
		dest.Manifests = make(map[string]*ManifestConfig)
	}
	for manifestID, manifest := range src.GetManifests() {
		dest.Manifests[manifestID] = manifest.CloneVT()
	}

	if dest.Build == nil {
		dest.Build = make(map[string]*BuildConfig)
	}
	for buildID, buildConf := range src.GetBuild() {
		dest.Build[buildID] = buildConf.CloneVT()
	}

	if dest.Remotes == nil {
		dest.Remotes = make(map[string]*RemoteConfig)
	}
	for remoteID, remoteConf := range src.GetRemotes() {
		dest.Remotes[remoteID] = remoteConf.CloneVT()
	}

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
	if err := ValidateProjectID(c.GetId()); err != nil {
		return err
	}
	if err := c.GetStart().Validate(); err != nil {
		return errors.Wrap(err, "start")
	}
	for manifestID, manifestConf := range c.GetManifests() {
		if err := manifest.ValidateManifestID(manifestID, false); err != nil {
			return errors.Wrap(err, "manifests: invalid manifest id")
		}
		if err := manifestConf.Validate(); err != nil {
			return errors.Wrapf(err, "manifests[%s]: config invalid", manifestID)
		}
	}
	for remoteID, remoteConf := range c.GetRemotes() {
		if err := remoteConf.Validate(); err != nil {
			return errors.Wrapf(err, "remotes[%s]: config invalid", remoteID)
		}
	}
	return nil
}

// Validate validates the repository config.
func (c *RemoteConfig) Validate() error {
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}
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
	storeObjKey = c.GetObjectKey()
	linkObjKeys = append([]string{storeObjKey}, c.GetLinkObjectKeys()...)
	slices.Sort(linkObjKeys)
	linkObjKeys = slices.Compact(linkObjKeys)
	return storeObjKey, linkObjKeys
}

// Validate validates the start configuration.
func (c *StartConfig) Validate() error {
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
	startupPath := c.GetLoadWebStartup()
	if len(startupPath) == 0 {
		return "", nil
	}
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
	srcObjectKeys := slices.Clone(c.GetSourceObjectKeys())
	slices.Sort(srcObjectKeys)
	srcObjectKeys = slices.Compact(srcObjectKeys)
	if len(srcObjectKeys) != 0 && srcObjectKeys[0] == "" {
		srcObjectKeys = srcObjectKeys[1:]
	}
	return srcObjectKeys
}

// DedupeManifests sorts and cleans up the list of manifest ids.
//
// Returns a copy of the slice stored in the object.
func (c *PublishConfig) DedupeManifests() []string {
	manifests := slices.Clone(c.GetManifests())
	slices.Sort(manifests)
	manifests = slices.Compact(manifests)
	if len(manifests) != 0 && manifests[0] == "" {
		manifests = manifests[1:]
	}
	return manifests
}

// DedupePlatformIDs sorts and cleans up the list of platform ids.
//
// Returns a copy of the slice stored in the object.
func (c *PublishConfig) DedupePlatformIDs() []string {
	platformIDs := slices.Clone(c.GetPlatformIds())
	slices.Sort(platformIDs)
	platformIDs = slices.Compact(platformIDs)
	if len(platformIDs) != 0 && platformIDs[0] == "" {
		platformIDs = platformIDs[1:]
	}
	return platformIDs
}

// LoadExtendedProjectConfig loads a project config from an extended module path.
// sourcePath is the root directory of the current project (containing vendor/).
// modulePath is the Go module path to resolve (e.g. "github.com/aperturerobotics/alpha").
func LoadExtendedProjectConfig(sourcePath, modulePath string) (*ProjectConfig, error) {
	if modulePath == "" {
		return nil, errors.New("extends: empty module path")
	}
	vendorPath := filepath.Join(sourcePath, "vendor", modulePath)
	configPath := filepath.Join(vendorPath, "bldr.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", configPath)
	}
	conf := &ProjectConfig{}
	if err := UnmarshalProjectConfig(data, conf); err != nil {
		return nil, errors.Wrapf(err, "unmarshal %s", configPath)
	}
	return conf, nil
}

// Merge merges another config into this config.
func (c *PublishStorageConfig) Merge(ot *PublishStorageConfig) {
	if c == nil || ot == nil {
		return
	}
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
