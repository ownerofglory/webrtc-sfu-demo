package handler

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/ownerofglory/webrtc-sfu-demo/config"
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/domain"
	"github.com/ownerofglory/webrtc-sfu-demo/internal/core/ports"
	"github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const WSPath = basePathWS + "/{roomId}"
const WSAppPath = basePathWS + "/app"

type (
	wsHandler struct {
		upgrader          *websocket.Upgrader
		sfuHandler        *sfu.SFU
		configFetcher     ports.RTCConfigFetcher
		nicknameGenerator ports.NicknameGenerator
		roomNameGenerator ports.RoomNameGenerator
	}

	WebRTCClientID string

	WebRTCSignalingMessageType string
	WebrtcSignalingMessageSDP  string

	WebRTCSignalingMessage struct {
		MessageType   WebRTCSignalingMessageType `json:"type,omitempty"`
		SDP           WebrtcSignalingMessageSDP  `json:"sdp,omitempty"`
		Candidate     string                     `json:"candidate,omitempty"`
		SDPMid        string                     `json:"sdpMid,omitempty"`
		SDPMLineIndex *uint16                    `json:"sdpMLineIndex,omitempty"`
	}

	WebRTCClientMessage struct {
		RoomID           string                  `json:"room,omitempty"`
		SignalingMessage *WebRTCSignalingMessage `json:"signal"`
		ReceiverPeerID   WebRTCClientID          `json:"to"`
		OriginPeerID     WebRTCClientID          `json:"from"`
	}

	webRTCClientConn struct {
		conn              *websocket.Conn
		id                WebRTCClientID
		connCloseOnce     sync.Once
		sfuPeer           *sfu.PeerLocal
		readCh            chan *WebRTCClientMessage
		writeCh           chan *WebRTCClientMessage
		nicknameGenerator ports.NicknameGenerator
	}

	webRTCRoom struct {
		id      string
		members map[WebRTCClientID]*webRTCClientConn
		session sfu.Session
		cfg     *sfu.WebRTCTransportConfig
	}

	webRTCRoomMap map[string]*webRTCRoom
)

const (
	webrtcOffer     WebRTCSignalingMessageType = "offer"
	webrtcAnswer    WebRTCSignalingMessageType = "answer"
	webrtcCandidate WebRTCSignalingMessageType = "candidate"
)

var (
	rooms   webRTCRoomMap = make(map[string]*webRTCRoom)
	roomsMx sync.RWMutex
)

var (
	webRTCConnections = make(map[WebRTCClientID]*webRTCClientConn)
	connMx            sync.RWMutex
)

func (r webRTCRoomMap) GetSession(sid string) (sfu.Session, sfu.WebRTCTransportConfig) {
	roomsMx.RLock()
	room, ok := r[sid]
	roomsMx.RUnlock()
	if !ok {
		slog.Warn("room not found")
		return nil, sfu.WebRTCTransportConfig{}
	}

	return room.session, *room.cfg
}

