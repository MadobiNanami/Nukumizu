package handler

import (
	"encoding/json"
	"net/http"

	"nukumizu-backend/internal/controller"
	"nukumizu-backend/postLog"
	"nukumizu-backend/utils"
)

// OneBotMessage represents a OneBot 11 message event forwarded from napcat-bridge.
type OneBotMessage struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	GroupID     int64  `json:"group_id"`
	UserID      int64  `json:"user_id"`
	RawMessage  string `json:"raw_message"`
	Message     string `json:"message"`
	Sender      struct {
		UserID   int64  `json:"user_id"`
		Nickname string `json:"nickname"`
	} `json:"sender"`
	SelfID  int64  `json:"self_id"`
	SubType string `json:"sub_type"`
}

// BotMessageHandler handles POST /api/bot/msg/recv.
// Receives OneBot 11 events forwarded from napcat-bridge and
// routes them to the appropriate bot controller.
func BotMessageHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "POST", "bot") {
		return
	}

	var event OneBotMessage
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		postLog.Debug("Failed to parse bot message: " + err.Error())
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid message format")
		return
	}

	// Only handle message events.
	if event.PostType != "message" {
		utils.SendSuccessResponse(w, "event ignored", map[string]interface{}{
			"post_type": event.PostType,
		})
		return
	}

	// Route to the Napcat/QQ controller.
	ctrl := controller.GetController("qq(napcat)")
	if ctrl == nil {
		postLog.Debug("QQ controller not available, ignoring message")
		utils.SendSuccessResponse(w, "controller not available", nil)
		return
	}

	cmdCtrl, ok := ctrl.(controller.CommandController)
	if !ok {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "controller does not support commands")
		return
	}

	// Determine chat information.
	chatID := event.GroupID
	chatType := event.MessageType
	if chatType == "private" {
		chatID = event.UserID
	}

	// Build a command from the raw message.
	cmd := controller.Command{
		RawText:  event.RawMessage,
		ChatID:   chatID,
		ChatType: chatType,
		SenderID: event.UserID,
	}

	response, err := cmdCtrl.HandleCommand(cmd)
	if err != nil {
		postLog.Error("Bot command handling failed: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "command handling failed: "+err.Error())
		return
	}

	if response != "" {
		utils.SendSuccessResponse(w, "", map[string]interface{}{
			"response": response,
			"chatID":   chatID,
			"chatType": chatType,
		})
	} else {
		utils.SendSuccessResponse(w, "no response", nil)
	}
}
