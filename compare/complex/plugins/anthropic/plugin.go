package anthropicplug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// state calls the Messages API with the official SDK: tools, typed content
// blocks, errors and streaming. The API is a mock started by the plugin.
type state struct {
	client anthropic.Client
	srv    *httptest.Server
}

func setup(ctx context.Context) (*state, error) {
	s := &state{srv: httptest.NewServer(http.HandlerFunc(mockAPI))}
	s.client = anthropic.NewClient(
		option.WithBaseURL(s.srv.URL),
		option.WithAPIKey("sk-ant-test"),
		option.WithMaxRetries(0),
	)
	go func() {
		<-ctx.Done()
		s.srv.Close()
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx := req.Context()
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-test"),
		MaxTokens: 256,
		System:    []anthropic.TextBlockParam{{Text: "You classify HTTP requests."}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Method + " " + req.URL.Path)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &anthropic.ToolParam{
			Name:        "block",
			Description: anthropic.String("Block the request"),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{"reason": map[string]any{"type": "string"}},
				Required:   []string{"reason"},
			},
		}}},
	}
	msg, err := s.client.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return fmt.Sprintf("api error status=%d raw=%s", apiErr.StatusCode, strings.TrimSpace(apiErr.RawJSON())), nil
		}
		return "", err
	}
	var parts []string
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			parts = append(parts, "text:"+b.Text)
		case anthropic.ToolUseBlock:
			var in struct{ Reason string }
			if err := json.Unmarshal(b.Input, &in); err != nil {
				return "", err
			}
			parts = append(parts, "tool:"+b.Name+"("+in.Reason+")")
		}
	}

	// The same question, streamed and accumulated.
	stream := s.client.Messages.NewStreaming(ctx, params)
	acc := anthropic.Message{}
	events := 0
	for stream.Next() {
		if err := acc.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("accumulate event %d: %w", events, err)
		}
		events++
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("stream after %d events: %w", events, err)
	}
	streamed := ""
	for _, block := range acc.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			streamed += t.Text
		}
	}
	return fmt.Sprintf("id=%s stop=%s parts=%q usage=%d/%d stream=%d:%q stop2=%s",
		msg.ID, msg.StopReason, parts, msg.Usage.InputTokens, msg.Usage.OutputTokens,
		events, streamed, acc.StopReason), nil
}

// mockAPI answers /v1/messages as the Anthropic API does.
func mockAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "sk-ant-test" || r.Header.Get("Anthropic-Version") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`)
		return
	}
	var in struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &in); err != nil || len(in.Messages) != 1 || len(in.Tools) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad body"}}`)
		return
	}
	q := in.Messages[0].Content[0].Text
	if strings.Contains(q, "/admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"type":"error","error":{"type":"permission_error","message":"admin"}}`)
		return
	}
	answer := "allow " + strings.ToLower(q)
	if in.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		ev := func(name string, v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		}
		ev("message_start", map[string]any{"type": "message_start", "message": map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": in.Model, "content": []any{},
			"stop_reason": nil, "usage": map[string]any{"input_tokens": 10, "output_tokens": 1}}})
		ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""}})
		for _, part := range strings.SplitAfter(answer, " ") {
			ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": part}})
		}
		ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		ev("message_delta", map[string]any{"type": "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 7}})
		ev("message_stop", map[string]any{"type": "message_stop"})
		return
	}
	content := []any{map[string]any{"type": "text", "text": answer}}
	stop := "end_turn"
	if strings.HasPrefix(q, "POST /api/") {
		content = append(content, map[string]any{"type": "tool_use", "id": "toolu_1", "name": "block",
			"input": map[string]any{"reason": "api write"}})
		stop = "tool_use"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": in.Model, "content": content,
		"stop_reason": stop, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 10, "output_tokens": len(answer)},
	})
}
