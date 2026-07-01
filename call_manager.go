package main

import (
	"sync"

	meowcaller "github.com/purpshell/meowcaller"
	"go.mau.fi/whatsmeow/types"
)

// CallEntry é uma chamada ativa registrada no CallManager.
type CallEntry struct {
	Call       *meowcaller.Call
	UserID     string    // ID do usuário WuzAPI dono da sessão
	IsIncoming bool      // true = entrante (precisa de Answer); false = saída
	PreSink    *preSink  // não-nil para chamadas saintes: buffer de frames até o WS conectar
}

// PendingIncomingCall armazena metadados de chamadas entrantes ainda não atendidas,
// usados como fallback quando meowcaller não consegue descriptografar a callKey
// (ex: chamadas com <silence reason="privacy"/>).
type PendingIncomingCall struct {
	UserID       string
	CallerJID    types.JID // LID (@lid) — roteamento primário
	CallerAltJID types.JID // JID de telefone (@s.whatsapp.net) — fallback
}

// callManager é um registry thread-safe de chamadas ativas, keyed por callID.
var callManager = &CallManager{
	calls:   make(map[string]*CallEntry),
	pending: make(map[string]*PendingIncomingCall),
}

type CallManager struct {
	mu      sync.RWMutex
	calls   map[string]*CallEntry
	pending map[string]*PendingIncomingCall
}

func (m *CallManager) Register(callID, userID string, call *meowcaller.Call, isIncoming bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[callID] = &CallEntry{Call: call, UserID: userID, IsIncoming: isIncoming}
}

func (m *CallManager) Get(callID string) (*meowcaller.Call, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.calls[callID]
	if !ok {
		return nil, "", false
	}
	return e.Call, e.UserID, true
}

func (m *CallManager) GetEntry(callID string) (*CallEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.calls[callID]
	return e, ok
}

func (m *CallManager) Delete(callID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.calls, callID)
	delete(m.pending, callID)
}

// RegisterPending armazena metadados de uma chamada entrante antes do meowcaller
// processá-la. Serve de fallback para chamadas com <silence reason="privacy"/>.
func (m *CallManager) RegisterPending(callID, userID string, callerJID, callerAltJID types.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending[callID] = &PendingIncomingCall{UserID: userID, CallerJID: callerJID, CallerAltJID: callerAltJID}
}

func (m *CallManager) GetPending(callID string) (*PendingIncomingCall, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.pending[callID]
	return p, ok
}

func (m *CallManager) ListByUser(userID string) []CallEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []CallEntry
	for _, e := range m.calls {
		if e.UserID == userID {
			out = append(out, *e)
		}
	}
	return out
}
