package services

import (
	"fmt"
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/domain"
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/ports"
	"log/slog"
	"time"
)

type rtcConfigFetcher struct {
	client ports.RTCConfigClient
}

func NewRTCConfigFetcher(client ports.RTCConfigClient) *rtcConfigFetcher {
	return &rtcConfigFetcher{
		client: client,
	}
}

func (r *rtcConfigFetcher) FetchConfig(duration time.Duration) (domain.WebRTCConfig, error) {
	config, err := r.client.GetConfig(duration)
	if err != nil {
		slog.Error("Error when fetching config", "err", err)
		return domain.WebRTCConfig{}, fmt.Errorf("error when fetching config: %w", err)
	}

	return *config, nil
}
