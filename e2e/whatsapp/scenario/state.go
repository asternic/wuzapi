package scenario

import "wuzapi/e2e/apiclient"

type State struct {
	config Config
	api    *apiclient.Client
}

func New() *State {
	config := LoadConfig()
	return &State{
		config: config,
		api:    apiclient.New(config.APIBaseURL, config.APIAdminToken),
	}
}

func (state *State) Config() Config {
	return state.config
}

func (state *State) API() *apiclient.Client {
	return state.api
}

func (state *State) Close() error {
	return nil
}
