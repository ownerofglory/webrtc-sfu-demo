package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/domain"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	cloudFlareURL = "https://rtc.live.cloudflare.com/v1/turn/keys"
	defaultTTL    = 60 * 60
)

type configRequest struct {
	TTL int `json:"ttl"`
}

type config struct {
	ICEServers []struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username,omitempty"`
		Credential string   `json:"credential,omitempty"`
	} `json:"iceServers"`
	TTL *int `json:"ttl"`
}

type cloudFlareRTCConfigClient struct {
	turnKey      string
	turnAPIToken string
	client       *http.Client
}

func NewClient(turnKey, turnAPIToken string, client *http.Client) *cloudFlareRTCConfigClient {
	return &cloudFlareRTCConfigClient{
		turnKey:      turnKey,
		turnAPIToken: turnAPIToken,
		client:       client,
	}
}

func (c *cloudFlareRTCConfigClient) GetConfig(duration time.Duration) (*domain.WebRTCConfig, error) {
	url := fmt.Sprintf("%s/%s/credentials/generate-ice-servers", cloudFlareURL, c.turnKey)
	ttl := duration.Seconds()
	if ttl <= 0 {
		ttl = defaultTTL
	}
	reqBody := &configRequest{
		TTL: int(ttl),
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		slog.Error("Error marshalling request body", "error", err)
		return nil, fmt.Errorf("Error marshalling request body: %w", err)
	}

	clientReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		slog.Error("Error creating request", "error", err)
		return nil, fmt.Errorf("Error creating request: %w", err)
	}
	clientReq.Header.Add("Authorization", "Bearer "+c.turnAPIToken)
	clientReq.Header.Add("Content-Type", "application/json")

	res, err := c.client.Do(clientReq)
	if err != nil {
		slog.Error("Error getting rtc config", "error", err)
		return nil, fmt.Errorf("error getting cloudflare rtc config: %w", err)
	}
	defer res.Body.Close()

	respPayload, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error("Error reading response body", "error", err)
		return nil, fmt.Errorf("error reading cloudflare response body: %w", err)
	}

	cloudFlareConfig := config{}
	err = json.Unmarshal(respPayload, &cloudFlareConfig)
	if err != nil {
		slog.Error("Error unmarshalling response body", "error", err)
		return nil, fmt.Errorf("error unmarshalling cloudflare response body: %w", err)
	}

	cloudFlareConfig.TTL = &reqBody.TTL

	return &domain.WebRTCConfig{
		TTL:        cloudFlareConfig.TTL,
		ICEServers: cloudFlareConfig.ICEServers,
	}, nil

}
