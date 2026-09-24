package config

import (
	"errors"
	"fmt"
	"strings"
)

// Errors reported by the incoming webhook endpoint helpers. The HTTP layer maps
// them onto statuses: exists -> 409, not found -> 404, invalid -> 400.
var (
	// ErrWebhookEndpointExists is returned by AddWebhookEndpoint when the name
	// is already configured.
	ErrWebhookEndpointExists = errors.New("webhook endpoint already exists")

	// ErrWebhookEndpointNotFound is returned when the named endpoint is not
	// configured.
	ErrWebhookEndpointNotFound = errors.New("webhook endpoint not found")

	// ErrWebhookEndpointInvalid is returned when a name or field supplied for an
	// endpoint cannot be stored.
	ErrWebhookEndpointInvalid = errors.New("invalid webhook endpoint")
)

// webhookEndpointFields are the endpoint keys a client may set. A field that is
// absent from an update is left untouched; a field that is present but not
// listed here is rejected rather than written, so a typo cannot leave an
// endpoint silently ignoring a setting.
var webhookEndpointFields = map[string]func(interface{}) bool{
	"enabled":     isJSONBool,
	"token":       isJSONString,
	"notifyPipes": isJSONStringArray,
}

// WebhookEndpoints returns the configured incoming webhook endpoints keyed by
// name, as a copy: changing the result does not change the loaded
// configuration.
func WebhookEndpoints() map[string]WebhookEndpointConfig {
	endpoints := map[string]WebhookEndpointConfig{}
	if C_globalConfig == nil {
		return endpoints
	}
	for name, endpoint := range C_globalConfig.Webhook.Endpoints {
		endpoints[name] = endpoint
	}
	return endpoints
}

// AddWebhookEndpoint registers a new incoming webhook endpoint under name. Only
// the fields present in fields are set, so an endpoint can be created with
// default values and completed later by ModifyWebhookEndpoint. Unlike
// ModifyWebhookEndpoint it refuses to touch an endpoint that already exists.
func AddWebhookEndpoint(name string, fields map[string]interface{}) error {
	if err := validateWebhookEndpointName(name); err != nil {
		return err
	}
	patch, err := webhookEndpointPatch(fields)
	if err != nil {
		return err
	}

	settingsLock.Lock()
	defer settingsLock.Unlock()

	if _, exists := webhookEndpoint(name); exists {
		return fmt.Errorf("%w: %s", ErrWebhookEndpointExists, name)
	}
	return updateSettingsLocked(SettingGlobal, webhookEndpointsPatch(name, patch))
}

// ModifyWebhookEndpoint updates an existing incoming webhook endpoint. Only the
// fields present in fields are changed; every other field keeps its configured
// value.
func ModifyWebhookEndpoint(name string, fields map[string]interface{}) error {
	if err := validateWebhookEndpointName(name); err != nil {
		return err
	}
	patch, err := webhookEndpointPatch(fields)
	if err != nil {
		return err
	}
	if len(patch) == 0 {
		return fmt.Errorf("%w: no fields to update", ErrWebhookEndpointInvalid)
	}

	settingsLock.Lock()
	defer settingsLock.Unlock()

	if _, exists := webhookEndpoint(name); !exists {
		return fmt.Errorf("%w: %s", ErrWebhookEndpointNotFound, name)
	}
	return updateSettingsLocked(SettingGlobal, webhookEndpointsPatch(name, patch))
}

// DeleteWebhookEndpoint removes the incoming webhook endpoint registered under
// name. The endpoint stops accepting requests as soon as the configuration is
// reloaded.
func DeleteWebhookEndpoint(name string) error {
	settingsLock.Lock()
	defer settingsLock.Unlock()

	if _, exists := webhookEndpoint(name); !exists {
		return fmt.Errorf("%w: %s", ErrWebhookEndpointNotFound, name)
	}

	return updateSettingsLocked(SettingGlobal, webhookEndpointDeletePatch(name))
}

// webhookEndpoint returns the named endpoint held by the loaded configuration.
// No lock is needed to read it: a reload replaces the whole configuration
// rather than mutating it in place, and the value is read from whichever
// version is current.
func webhookEndpoint(name string) (WebhookEndpointConfig, bool) {
	if C_globalConfig == nil {
		return WebhookEndpointConfig{}, false
	}
	endpoint, exists := C_globalConfig.Webhook.Endpoints[name]
	return endpoint, exists
}

// webhookEndpointsPatch wraps the fields of one endpoint into the nested patch
// the settings merge expects for webhook.endpoints.<name>.
func webhookEndpointsPatch(name string, fields map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"webhook": map[string]interface{}{
			"endpoints": map[string]interface{}{name: fields},
		},
	}
}

// webhookEndpointDeletePatch is the patch that removes an endpoint. The value is
// a null, which the settings merge reads as "delete this key". It must be an
// untyped nil: a nil map of type map[string]interface{} would be merged as an
// empty object instead, leaving the endpoint in the configuration.
func webhookEndpointDeletePatch(name string) map[string]interface{} {
	return map[string]interface{}{
		"webhook": map[string]interface{}{
			"endpoints": map[string]interface{}{name: nil},
		},
	}
}

// webhookEndpointPatch validates the fields of one endpoint and returns them as
// the value to merge. Fields not accepted for an endpoint are rejected instead
// of being written to the configuration file.
func webhookEndpointPatch(fields map[string]interface{}) (map[string]interface{}, error) {
	patch := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		accepts, known := webhookEndpointFields[key]
		if !known {
			return nil, fmt.Errorf("%w: unknown field %q", ErrWebhookEndpointInvalid, key)
		}
		if !accepts(value) {
			return nil, fmt.Errorf("%w: field %q has the wrong type", ErrWebhookEndpointInvalid, key)
		}
		patch[key] = value
	}
	return patch, nil
}

// validateWebhookEndpointName checks that a name can address an endpoint. The
// name is the last segment of the endpoint URL, so a name containing a slash
// could never be reached.
func validateWebhookEndpointName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrWebhookEndpointInvalid)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("%w: name must not contain %q", ErrWebhookEndpointInvalid, "/")
	}
	return nil
}

// The predicates below accept the decoded JSON types a field may carry. Numbers
// decoded with UseNumber stay json.Number, so a JSON true/false is the only
// value accepted for a boolean field.
func isJSONBool(value interface{}) bool {
	_, ok := value.(bool)
	return ok
}

func isJSONString(value interface{}) bool {
	_, ok := value.(string)
	return ok
}

func isJSONStringArray(value interface{}) bool {
	items, ok := value.([]interface{})
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}
