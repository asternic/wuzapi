package main

import (
	"sync"

	"github.com/go-resty/resty/v2"
	"go.mau.fi/whatsmeow"
)

type ClientManager struct {
	sync.RWMutex
	whatsmeowClients map[int]*whatsmeow.Client
	httpClients      map[int]*resty.Client
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		whatsmeowClients: make(map[int]*whatsmeow.Client),
		httpClients:      make(map[int]*resty.Client),
	}
}

func (cm *ClientManager) SetWhatsmeowClient(userID int, client *whatsmeow.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.whatsmeowClients[userID] = client
}

func (cm *ClientManager) GetWhatsmeowClient(userID int) *whatsmeow.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.whatsmeowClients[userID]
}

func (cm *ClientManager) DeleteWhatsmeowClient(userID int) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.whatsmeowClients, userID)
}

func (cm *ClientManager) SetHTTPClient(userID int, client *resty.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.httpClients[userID] = client
}

func (cm *ClientManager) GetHTTPClient(userID int) *resty.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.httpClients[userID]
}

func (cm *ClientManager) DeleteHTTPClient(userID int) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.httpClients, userID)
}
