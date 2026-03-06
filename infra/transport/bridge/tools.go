package bridge

import (
	"context"

	"gogogot/infra/tools"
	"gogogot/infra/transport"
)

func TransportTools() []tools.Tool {
	return []tools.Tool{
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

func sendFileHandler(ctx context.Context, input map[string]any) tools.Result {
	t, channelID, ok := transport.FromContext(ctx)
	if !ok {
		return tools.Result{Output: "error: no transport in context", IsErr: true}
	}

	fs, ok := t.(transport.FileSender)
	if !ok {
		return tools.Result{
			Output: "error: current transport (" + t.Name() + ") does not support file sending",
			IsErr:  true,
		}
	}

	path, _ := input["path"].(string)
	if path == "" {
		return tools.Result{Output: "error: path is required", IsErr: true}
	}

	caption, _ := input["caption"].(string)

	if err := fs.SendFile(ctx, channelID, path, caption); err != nil {
		return tools.Result{Output: "failed to send file: " + err.Error(), IsErr: true}
	}
	return tools.Result{Output: "File sent successfully"}
}
