package main

import (
	"sync"

	meowcaller "github.com/purpshell/meowcaller"
)

// CallEntry é uma chamada ativa registrada no CallManager.
type CallEntry struct {
	Call   *meowcaller.Call
	UserID string // ID do usuário WuzAPI dono da sessão
}

// callManager é um registry thread-safe de chamadas ativas, keyed por callID.
var callManager = &CallManager{calls: make(map[string]*CallEntry)}

type CallManager struct {
	mu    sync.RWMutex
	calls map[string]*CallEntry
}

func (m *CallManager) Register(callID, userID string, call *meowcaller.Call) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[callID] = &CallEntry{Call: call, UserID: userID}
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

func (m *CallManager) Delete(callID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.calls, callID)
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