func NewWSHandler(conf *config.WebRTCSFUAppConfig,
	sfuHandler *sfu.SFU,
	configFetcher ports.RTCConfigFetcher,
	nicknameGenerator ports.NicknameGenerator,
	roomNameGenerator ports.RoomNameGenerator) *wsHandler {
	return &wsHandler{
		nicknameGenerator: nicknameGenerator,
		roomNameGenerator: roomNameGenerator,
		configFetcher:     configFetcher,
		sfuHandler:        sfuHandler,
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
	ctx, cancel := context.WithCancel(req.Context())
	conn, err := h.upgrader.Upgrade(rw, req, nil)
	if err != nil {
		slog.Error("Error when upgrading to websocket", "err", err.Error())
		return
	}
	defer conn.Close()

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

	roomID := req.PathValue("roomId")
	if roomID == "" {
		roomID = h.roomNameGenerator.Generate()
	}

	var room *webRTCRoom
	var ok bool
	var session sfu.Session
	if room, ok = rooms[roomID]; !ok {
		var conf domain.WebRTCConfig
		conf, err = h.configFetcher.FetchConfig(1 * time.Hour)
		if err != nil {
			slog.Error("Error when fetching config", "err", err)
			conf = domain.WebRTCConfig{}
		}
		var iceServers []sfu.ICEServerConfig
		for _, ice := range conf.ICEServers {
			iceServers = append(iceServers, sfu.ICEServerConfig{
				URLs:       ice.URLs,
				Username:   ice.Username,
				Credential: ice.Credential,
			})
		}
		sfuConfig := sfu.Config{
			WebRTC: sfu.WebRTCConfig{
				ICEServers: iceServers,
			},
		}

		wCfg := sfu.NewWebRTCTransportConfig(sfuConfig)
		session = sfu.NewSession(roomID, nil, wCfg)
		room = &webRTCRoom{
			id:      roomID,
			members: make(map[WebRTCClientID]*webRTCClientConn),
			session: session,
			cfg:     &wCfg,
		}

		roomsMx.Lock()
		rooms[roomID] = room
		roomsMx.Unlock()
		slog.Debug("Created new room", "room", roomID)
	}

	roomsMx.Lock()
	room.members[clientID] = clientConn
	roomsMx.Unlock()
	slog.Debug("Connected to room", "room", roomID, "client", clientID)

	connMx.Lock()
	webRTCConnections[clientID] = clientConn
	connMx.Unlock()

	slog.Debug("Client connected", "clientID", clientID)
	initialMsg := &WebRTCClientMessage{
		OriginPeerID: clientID,
		RoomID:       roomID,
	}
	payload, _ := json.Marshal(initialMsg)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		slog.Error("Error sending initial nickname", "err", err.Error())
	}

	peerLocal := sfu.NewPeer(rooms)
	defer func() {
		session.RemovePeer(peerLocal)
		roomsMx.Lock()
		room := rooms[roomID]
		delete(room.members, clientID)

		if len(room.members) == 0 {
			delete(rooms, roomID)
		}
		roomsMx.Unlock()
	}()
	peerLocal.OnIceCandidate = func(c *webrtc.ICECandidateInit, i int) {
		msg := WebRTCClientMessage{
			RoomID:       roomID,
			OriginPeerID: clientID,
			SignalingMessage: &WebRTCSignalingMessage{
				MessageType:   webrtcCandidate,
				SDPMid:        *c.SDPMid,
				SDPMLineIndex: c.SDPMLineIndex,
			},
		}
		_ = conn.WriteJSON(msg)
	}

	peerLocal.OnOffer = func(off *webrtc.SessionDescription) {
		_ = conn.WriteJSON(WebRTCClientMessage{
			RoomID: roomID,
			SignalingMessage: &WebRTCSignalingMessage{
				MessageType: webrtcOffer,
				SDP:         WebrtcSignalingMessageSDP(off.SDP),
			},
			OriginPeerID: clientID,
		})
	}

	if err = peerLocal.Join(roomID, string(clientID)); err != nil {
		slog.Error("Error joining room", "room", roomID, "client", clientID, "err", err.Error())
		cancel()
	}

	go func(ctx context.Context) {
		for {
			var m WebRTCClientMessage
			err = clientConn.conn.ReadJSON(&m)
			if err != nil {
				slog.Error("Error when reading websocket message", "err", err.Error())
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					break
				}
				break
			}

			if m.SignalingMessage == nil {
				continue
			}

			switch m.SignalingMessage.MessageType {
			case webrtcOffer:
				slog.Debug("Received offer", "from", m.OriginPeerID)
				offer := webrtc.SessionDescription{
					SDP:  string(m.SignalingMessage.SDP),
					Type: webrtc.SDPTypeOffer,
				}
				answer, err := peerLocal.Answer(offer)
				if err != nil {
					slog.Error("Unable to set remote description", "err", err)
					return
				}
				_ = conn.WriteJSON(WebRTCClientMessage{
					RoomID: roomID,
					SignalingMessage: &WebRTCSignalingMessage{
						MessageType: webrtcAnswer,
						SDP:         WebrtcSignalingMessageSDP(answer.SDP),
					},
					OriginPeerID: clientID,
				})

			case webrtcCandidate:
				_ = peerLocal.Trickle(webrtc.ICECandidateInit{
					Candidate:     m.SignalingMessage.Candidate,
					SDPMid:        &m.SignalingMessage.SDPMid,
					SDPMLineIndex: m.SignalingMessage.SDPMLineIndex,
				}, 0)
			case webrtcAnswer:
				_ = peerLocal.SetRemoteDescription(webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  string(m.SignalingMessage.SDP),
				})
			default:
				slog.Warn("Unsupported signaling message", "type", m.SignalingMessage.MessageType)
				continue
			}
		}

		cancel()
	}(ctx)

	<-ctx.Done()
}
