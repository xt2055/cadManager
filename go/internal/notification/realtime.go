package notification

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"cadguanliq/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/websocket"
)

// Hub multiplexes one PostgreSQL LISTEN connection to all local WebSockets.
// Each server instance receives committed events, including those from other
// instances. Slow clients coalesce invalidations and fetch the latest inbox.
type Hub struct {
	mu      sync.Mutex
	ready   bool
	clients map[string]map[chan struct{}]struct{}
}

func NewHub() *Hub { return &Hub{clients: make(map[string]map[chan struct{}]struct{})} }

func (h *Hub) subscribe(userID string) (chan struct{}, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.ready {
		return nil, func() {}, false
	}
	ch := make(chan struct{}, 1)
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[chan struct{}]struct{})
	}
	h.clients[userID][ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.clients[userID][ch]; ok {
			delete(h.clients[userID], ch)
			close(ch)
		}
		if len(h.clients[userID]) == 0 {
			delete(h.clients, userID)
		}
	}, true
}

func (h *Hub) publish(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients[userID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *Hub) disconnect() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = false
	for _, clients := range h.clients {
		for ch := range clients {
			close(ch)
		}
	}
	h.clients = make(map[string]map[chan struct{}]struct{})
}

func (h *Hub) Run(ctx context.Context, pool *pgxpool.Pool) {
	for ctx.Err() == nil {
		err := h.listen(ctx, pool)
		h.disconnect()
		if ctx.Err() != nil {
			return
		}
		log.Printf("[notifications] 实时通知连接中断，准备重连: %v", err)
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (h *Hub) listen(ctx context.Context, pool *pgxpool.Pool) error {
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	// Dedicated connection does not occupy a pooled request slot indefinitely.
	conn, err := pgx.ConnectConfig(connectCtx, pool.Config().ConnConfig.Copy())
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err = conn.Exec(ctx, "LISTEN cad_notifications"); err != nil {
		return err
	}
	h.mu.Lock()
	h.ready = true
	h.mu.Unlock()
	for {
		event, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		h.publish(event.Payload)
	}
}

type authenticator interface {
	CurrentUser(context.Context, string) (auth.AuthUser, error)
}

func (h *Hub) Handler(service authenticator, allowedOrigins []string) http.Handler {
	server := websocket.Server{
		Handshake: func(_ *websocket.Config, r *http.Request) error {
			origin := r.Header.Get("Origin")
			parsed, err := url.Parse(origin)
			sameOrigin := err == nil && parsed.Host == r.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
			if !sameOrigin && !slices.Contains(allowedOrigins, origin) && !slices.Contains(allowedOrigins, "*") {
				return fmt.Errorf("不允许的来源")
			}
			return nil
		},
		Handler: func(conn *websocket.Conn) { h.serve(conn, service) },
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		server.ServeHTTP(w, r)
	})
}

func (h *Hub) serve(conn *websocket.Conn, service authenticator) {
	defer conn.Close()
	conn.MaxPayloadBytes = 4096
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	// Browser WebSockets cannot attach Authorization headers. Authenticate the
	// first frame so the session token never appears in URLs or access logs.
	var hello struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := websocket.JSON.Receive(conn, &hello); err != nil || hello.Type != "auth" || hello.Token == "" {
		return
	}
	authenticate := func() (auth.AuthUser, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return service.CurrentUser(ctx, hello.Token)
	}
	send := func(kind string) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return websocket.JSON.Send(conn, map[string]string{"type": kind})
	}
	user, err := authenticate()
	if err != nil {
		_ = send("unauthorized")
		return
	}
	changes, unsubscribe, ok := h.subscribe(user.ID)
	if !ok {
		_ = send("unavailable")
		return
	}
	defer unsubscribe()
	if err = send("ready"); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
			var pong struct {
				Type string `json:"type"`
			}
			if err := websocket.JSON.Receive(conn, &pong); err != nil || pong.Type != "pong" {
				return
			}
		}
	}()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-done:
			return
		case _, ok := <-changes:
			if !ok {
				return
			}
			if err = send("changed"); err != nil {
				return
			}
		case <-heartbeat.C:
			current, authErr := authenticate()
			if authErr != nil || current.ID != user.ID {
				_ = send("unauthorized")
				return
			}
			if err = send("ping"); err != nil {
				return
			}
		}
	}
}
