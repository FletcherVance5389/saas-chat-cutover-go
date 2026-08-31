package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var ErrAccountSuspended = errors.New("account is suspended")
var ErrAccountNotFound = errors.New("account not found")

type RealtimeAPI interface {
	CreateChannel(context.Context, string, string) error
	IssueToken(context.Context, string, string, string) (json.RawMessage, error)
	Publish(context.Context, string, string, string, any, string) (json.RawMessage, error)
}

type AccountState string

const (
	AccountActive    AccountState = "active"
	AccountSuspended AccountState = "suspended"
)

type TenantChat struct {
	api      RealtimeAPI
	mu       sync.RWMutex
	accounts map[string]AccountState
}

type OnboardingResult struct {
	AccountID string          `json:"account_id"`
	Channel   string          `json:"channel"`
	State     AccountState    `json:"state"`
	Token     json.RawMessage `json:"token"`
}

func NewTenantChat(api RealtimeAPI) *TenantChat {
	return &TenantChat{api: api, accounts: make(map[string]AccountState)}
}

func (s *TenantChat) Onboard(ctx context.Context, accountID, clientID, requestID string) (OnboardingResult, error) {
	channel := tenantChannel(accountID)
	if err := s.api.CreateChannel(ctx, channel, requestID+":channel"); err != nil {
		return OnboardingResult{}, err
	}
	token, err := s.api.IssueToken(ctx, clientID, channel, requestID+":token")
	if err != nil {
		return OnboardingResult{}, err
	}
	s.mu.Lock()
	s.accounts[accountID] = AccountActive
	s.mu.Unlock()
	return OnboardingResult{AccountID: accountID, Channel: channel, State: AccountActive, Token: token}, nil
}

func (s *TenantChat) SetAccountState(accountID string, state AccountState) error {
	if state != AccountActive && state != AccountSuspended {
		return fmt.Errorf("invalid account state %q", state)
	}
	s.mu.Lock()
	s.accounts[accountID] = state
	s.mu.Unlock()
	return nil
}

func (s *TenantChat) Announce(ctx context.Context, accountID, event string, data any, requestID string) (json.RawMessage, error) {
	s.mu.RLock()
	state := s.accounts[accountID]
	s.mu.RUnlock()
	if state == AccountSuspended {
		return nil, ErrAccountSuspended
	}
	if state != AccountActive {
		return nil, ErrAccountNotFound
	}
	return s.api.Publish(ctx, tenantChannel(accountID), event, accountID, data, requestID)
}

func tenantChannel(accountID string) string {
	return "tenant:" + accountID
}
