package main

import (
	"sync"

	"github.com/go-resty/resty/v2"
	"go.mau.fi/whatsmeow"
)

type ClientManager struct {
	sync.RWMutex
	whatsmeowClients map[string]*whatsmeow.Client
	httpClients      map[string]*resty.Client
	myClients        map[string]*MyClient
	// pollOptions stores the plaintext options sent for each poll, keyed on
	// userID then on the poll's message ID. This lets the event handler
	// SHA-256-match incoming vote hashes back to the original option text
	// before emitting the webhook payload. Entries are best-effort and
	// in-memory only — if wuzapi restarts between send and vote, plaintext
	// resolution is skipped and the webhook falls back to hashes only.
	pollOptions map[string]map[string][]string
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		whatsmeowClients: make(map[string]*whatsmeow.Client),
		httpClients:      make(map[string]*resty.Client),
		myClients:        make(map[string]*MyClient),
		pollOptions:      make(map[string]map[string][]string),
	}
}

func (cm *ClientManager) SetWhatsmeowClient(userID string, client *whatsmeow.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.whatsmeowClients[userID] = client
}

func (cm *ClientManager) GetWhatsmeowClient(userID string) *whatsmeow.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.whatsmeowClients[userID]
}

func (cm *ClientManager) DeleteWhatsmeowClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.whatsmeowClients, userID)
}

func (cm *ClientManager) SetHTTPClient(userID string, client *resty.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.httpClients[userID] = client
}

func (cm *ClientManager) GetHTTPClient(userID string) *resty.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.httpClients[userID]
}

func (cm *ClientManager) DeleteHTTPClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.httpClients, userID)
}

func (cm *ClientManager) SetMyClient(userID string, client *MyClient) {
	cm.Lock()
	defer cm.Unlock()
	cm.myClients[userID] = client
}

func (cm *ClientManager) GetMyClient(userID string) *MyClient {
	cm.RLock()
	defer cm.RUnlock()
	return cm.myClients[userID]
}

// DeleteMyClient removes a user's MyClient entry and clears any cached
// poll options associated with that user (best-effort; callers that only
// want to detach the MyClient should call clientManager.myClients delete
// directly, but no such caller exists today).
func (cm *ClientManager) DeleteMyClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.myClients, userID)
	delete(cm.pollOptions, userID)
}

// SetPollOptions remembers the plaintext options of a poll we just sent so
// that incoming votes (which arrive as SHA-256 hashes of the option text)
// can be resolved back to readable strings.
func (cm *ClientManager) SetPollOptions(userID, msgID string, options []string) {
	cm.Lock()
	defer cm.Unlock()
	if cm.pollOptions[userID] == nil {
		cm.pollOptions[userID] = make(map[string][]string)
	}
	stored := make([]string, len(options))
	copy(stored, options)
	cm.pollOptions[userID][msgID] = stored
}

// GetPollOptions returns the plaintext options associated with a poll
// message, or nil if none were recorded (e.g. wuzapi restarted after the
// poll was sent).
func (cm *ClientManager) GetPollOptions(userID, msgID string) []string {
	cm.RLock()
	defer cm.RUnlock()
	if byUser := cm.pollOptions[userID]; byUser != nil {
		return byUser[msgID]
	}
	return nil
}
