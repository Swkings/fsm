package visualization

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidDiscovery  = errors.New("invalid visualization service discovery")
	ErrDiscoveryRepeated = errors.New("visualization service discovery id is repeated")
	ErrDiscoveryNotFound = errors.New("visualization service discovery not found")
)

type DiscoveredService struct {
	Key            string        `json:"key,omitempty"`
	RegistrationID string        `json:"registrationId,omitempty"`
	Endpoint       string        `json:"endpoint"`
	DisplayName    string        `json:"displayName,omitempty"`
	Path           string        `json:"path,omitempty"`
	PullInterval   time.Duration `json:"-"`
}

// ServiceDiscovery 发现可直接拉取的单个远程状态机端点。
type ServiceDiscovery interface {
	Discover(context.Context) ([]DiscoveredService, error)
}

// DiscoveredServer 描述注册中心发现的业务进程可视化 Client Server。
type DiscoveredServer = DiscoveredService

// ServerDiscovery 发现业务进程可视化 Client Server 地址。
type ServerDiscovery interface {
	Discover(context.Context) ([]DiscoveredServer, error)
}

type DiscoveryConfig struct {
	ID              string
	Name            string
	PathPrefix      string
	RefreshInterval time.Duration
	PullInterval    time.Duration
}

type DiscoverySnapshot struct {
	ID              string
	Name            string
	Endpoint        string
	PathPrefix      string
	RefreshInterval time.Duration
	PullInterval    time.Duration
	Discovered      int
	Registered      int
	LastError       string
	LastRefresh     time.Time
}

type DiscoveryRegistration struct {
	server *Server
	id     string
	done   <-chan struct{}
}

func (registration *DiscoveryRegistration) ID() string            { return registration.id }
func (registration *DiscoveryRegistration) Done() <-chan struct{} { return registration.done }
func (registration *DiscoveryRegistration) Unregister() bool {
	return registration != nil && registration.server != nil &&
		registration.server.UnregisterDiscovery(registration.id)
}

type discoveredRegistration struct {
	service      DiscoveredService
	registration *Registration
}

type registeredDiscovery struct {
	server    *Server
	config    DiscoveryConfig
	discovery ServiceDiscovery
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	refresh   chan chan error

	mu         sync.RWMutex
	services   map[string]*discoveredRegistration
	discovered int
	lastError  string
	lastUpdate time.Time
}

func normalizeDiscoveryConfig(config DiscoveryConfig) (DiscoveryConfig, error) {
	config.ID = strings.TrimSpace(config.ID)
	if config.ID == "" || strings.ContainsAny(config.ID, "/\\") {
		return DiscoveryConfig{}, fmt.Errorf("%w: invalid id %q", ErrInvalidDiscovery, config.ID)
	}
	config.Name = strings.TrimSpace(config.Name)
	if config.Name == "" {
		config.Name = config.ID
	}
	path, err := NormalizePath(config.PathPrefix)
	if err != nil {
		return DiscoveryConfig{}, err
	}
	config.PathPrefix = path
	if config.RefreshInterval < 0 || config.PullInterval < 0 {
		return DiscoveryConfig{}, fmt.Errorf("%w: intervals must not be negative", ErrInvalidDiscovery)
	}
	if config.RefreshInterval == 0 {
		config.RefreshInterval = 30 * time.Second
	}
	if config.PullInterval == 0 {
		config.PullInterval = 5 * time.Second
	}
	return config, nil
}

func (server *Server) RegisterDiscovery(ctx context.Context, discovery ServiceDiscovery, config DiscoveryConfig) (*DiscoveryRegistration, error) {
	if server == nil || isNilInterface(discovery) {
		return nil, ErrInvalidDiscovery
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := normalizeDiscoveryConfig(config)
	if err != nil {
		return nil, err
	}
	discoveryContext, cancel := context.WithCancel(ctx)
	registered := &registeredDiscovery{
		server: server, config: normalized, discovery: discovery,
		ctx: discoveryContext, cancel: cancel, done: make(chan struct{}),
		refresh: make(chan chan error), services: make(map[string]*discoveredRegistration),
	}
	server.mu.RLock()
	if server.closed {
		server.mu.RUnlock()
		cancel()
		return nil, ErrServerClosed
	}
	server.discoveryMu.Lock()
	if _, exists := server.discoveries[normalized.ID]; exists {
		server.discoveryMu.Unlock()
		server.mu.RUnlock()
		cancel()
		return nil, fmt.Errorf("%w: %s", ErrDiscoveryRepeated, normalized.ID)
	}
	server.discoveries[normalized.ID] = registered
	server.discoveryMu.Unlock()
	server.mu.RUnlock()
	go registered.run()
	return &DiscoveryRegistration{server: server, id: normalized.ID, done: registered.done}, nil
}

func (registered *registeredDiscovery) run() {
	defer func() {
		registered.unregisterAll()
		registered.server.discoveryMu.Lock()
		if current, exists := registered.server.discoveries[registered.config.ID]; exists && current == registered {
			delete(registered.server.discoveries, registered.config.ID)
		}
		registered.server.discoveryMu.Unlock()
		close(registered.done)
		registered.server.broadcast("discovery", registered.config.ID)
	}()
	_ = registered.reconcile()
	ticker := time.NewTicker(registered.config.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = registered.reconcile()
		case result := <-registered.refresh:
			result <- registered.reconcile()
		case <-registered.ctx.Done():
			return
		}
	}
}

