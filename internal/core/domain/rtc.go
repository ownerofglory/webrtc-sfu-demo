package domain

type WebRTCConfig struct {
	ICEServers []struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username,omitempty"`
		Credential string   `json:"credential,omitempty"`
	} `json:"iceServers"`
	TTL *int `json:"ttl"`
}
