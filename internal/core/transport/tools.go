package transport

import (
	"context"
	"gogogot/internal/channel"
	"gogogot/internal/tools/types"
)

func ChannelTools() []types.Tool {
	return []types.Tool{
		{
			Name:        "send_file",
			Description: "Send a file (document, image, audio, video) back to the user through the current communication channel.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to send",
				},
				"caption": map[string]any{
					"type":        "string",
					"description": "Optional caption for the file",
				},
			},
			Required: []string{"path"},
			Handler:  sendFileHandler,
		},
	}
}

func sendFileHandler(ctx context.Context, input map[string]any) types.Result {
	ch, channelID, ok := channel.FromContext(ctx)
	if !ok {
		return types.Result{Output: "error: no channel in context", IsErr: true}
	}

	path, _ := input["path"].(string)
	if path == "" {
		return types.Result{Output: "error: path is required", IsErr: true}
	}

	caption, _ := input["caption"].(string)

	if err := ch.SendFile(ctx, channelID, path, caption); err != nil {
		return types.Result{Output: "failed to send file: " + err.Error(), IsErr: true}
	}
	return types.Result{Output: "File sent successfully"}
}
