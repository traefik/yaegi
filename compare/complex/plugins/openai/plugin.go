package openaiplug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// state calls an OpenAI compatible API with the official SDK. The API is a
// mock started by the plugin itself, so the plugin works without network.
type state struct {
	client openai.Client
	srv    *httptest.Server
}

func setup(ctx context.Context) (*state, error) {
	s := &state{srv: httptest.NewServer(http.HandlerFunc(mockAPI))}
	s.client = openai.NewClient(
		option.WithBaseURL(s.srv.URL+"/v1/"),
		option.WithAPIKey("sk-test"),
		option.WithMaxRetries(0),
		option.WithHeader("X-Plugin", "complex"),
	)
	go func() {
		<-ctx.Done()
		s.srv.Close()
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx := req.Context()
	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4oMini,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You classify HTTP requests."),
			openai.UserMessage(req.Method + " " + req.URL.Path),
		},
		Temperature: openai.Float(0.2),
		MaxTokens:   openai.Int(50),
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        "block",
				Description: openai.String("Block the request"),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": map[string]any{"reason": map[string]string{"type": "string"}},
				},
			}),
		},
	}
	cc, err := s.client.Chat.Completions.New(ctx, params)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return fmt.Sprintf("api error status=%d type=%s msg=%s", apiErr.StatusCode, apiErr.Type, apiErr.Message), nil
		}
		return "", err
	}
	msg := cc.Choices[0].Message
	var tool string
	for _, tc := range msg.ToolCalls {
		var args struct{ Reason string }
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return "", err
		}
		tool += tc.Function.Name + "(" + args.Reason + ")"
	}

	// The same question, streamed.
	stream := s.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}
	chunks := 0
	for stream.Next() {
		acc.AddChunk(stream.Current())
		chunks++
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	streamed := ""
	if len(acc.Choices) > 0 {
		streamed = acc.Choices[0].Message.Content
	}
	return fmt.Sprintf("id=%s model=%s finish=%s content=%q tool=%s tokens=%d stream=%d:%q raw=%t",
		cc.ID, cc.Model, cc.Choices[0].FinishReason, msg.Content, tool, cc.Usage.TotalTokens,
		chunks, streamed, cc.JSON.Usage.Valid()), nil
}

// mockAPI answers /v1/chat/completions as the OpenAI API does.
func mockAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-test" {
		http.Error(w, `{"error":{"message":"bad path or key","type":"invalid_request_error"}}`, http.StatusNotFound)
		return
	}
	var in struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &in); err != nil || len(in.Messages) != 2 || len(in.Tools) != 1 {
		http.Error(w, `{"error":{"message":"bad body","type":"invalid_request_error"}}`, http.StatusBadRequest)
		return
	}
	q := in.Messages[1].Content
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(q, "/admin") {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":{"message":"content policy","type":"policy_violation","code":"blocked"}}`)
		return
	}
	answer := "allow " + strings.ToLower(q)
	if in.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		for i, part := range strings.SplitAfter(answer, " ") {
			c := map[string]any{"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1700000000, "model": in.Model,
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": part}}}}
			if i == 0 {
				c["system_fingerprint"] = "fp_test"
			}
			cb, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", cb)
		}
		io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	msg := map[string]any{"role": "assistant", "content": answer}
	finish := "stop"
	if strings.HasPrefix(q, "POST /api/") {
		msg["tool_calls"] = []any{map[string]any{"id": "call_1", "type": "function",
			"function": map[string]any{"name": "block", "arguments": `{"reason":"api write"}`}}}
		finish = "tool_calls"
	}
	json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion", "created": 1700000000, "model": in.Model,
		"choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}},
		"usage":   map[string]any{"prompt_tokens": 12, "completion_tokens": len(answer), "total_tokens": 12 + len(answer)},
	})
}