func (registered *registeredDiscovery) reconcile() error {
	services, err := safeDiscover(registered.discovery, registered.ctx)
	if err != nil {
		registered.updateStatus(0, err)
		return err
	}
	desired := make(map[string]DiscoveredService, len(services))
	var reconcileErrors []error
	for _, service := range services {
		normalized, normalizeErr := normalizeDiscoveredService(service)
		if normalizeErr != nil {
			reconcileErrors = append(reconcileErrors, normalizeErr)
			continue
		}
		if _, repeated := desired[normalized.Key]; repeated {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("%w: repeated service key %q", ErrInvalidDiscovery, normalized.Key))
			continue
		}
		desired[normalized.Key] = normalized
	}
	registered.mu.RLock()
	existing := make(map[string]*discoveredRegistration, len(registered.services))
	for key, current := range registered.services {
		existing[key] = current
	}
	registered.mu.RUnlock()
	for key, current := range existing {
		target, exists := desired[key]
		if exists && sameDiscoveredService(current.service, target) && registrationActive(current.registration) {
			continue
		}
		current.registration.Unregister()
		registered.mu.Lock()
		delete(registered.services, key)
		registered.mu.Unlock()
	}
	for key, service := range desired {
		registered.mu.RLock()
		_, exists := registered.services[key]
		registered.mu.RUnlock()
		if exists {
			continue
		}
		if registerErr := registered.registerService(service); registerErr != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("register discovered service %q: %w", key, registerErr))
		}
	}
	err = errors.Join(reconcileErrors...)
	registered.updateStatus(len(desired), err)
	registered.server.broadcast("discovery", registered.config.ID)
	return err
}

func registrationActive(registration *Registration) bool {
	if registration == nil {
		return false
	}
	select {
	case <-registration.Done():
		return false
	default:
		return true
	}
}

func safeDiscover(discovery ServiceDiscovery, ctx context.Context) (services []DiscoveredService, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: panic: %v", ErrInvalidDiscovery, recovered)
		}
	}()
	return discovery.Discover(ctx)
}

func normalizeDiscoveredService(service DiscoveredService) (DiscoveredService, error) {
	service.Endpoint = strings.TrimRight(strings.TrimSpace(service.Endpoint), "/")
	if err := validateRemoteEndpoint(service.Endpoint); err != nil {
		return DiscoveredService{}, err
	}
	service.Key = strings.TrimSpace(service.Key)
	if service.Key == "" {
		service.Key = service.Endpoint
	}
	service.RegistrationID = strings.TrimSpace(service.RegistrationID)
	if strings.ContainsAny(service.RegistrationID, "/\\") {
		return DiscoveredService{}, fmt.Errorf("%w: invalid registration id %q", ErrInvalidDiscovery, service.RegistrationID)
	}
	service.DisplayName = strings.TrimSpace(service.DisplayName)
	path, err := NormalizePath(service.Path)
	if err != nil {
		return DiscoveredService{}, err
	}
	service.Path = path
	if service.PullInterval < 0 {
		return DiscoveredService{}, fmt.Errorf("%w: pull interval must not be negative", ErrInvalidDiscovery)
	}
	return service, nil
}

func validateRemoteEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: invalid endpoint %q", ErrInvalidDiscovery, endpoint)
	}
	return nil
}

func remoteResourceEndpoint(endpoint, suffix string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return strings.TrimRight(endpoint, "/") + suffix
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + suffix
	return parsed.String()
}

func sameDiscoveredService(left, right DiscoveredService) bool {
	return left.Key == right.Key && left.Endpoint == right.Endpoint &&
		left.RegistrationID == right.RegistrationID &&
		left.DisplayName == right.DisplayName && left.Path == right.Path &&
		left.PullInterval == right.PullInterval
}

