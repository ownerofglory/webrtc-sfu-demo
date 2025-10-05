package handler

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/ownerofglory/webrtc-sfu-demo/config"
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/ports"
	"log/slog"
	"net/http"
	"sync"
)

const WSPath = basePathWS
const WSAppPath = basePathWS + "/app"

type (
	wsHandler struct {
		upgrader          *websocket.Upgrader
		nicknameGenerator ports.NicknameGenerator
	}

	WebRTCClientID string

	WebRTCSignalingMessageType string
	WebrtcSignalingMessageSDP  string

	WebRTCSignalingMessage struct {
		MessageType   WebRTCSignalingMessageType `json:"type,omitempty"`
		SDP           WebrtcSignalingMessageSDP  `json:"sdp,omitempty"`
		Candidate     string                     `json:"candidate,omitempty"`
		SDPMid        string                     `json:"sdpMid,omitempty"`
		SDPMLineIndex *int                       `json:"sdpMLineIndex,omitempty"`
	}

	WebRTCClientMessage struct {
		SignalingMessage *WebRTCSignalingMessage `json:"signal"`
		ReceiverPeerID   WebRTCClientID          `json:"to"`
		OriginPeerID     WebRTCClientID          `json:"from"`
	}

	webRTCClientConn struct {
		conn              *websocket.Conn
		id                WebRTCClientID
		connCloseOnce     sync.Once
		readCh            chan *WebRTCClientMessage
		writeCh           chan *WebRTCClientMessage
		nicknameGenerator ports.NicknameGenerator
	}
)

const (
	webrtcOffer     WebRTCSignalingMessageType = "offer"
	webrtcAnswer    WebRTCSignalingMessageType = "answer"
	webrtcCandidate WebRTCSignalingMessageType = "candidate"
)

var (
	webRTCConnections = make(map[WebRTCClientID]*webRTCClientConn)
	connMx            sync.RWMutex
)

func NewWSHandler(conf *config.WebRTCSFUAppConfig, nicknameGenerator ports.NicknameGenerator) *wsHandler {
	return &wsHandler{
		nicknameGenerator: nicknameGenerator,
		upgrader: &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return false
				}

				for _, allowed := range conf.AllowedOrigins {
					if origin == allowed {
						return true
					}
				}

				return false
			},
		},
	}
}

func (h *wsHandler) HandleWS(rw http.ResponseWriter, req *http.Request) {
	conn, err := h.upgrader.Upgrade(rw, req, nil)
	if err != nil {
		slog.Error("Error when upgrading to websocket", "err", err.Error())
		return
	}

	clientNickname := h.nicknameGenerator.Generate()
	clientID := WebRTCClientID(clientNickname)

	clientConn := &webRTCClientConn{
		conn:              conn,
		id:                clientID,
		readCh:            make(chan *WebRTCClientMessage),
		writeCh:           make(chan *WebRTCClientMessage),
		nicknameGenerator: h.nicknameGenerator,
	}
	defer close(clientConn.readCh)
	defer close(clientConn.writeCh)
	defer func() {
		delete(webRTCConnections, clientID)
		h.nicknameGenerator.Release(clientNickname)
	}()

	connMx.Lock()
	webRTCConnections[clientID] = clientConn
	connMx.Unlock()

	slog.Info("Client connected", "clientID", clientID)
	initialMsg := &WebRTCClientMessage{
		OriginPeerID: clientID,
	}
	payload, _ := json.Marshal(initialMsg)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		slog.Error("Error sending initial nickname", "err", err.Error())
	}

	ctx, cancel := context.WithCancel(req.Context())

	go clientConn.writeRoutine(ctx)
	go clientConn.processMessage(ctx)
	for {
		var m WebRTCClientMessage
		err := clientConn.conn.ReadJSON(&m)
		if err != nil {
			slog.Error("Error when reading websocket message", "err", err.Error())
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				break
			}
			break
		}
		clientConn.readCh <- &m
	}
	cancel()
}

func (c *webRTCClientConn) writeRoutine(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-c.readCh:
			if !ok {
				return
			}
			err := c.conn.WriteJSON(m)
			if err != nil {
				slog.Error("Error when sending message", "err", err.Error())
				return
			}
		}
	}
}

func (c *webRTCClientConn) processMessage(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-c.readCh:
			if !ok {
				return
			}
			func() {
				connMx.RLock()
				defer connMx.RUnlock()

				recepient := webRTCConnections[m.ReceiverPeerID]
				if recepient == nil {
					slog.Error("Error when receiving message from client", "clientId", c.id)
					return
				}

				m.OriginPeerID = c.id

				recepient.writeCh <- m
			}()
		}
	}
}
