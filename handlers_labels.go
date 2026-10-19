package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.mau.fi/whatsmeow/appstate"
)

func validLabelID(id string) bool {
	n, err := strconv.ParseUint(id, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == id
}

func labelEditPatch(reader io.Reader) (appstate.PatchInfo, error) {
	var request struct {
		LabelID string
		Name    *string
		Color   *int32
		Deleted *bool
	}
	if err := decodeAPIRequest(reader, &request); err != nil {
		return appstate.PatchInfo{}, err
	}
	if !validLabelID(request.LabelID) || request.Deleted == nil {
		return appstate.PatchInfo{}, fmt.Errorf("numeric LabelID and Deleted are required")
	}
	var name string
	var color int32
	if !*request.Deleted {
		if request.Name == nil || strings.TrimSpace(*request.Name) == "" || utf8.RuneCountInString(*request.Name) > 100 || request.Color == nil || *request.Color < 0 || *request.Color > 19 {
			return appstate.PatchInfo{}, fmt.Errorf("Name (1–100 characters) and Color (0–19) are required")
		}
	}
	if request.Name != nil {
		name = *request.Name
	}
	if request.Color != nil {
		color = *request.Color
	}
	return appstate.BuildLabelEdit(request.LabelID, name, color, *request.Deleted), nil
}

func labelAssociationPatch(message bool) patchBuilder {
	return func(reader io.Reader) (appstate.PatchInfo, error) {
		var request struct {
			Chat    string
			LabelID string
			Id      string
			Labeled *bool
		}
		if err := decodeAPIRequest(reader, &request); err != nil {
			return appstate.PatchInfo{}, err
		}
		chat, err := apiChatJID(request.Chat)
		if err != nil {
			return appstate.PatchInfo{}, err
		}
		if !validLabelID(request.LabelID) || request.Labeled == nil {
			return appstate.PatchInfo{}, fmt.Errorf("numeric LabelID and Labeled are required")
		}
		if message {
			if strings.TrimSpace(request.Id) == "" {
				return appstate.PatchInfo{}, fmt.Errorf("Id is required")
			}
			return appstate.BuildLabelMessage(chat, request.LabelID, request.Id, *request.Labeled), nil
		}
		if request.Id != "" {
			return appstate.PatchInfo{}, fmt.Errorf("Id is only supported for message labels")
		}
		return appstate.BuildLabelChat(chat, request.LabelID, *request.Labeled), nil
	}
}
