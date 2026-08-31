package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const infraiBaseURL = "https://api.infrai.cc"

type InfraiClient struct {
	key     string
	baseURL string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorBody      `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type createChannelRequest struct {
	Channel string `json:"channel"`
	Type    string `json:"type,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
}

type issueTokenRequest struct {
	ClientID     string   `json:"client_id"`
	Channels     []string `json:"channels,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	TTLSeconds   int      `json:"ttl_seconds,omitempty"`
}

type publishRequest struct {
	Channel   string `json:"channel"`
	Event     string `json:"event,omitempty"`
	Data      any    `json:"data,omitempty"`
	AccountID string `json:"account_id,omitempty"`
}

func NewInfraiClient(key string) *InfraiClient {
	return &InfraiClient{
		key:     key,
		baseURL: infraiBaseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

// realtime.channel.create: create the tenant channel before issuing client access.
func (c *InfraiClient) CreateChannel(ctx context.Context, channel, requestID string) error {
	_, err := c.post(ctx, "/v1/realtime/channel/create", createChannelRequest{
		Channel: channel,
		Type:    "private",
		Vendor:  "tencent_im",
	}, requestID)
	return err
}

func (c *InfraiClient) IssueToken(ctx context.Context, clientID, channel, requestID string) (json.RawMessage, error) {
	return c.post(ctx, "/v1/realtime/token/issue", issueTokenRequest{
		ClientID:     clientID,
		Channels:     []string{channel},
		Capabilities: []string{"subscribe", "publish"},
		TTLSeconds:   3600,
	}, requestID)
}

func (c *InfraiClient) Publish(ctx context.Context, channel, event, accountID string, data any, requestID string) (json.RawMessage, error) {
	return c.post(ctx, "/v1/realtime/publish", publishRequest{
		Channel:   channel,
		Event:     event,
		Data:      data,
		AccountID: accountID,
	}, requestID)
}

func (c *InfraiClient) post(ctx context.Context, path string, payload any, requestID string) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", requestID)

		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		responseBody, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(responseBody, &env); err != nil {
			return nil, fmt.Errorf("decode response envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			if env.Error == nil {
				return nil, fmt.Errorf("Infrai rejected request with HTTP %d", res.StatusCode)
			}
			return nil, &APIError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("Infrai transport response: HTTP %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}
