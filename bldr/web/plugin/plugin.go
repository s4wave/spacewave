package bldr_web_plugin

import (
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/pkg/errors"
)

// Validate validates the HandleWebViewViaPluginRequest configuration.
func (m *HandleWebViewViaPluginRequest) Validate() error {
	// Validate plugin ID
	if err := bldr_plugin.ValidatePluginID(m.GetHandlePluginId(), false); err != nil {
		return errors.Wrap(err, "handle_plugin_id")
	}

	return nil
}

// Validate validates the HandleWebPkgViaPluginRequest configuration.
func (m *HandleWebPkgViaPluginRequest) Validate() error {
	// Validate plugin ID
	if err := bldr_plugin.ValidatePluginID(m.GetHandlePluginId(), false); err != nil {
		return errors.Wrap(err, "handle_plugin_id")
	}

	// Validate web pkg IDs in prefix list
	for i, webPkgID := range m.GetWebPkgIdPrefixes() {
		if err := web_pkg.ValidateWebPkgId(webPkgID); err != nil {
			return errors.Wrapf(err, "web_pkg_id_prefixes[%d]: %s", i, webPkgID)
		}
	}

	// Validate web pkg IDs in ID list
	for i, webPkgID := range m.GetWebPkgIdList() {
		if err := web_pkg.ValidateWebPkgId(webPkgID); err != nil {
			return errors.Wrapf(err, "web_pkg_id_list[%d]: %s", i, webPkgID)
		}
	}

	return nil
}

// Validate validates the HandleRpcViaPluginRequest configuration.
func (m *HandleRpcViaPluginRequest) Validate() error {
	// Validate plugin ID
	if err := bldr_plugin.ValidatePluginID(m.GetHandlePluginId(), false); err != nil {
		return errors.Wrap(err, "handle_plugin_id")
	}

	// Validate backoff configuration
	if err := m.GetBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "backoff")
	}

	return nil
}

// Validate validates the HandleWebViewViaHandlersRequest configuration.
func (m *HandleWebViewViaHandlersRequest) Validate() error {
	// Validate config
	if err := m.GetConfig().Validate(); err != nil {
		return errors.Wrap(err, "config")
	}

	return nil
}

// Validate validates the HandleWebPkgsViaPluginAssetsRequest configuration.
func (m *HandleWebPkgsViaPluginAssetsRequest) Validate() error {
	// Validate plugin ID
	if err := bldr_plugin.ValidatePluginID(m.GetHandlePluginId(), false); err != nil {
		return errors.Wrap(err, "handle_plugin_id")
	}

	// Validate web pkg IDs
	for i, webPkgID := range m.GetWebPkgIdList() {
		if err := web_pkg.ValidateWebPkgId(webPkgID); err != nil {
			return errors.Wrapf(err, "web_pkg_id_list[%d]: %s", i, webPkgID)
		}
	}

	return nil
}
