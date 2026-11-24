package server

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/stevenlagoy/ramserver-core/server/internal/admin"
	"github.com/stevenlagoy/ramserver-core/server/internal/session"
)

// How long an admin request waits for a session to answer before reporting it unresponsive
const sessionQueryTimeout = 2 * time.Second

// Identity of an unauthenticated client
type clientMeta struct {
	playerID    string
	displayName string
}

// Server control surface for authentication
type Hub struct {
	Metrics  *admin.Metrics
	mu       sync.Mutex
	clients  map[string]*Client
	nextID   uint64
	sessions *session.SessionStore
}

var _ admin.Controller = (*Hub)(nil)

func NewHub() *Hub {
	return &Hub{Metrics: admin.NewMetrics(), clients: make(map[string]*Client)}
}

func (h *Hub) attach(sessions *session.SessionStore) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions = sessions
	sessions.Metrics = h.Metrics
}

func (h *Hub) store() *session.SessionStore {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions
}

// Registers c and returns the function which unregisters it
func (h *Hub) add(c *Client) (remove func()) {
	h.mu.Lock()
	h.nextID++
	id := fmt.Sprintf("conn-%d", h.nextID)
	c.connID, c.connSeq, c.connectedAt, c.metrics = id, h.nextID, time.Now(), h.Metrics
	h.clients[id] = c
	h.mu.Unlock()
	h.Metrics.Inc(admin.ConnectionsAccepted)
	return func() {
		h.mu.Lock()
		delete(h.clients, id)
		h.mu.Unlock()
	}
}

func (h *Hub) Sessions(ctx context.Context) ([]admin.SessionInfo, error) {
	st := h.store()
	if st == nil {
		return nil, nil
	}
	list := st.List("", false)
	out := make([]admin.SessionInfo, 0, len(list))
	for _, s := range list {
		out = append(out, sessionInfo(ctx, s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

func sessionInfo(ctx context.Context, s *session.Session) admin.SessionInfo {
	qctx, cancel := context.WithTimeout(ctx, sessionQueryTimeout)
	defer cancel()
	info, err := s.Info(qctx)
	if err != nil {
		return admin.SessionInfo{ID: s.ID, Game: s.GameName, Public: s.Public, Status: "unresponsive", Members: []admin.MemberInfo{}}
	}
	return info
}

func (h *Hub) Session(ctx context.Context, id string) (admin.SessionInfo, error) {
	st := h.store()
	if st == nil {
		return admin.SessionInfo{}, admin.ErrNotFound
	}
	s, ok := st.Get(id)
	if !ok {
		return admin.SessionInfo{}, admin.ErrNotFound
	}
	return sessionInfo(ctx, s), nil
}

// Closes the session and removes it from the store
func (h *Hub) CloseSession(ctx context.Context, id, reason string) error {
	st := h.store()
	if st == nil {
		return admin.ErrNotFound
	}
	s, ok := st.Get(id)
	if !ok {
		return admin.ErrNotFound
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.Close(cctx, reason); err != nil {
		log.Printf("admin: session %s did not answer (%v); removing it from the store", id, err)
	}
	st.Remove(id)
	h.Metrics.Inc(admin.SessionsClosedByAdmin)
	return nil
}

func (h *Hub) Connections(ctx context.Context) ([]admin.ConnInfo, error) {
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	sessionOf := map[string]string{} // player id -> session id
	if sessions, err := h.Sessions(ctx); err == nil {
		for _, s := range sessions {
			for _, m := range s.Members {
				sessionOf[m.PlayerID] = s.ID
			}
		}
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].connSeq < clients[j].connSeq })
	out := make([]admin.ConnInfo, 0, len(clients))
	for _, c := range clients {
		info := c.info()
		info.SessionID = sessionOf[info.PlayerID]
		out = append(out, info)
	}
	return out, nil
}

// Closes the TCP connection and frees seats of any connected clients
func (h *Hub) CloseConnection(_ context.Context, id string) error {
	h.mu.Lock()
	c, ok := h.clients[id]
	h.mu.Unlock()
	if !ok {
		return admin.ErrNotFound
	}
	c.kicked.Store(true)
	c.connection.Close()
	h.Metrics.Inc(admin.ConnectionsKicked)
	return nil
}

func (h *Hub) Gauges() map[string]int64 {
	h.mu.Lock()
	conns := int64(len(h.clients))
	st := h.sessions
	h.mu.Unlock()
	g := map[string]int64{"connections": conns}
	if st != nil {
		g["sessions"] = int64(len(st.List("", false)))
	}
	return g
}

func (c *Client) info() admin.ConnInfo {
	info := admin.ConnInfo{
		ID:               c.connID,
		Remote:           c.connection.RemoteAddr().String(),
		Connected:        c.connectedAt,
		MessagesReceived: c.msgsIn.Load(),
	}
	if m := c.meta.Load(); m != nil {
		info.Authenticated, info.PlayerID, info.DisplayName = true, m.playerID, m.displayName
	}
	return info
}

// Frees the client's seat after an administrator closes a connection
func (c *Client) leaveSessionOnExit(serverCtx context.Context) {
	if !c.kicked.Load() || c.session == nil || serverCtx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c.session.Leave(ctx, c.ID)
	c.session = nil
}
