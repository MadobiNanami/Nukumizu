package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"nukumizu-backend/config"
	"nukumizu-backend/utils"
)

// The incoming webhook endpoints are managed from the admin API below. They
// live in the same listener as the rest of the admin API — unlike the endpoints
// they configure, which are served on the webhook listener (see webhook.go).
//
// Every handler takes a JSON object naming the endpoint, arranged the same way
// as /api/settings/set: whatever fields the request carries are the fields that
// change, and everything else keeps its configured value. Only the fields an
// endpoint actually has are accepted, so a misspelled field is reported instead
// of being written to the configuration file.

// decodeWebhookEndpointRequest authenticates an admin request, decodes its JSON
// object body, and splits it into the endpoint name and the remaining fields.
// It answers the request itself and reports ok == false when anything is wrong.
func decodeWebhookEndpointRequest(w http.ResponseWriter, r *http.Request) (name string, fields map[string]interface{}, ok bool) {
	if !utils.Auth(w, r, "POST", "admin") {
		return "", nil, false
	}

	dec := json.NewDecoder(r.Body)
	dec.UseNumber() // Keep values verbatim, as /api/settings/set does.
	var body map[string]interface{}
	if err := dec.Decode(&body); err != nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid request body: expected a JSON object")
		return "", nil, false
	}
	if body == nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "request body must be a JSON object")
		return "", nil, false
	}

	rawName, present := body["name"]
	if !present {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing required parameter: name")
		return "", nil, false
	}
	name, isString := rawName.(string)
	if !isString {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid parameter: name must be a string")
		return "", nil, false
	}
	delete(body, "name")

	return name, body, true
}

// sendWebhookEndpointError maps the errors of the endpoint helpers onto the
// matching HTTP responses.
func sendWebhookEndpointError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, config.ErrWebhookEndpointExists):
		utils.SendErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, config.ErrWebhookEndpointNotFound):
		utils.SendErrorResponse(w, http.StatusNotFound, err.Error())
	case errors.Is(err, config.ErrWebhookEndpointInvalid):
		utils.SendErrorResponse(w, http.StatusBadRequest, err.Error())
	default:
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to update webhook endpoints: "+err.Error())
	}
}

// WebhookAddHandler handles POST /api/webhook/add.
// Body: {name, ...fields}. The endpoint must not exist yet; the fields given are
// stored and any field left out starts at its default (disabled, no token, no
// notify pipes).
func WebhookAddHandler(w http.ResponseWriter, r *http.Request) {
	name, fields, ok := decodeWebhookEndpointRequest(w, r)
	if !ok {
		return
	}

	if err := config.AddWebhookEndpoint(name, fields); err != nil {
		sendWebhookEndpointError(w, err)
		return
	}

	utils.SendSuccessResponse(w, "webhook endpoint added", map[string]interface{}{"name": name})
}

// WebhookModifyHandler handles POST /api/webhook/modify.
// Body: {name, ...fields}. Only the fields given are changed.
func WebhookModifyHandler(w http.ResponseWriter, r *http.Request) {
	name, fields, ok := decodeWebhookEndpointRequest(w, r)
	if !ok {
		return
	}

	if err := config.ModifyWebhookEndpoint(name, fields); err != nil {
		sendWebhookEndpointError(w, err)
		return
	}

	utils.SendSuccessResponse(w, "webhook endpoint updated", map[string]interface{}{"name": name})
}

// WebhookDeleteHandler handles POST /api/webhook/delete.
// Body: {name}.
func WebhookDeleteHandler(w http.ResponseWriter, r *http.Request) {
	name, _, ok := decodeWebhookEndpointRequest(w, r)
	if !ok {
		return
	}

	if err := config.DeleteWebhookEndpoint(name); err != nil {
		sendWebhookEndpointError(w, err)
		return
	}

	utils.SendSuccessResponse(w, "webhook endpoint deleted", map[string]interface{}{"name": name})
}

// WebhookListHandler handles GET /api/webhook/list.
// Returns every configured incoming webhook endpoint, keyed by name.
func WebhookListHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "GET", "admin") {
		return
	}

	utils.SendSuccessResponse(w, "", map[string]interface{}{
		"endpoints": config.WebhookEndpoints(),
	})
}