func (registered *registeredDiscovery) registerService(service DiscoveredService) error {
	client := &remotePullClient{
		ctx: registered.ctx, endpoint: service.Endpoint,
		registrationID: service.RegistrationID, client: &http.Client{Timeout: 10 * time.Second},
	}
	pullInterval := service.PullInterval
	if pullInterval == 0 {
		pullInterval = registered.config.PullInterval
	}
	registration, err := registered.server.Register(registered.ctx, client, RegistrationConfig{
		DisplayName: service.DisplayName,
		Path:        joinDiscoveryPath(registered.config.PathPrefix, service.Path),
		Mode:        ModePull, PullInterval: pullInterval,
	})
	if err != nil {
		return err
	}
	registered.mu.Lock()
	registered.services[service.Key] = &discoveredRegistration{service: service, registration: registration}
	registered.mu.Unlock()
	return nil
}

func joinDiscoveryPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	return prefix + "/" + path
}

func (registered *registeredDiscovery) updateStatus(discovered int, err error) {
	registered.mu.Lock()
	registered.discovered = discovered
	registered.lastUpdate = time.Now()
	if err == nil {
		registered.lastError = ""
	} else {
		registered.lastError = err.Error()
	}
	registered.mu.Unlock()
}

func (registered *registeredDiscovery) unregisterAll() {
	registered.mu.Lock()
	services := make([]*discoveredRegistration, 0, len(registered.services))
	for _, service := range registered.services {
		services = append(services, service)
	}
	registered.services = make(map[string]*discoveredRegistration)
	registered.mu.Unlock()
	for _, service := range services {
		service.registration.Unregister()
	}
}

func (registered *registeredDiscovery) snapshot() DiscoverySnapshot {
	registered.mu.RLock()
	defer registered.mu.RUnlock()
	return DiscoverySnapshot{
		ID: registered.config.ID, Name: registered.config.Name,
		Endpoint: discoveryEndpoint(registered.discovery), PathPrefix: registered.config.PathPrefix,
		RefreshInterval: registered.config.RefreshInterval, PullInterval: registered.config.PullInterval,
		Discovered: registered.discovered, Registered: len(registered.services),
		LastError: registered.lastError, LastRefresh: registered.lastUpdate,
	}
}

func discoveryEndpoint(discovery ServiceDiscovery) string {
	switch source := discovery.(type) {
	case *HTTPServiceDiscovery:
		return source.Endpoint
	case *ClientServerDiscovery:
		return discoveryEndpoint(source.Discovery)
	default:
		return ""
	}
}

func (server *Server) DiscoverySnapshots() []DiscoverySnapshot {
	server.discoveryMu.RLock()
	snapshots := make([]DiscoverySnapshot, 0, len(server.discoveries))
	for _, discovery := range server.discoveries {
		snapshots = append(snapshots, discovery.snapshot())
	}
	server.discoveryMu.RUnlock()
	sort.Slice(snapshots, func(left, right int) bool {
		if snapshots[left].Name != snapshots[right].Name {
			return snapshots[left].Name < snapshots[right].Name
		}
		return snapshots[left].ID < snapshots[right].ID
	})
	return snapshots
}

func (server *Server) RefreshDiscovery(ctx context.Context, id string) error {
	server.discoveryMu.RLock()
	registered, exists := server.discoveries[id]
	server.discoveryMu.RUnlock()
	if !exists {
		return fmt.Errorf("%w: %s", ErrDiscoveryNotFound, id)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan error, 1)
	select {
	case registered.refresh <- result:
	case <-registered.done:
		return fmt.Errorf("%w: %s", ErrDiscoveryNotFound, id)
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-result:
		return err
	case <-registered.done:
		return fmt.Errorf("%w: %s", ErrDiscoveryNotFound, id)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (server *Server) UnregisterDiscovery(id string) bool {
	server.discoveryMu.Lock()
	registered, exists := server.discoveries[id]
	if exists {
		delete(server.discoveries, id)
	}
	server.discoveryMu.Unlock()
	if !exists {
		return false
	}
	registered.cancel()
	<-registered.done
	server.broadcast("discovery", id)
	return true
}

func (server *Server) stopAllDiscoveries() {
	server.discoveryMu.Lock()
	discoveries := make([]*registeredDiscovery, 0, len(server.discoveries))
	for _, discovery := range server.discoveries {
		discoveries = append(discoveries, discovery)
	}
	server.discoveries = make(map[string]*registeredDiscovery)
	server.discoveryMu.Unlock()
	for _, discovery := range discoveries {
		discovery.cancel()
	}
	for _, discovery := range discoveries {
		<-discovery.done
	}
}
