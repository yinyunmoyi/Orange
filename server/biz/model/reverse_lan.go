package model

type ReverseLanRegistrationRequest struct {
	APIVersion int    `json:"apiVersion"`
	ClientID   string `json:"clientId"`
	ServerID   string `json:"serverId"`
	Generation string `json:"generation"`
	Secret     string `json:"secret"`
}

type ReverseLanRegistrationData struct {
	ExpiresInSeconds int `json:"expiresInSeconds"`
}

type ReverseLanRegistrationResponse struct {
	Code int                         `json:"code"`
	Msg  string                      `json:"msg,omitempty"`
	Data *ReverseLanRegistrationData `json:"data,omitempty"`
}
