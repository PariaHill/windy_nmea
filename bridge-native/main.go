package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows/registry"
)

const (
	appName      = "WindyNMEABridge"
	defaultHost  = "127.0.0.1"
	defaultPort  = 8787
	registryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
)

//go:embed settings.html
var settingsHTML string

type Config struct {
	GPSHost             string `json:"gpsHost"`
	GPSPort             int    `json:"gpsPort"`
	ListenHost          string `json:"listenHost"`
	ListenPort          int    `json:"listenPort"`
	RunAtStartup        bool   `json:"runAtStartup"`
	StartReceivingOnRun bool   `json:"startReceivingOnRun"`
}

type Status struct {
	Running     bool   `json:"running"`
	Connected   bool   `json:"connected"`
	ClientCount int    `json:"clientCount"`
	LastError   string `json:"lastError"`
	LastDataAt  string `json:"lastDataAt"`
}

type App struct {
	mu       sync.Mutex
	config   Config
	status   Status
	clients  map[*websocket.Conn]bool
	upgrader websocket.Upgrader
	cancel   context.CancelFunc
}

func defaultConfig() Config {
	return Config{
		GPSHost:             "192.168.0.10",
		GPSPort:             5017,
		ListenHost:          defaultHost,
		ListenPort:          defaultPort,
		RunAtStartup:        false,
		StartReceivingOnRun: false,
	}
}

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName, "config.json"), nil
}

func loadConfig() Config {
	cfg := defaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig()
	}
	if cfg.ListenHost == "" {
		cfg.ListenHost = defaultHost
	}
	if cfg.ListenPort == 0 {
		cfg.ListenPort = defaultPort
	}
	return cfg
}

func saveConfig(cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.GPSHost) == "" {
		return errors.New("GPS IP is required")
	}
	if cfg.GPSPort < 1 || cfg.GPSPort > 65535 {
		return errors.New("GPS port must be between 1 and 65535")
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return errors.New("local bridge port must be between 1 and 65535")
	}
	return nil
}

func setRunAtStartup(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		if err := key.DeleteValue(appName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	return key.SetStringValue(appName, fmt.Sprintf(`"%s" --background`, exe))
}

func NewApp(cfg Config) *App {
	return &App{
		config:  cfg,
		clients: map[*websocket.Conn]bool{},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func (a *App) snapshot() (Config, Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	status := a.status
	status.ClientCount = len(a.clients)
	return a.config, status
}

func (a *App) setError(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.LastError = message
}

func (a *App) setConnected(connected bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Connected = connected
	if connected {
		a.status.LastError = ""
	}
}

func (a *App) broadcast(data []byte) {
	a.mu.Lock()
	a.status.LastDataAt = time.Now().Format(time.RFC3339)
	clients := make([]*websocket.Conn, 0, len(a.clients))
	for client := range a.clients {
		clients = append(clients, client)
	}
	a.mu.Unlock()

	for _, client := range clients {
		if err := client.WriteMessage(websocket.TextMessage, data); err != nil {
			client.Close()
			a.mu.Lock()
			delete(a.clients, client)
			a.mu.Unlock()
		}
	}
}

func (a *App) startReceiving() {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg := a.config
	a.cancel = cancel
	a.status.Running = true
	a.status.LastError = ""
	a.mu.Unlock()

	go a.receiveLoop(ctx, cfg)
}

func (a *App) stopReceiving() {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.status.Running = false
	a.status.Connected = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) receiveLoop(ctx context.Context, cfg Config) {
	address := net.JoinHostPort(cfg.GPSHost, strconv.Itoa(cfg.GPSPort))
	buffer := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			a.setConnected(false)
			a.setError(err.Error())
			sleepOrDone(ctx, 2*time.Second)
			continue
		}

		a.setConnected(true)
		for {
			n, err := conn.Read(buffer)
			if n > 0 {
				a.broadcast(buffer[:n])
			}
			if err != nil {
				conn.Close()
				a.setConnected(false)
				if !errors.Is(err, io.EOF) {
					a.setError(err.Error())
				}
				break
			}
			select {
			case <-ctx.Done():
				conn.Close()
				return
			default:
			}
		}
		sleepOrDone(ctx, 2*time.Second)
	}
}

func sleepOrDone(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (a *App) handleSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(settingsHTML))
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, status := a.snapshot()
		writeJSON(w, map[string]any{"config": cfg, "status": status})
	case http.MethodPost:
		var cfg Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := setRunAtStartup(cfg.RunAtStartup); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.mu.Lock()
		wasRunning := a.status.Running
		a.mu.Unlock()
		if wasRunning {
			a.stopReceiving()
		}
		a.mu.Lock()
		a.config = cfg
		a.mu.Unlock()
		if wasRunning {
			a.startReceiving()
		}
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleStart(w http.ResponseWriter, _ *http.Request) {
	a.startReceiving()
	writeJSON(w, map[string]string{"ok": "true"})
}

func (a *App) handleStop(w http.ResponseWriter, _ *http.Request) {
	a.stopReceiving()
	writeJSON(w, map[string]string{"ok": "true"})
}

func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		http.Redirect(w, r, "/settings", http.StatusFound)
		return
	}
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	a.mu.Lock()
	a.clients[conn] = true
	a.mu.Unlock()

	go func() {
		defer func() {
			conn.Close()
			a.mu.Lock()
			delete(a.clients, conn)
			a.mu.Unlock()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func openBrowser(url string) {
	if runtime.GOOS != "windows" {
		return
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func main() {
	cfg := loadConfig()
	app := NewApp(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleWebSocket)
	mux.HandleFunc("/settings", app.handleSettings)
	mux.HandleFunc("/api/config", app.handleConfig)
	mux.HandleFunc("/api/start", app.handleStart)
	mux.HandleFunc("/api/stop", app.handleStop)

	if cfg.StartReceivingOnRun {
		app.startReceiving()
	}

	address := net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.ListenPort))
	settingsURL := fmt.Sprintf("http://%s/settings", address)
	if len(os.Args) == 1 {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openBrowser(settingsURL)
		}()
	}

	log.Printf("Windy NMEA Bridge listening at ws://%s", address)
	log.Printf("Settings: %s", settingsURL)
	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatal(err)
	}
}
