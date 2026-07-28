package main

import (
	"sync"

	meowcaller "github.com/purpshell/meowcaller"
	"go.mau.fi/whatsmeow/types"
)

// CallEntry é uma chamada ativa registrada no CallManager.
type CallEntry struct {
	Call       *meowcaller.Call
	UserID     string   // ID do usuário WuzAPI dono da sessão
	IsIncoming bool     // true = entrante (precisa de Answer); false = saída
	PreSink    *preSink // não-nil para chamadas saintes: buffer de frames até o WS conectar

	mu           sync.Mutex
	endListeners []func(reason string) // ouvintes do término da chamada (ver AddEndListener)
	ended        bool
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

// Register cria e armazena a entrada da chamada, e é o ÚNICO ponto que chama
// call.OnEnd — a lib meowcaller só guarda um callback por chamada (não uma lista),
// então qualquer outra parte que precise ser avisada do término deve usar
// AddEndListener em vez de chamar call.OnEnd diretamente (o que sobrescreveria
// silenciosamente este registro).
func (m *CallManager) Register(callID, userID string, call *meowcaller.Call, isIncoming bool) *CallEntry {
	entry := &CallEntry{Call: call, UserID: userID, IsIncoming: isIncoming}
	m.mu.Lock()
	m.calls[callID] = entry
	m.mu.Unlock()

	call.OnEnd(func(reason string) {
		m.fireEnd(callID, reason)
	})

	return entry
}

// AddEndListener registra um callback a ser executado quando a chamada terminar.
// Suporta múltiplos ouvintes (diferente de call.OnEnd, que só guarda o último
// registrado). Se a chamada já tiver terminado quando este método é chamado,
// dispara o callback imediatamente.
func (m *CallManager) AddEndListener(callID string, fn func(reason string)) {
	m.mu.RLock()
	entry, ok := m.calls[callID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	entry.mu.Lock()
	if entry.ended {
		entry.mu.Unlock()
		fn("already-ended")
		return
	}
	entry.endListeners = append(entry.endListeners, fn)
	entry.mu.Unlock()
}

// fireEnd dispara todos os ouvintes de término registrados para a chamada.
// Chamado uma única vez, pelo único call.OnEnd wired em Register.
func (m *CallManager) fireEnd(callID, reason string) {
	m.mu.RLock()
	entry, ok := m.calls[callID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	entry.mu.Lock()
	entry.ended = true
	listeners := entry.endListeners
	entry.mu.Unlock()

	for _, fn := range listeners {
		fn(reason)
	}
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

func (m *CallManager) ListByUser(userID string) []*CallEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*CallEntry
	for _, e := range m.calls {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	return out
}
