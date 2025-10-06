package ports

import (
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/domain"
	"time"
)

type RTCConfigFetcher interface {
	FetchConfig(duration time.Duration) (domain.WebRTCConfig, error)
}

type RTCConfigClient interface {
	GetConfig(duration time.Duration) (*domain.WebRTCConfig, error)
}
