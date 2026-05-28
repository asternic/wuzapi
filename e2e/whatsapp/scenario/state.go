package scenario

import (
	"errors"

	"wuzapi/e2e/apiclient"
	"wuzapi/e2e/appium"
)

type State struct {
	config    Config
	device    *appium.Session
	api       *apiclient.Client
	user      *apiclient.ScenarioUser
	pairPhone string
	pairCode  string
}

func New() *State {
	config := LoadConfig()
	return &State{
		config:    config,
		api:       apiclient.New(config.APIBaseURL, config.APIAdminToken),
		pairPhone: config.PairPhone,
	}
}

func (state *State) Config() Config {
	return state.config
}

func (state *State) API() *apiclient.Client {
	return state.api
}

func (state *State) User() *apiclient.ScenarioUser {
	return state.user
}

func (state *State) SetUser(user *apiclient.ScenarioUser) {
	state.user = user
}

func (state *State) PairPhone() string {
	return state.pairPhone
}

func (state *State) SetPairPhone(pairPhone string) {
	state.pairPhone = pairPhone
}

func (state *State) PairCode() string {
	return state.pairCode
}

func (state *State) SetPairCode(pairCode string) {
	state.pairCode = pairCode
}

func (state *State) SetDevice(device *appium.Session) {
	state.device = device
}

func (state *State) Device(message string) (*appium.Session, error) {
	if state.device == nil {
		return nil, errors.New(message)
	}

	return state.device, nil
}

func (state *State) Close() error {
	if state == nil || state.device == nil {
		return nil
	}

	return state.device.Close()
}
